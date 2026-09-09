import type { TafPeriod, TafReport, TafsResponse } from "@panoptrain/shared";
import { AIRPORTS } from "@panoptrain/shared";

/**
 * TAF poller — terminal aerodrome forecasts from aviationweather.gov
 * for each airport in the AIRPORTS list. The popup surfaces the issue
 * time, validity window, current forecast period, and full raw text.
 *
 * TAFs issue every 6 hours (00, 06, 12, 18 UTC) and are valid for 24-30
 * hours. Amendments fire mid-cycle when conditions deviate from the
 * forecast. Polling every 30 minutes catches amendments within a
 * useful window without thrashing the upstream — TAFs change far less
 * frequently than METARs.
 *
 * aviationweather.gov etiquette mirrors the METAR poller: identifiable
 * User-Agent, JSON format only.
 */
const AVIATIONWEATHER_BASE = "https://aviationweather.gov/api/data/taf";
const USER_AGENT = "panoptrain/0.1.0 (contact: cjunks94@gmail.com)";
const FETCH_TIMEOUT_MS = 10_000;
const RETRY_DELAYS_MS = [0, 1_000, 3_000];
/** Eight hours — TAFs are valid 24-30h, but the cache fallback only
 *  needs to bridge a transient upstream outage during the user's active
 *  session. Beyond ~8h the forecast itself starts to feel stale even
 *  if it's still technically within validity. */
const CACHE_TTL_MS = 8 * 60 * 60 * 1000;

let interval: ReturnType<typeof setInterval> | undefined;
let snapshot: TafsResponse | null = null;
let cached: { snapshot: TafsResponse; cachedAt: number } | null = null;

const ALL_ICAO = AIRPORTS.map((a) => a.icao);

/** Raw record shape from aviationweather.gov's /api/data/taf endpoint
 *  with format=json. We declare only the fields we read. */
interface AwxTafRecord {
  icaoId?: string;
  /** ISO 8601 timestamp string. */
  issueTime?: string;
  /** Unix seconds. */
  validTimeFrom?: number;
  validTimeTo?: number;
  rawTAF?: string;
  fcsts?: AwxTafForecast[];
}

interface AwxTafForecast {
  /** Unix seconds. */
  timeFrom?: number;
  timeTo?: number;
  /** "FM" | "TEMPO" | "BECMG" | "PROB" | null */
  fcstChange?: string | null;
  probability?: number | null;
  wdir?: number | string | null;
  wspd?: number | null;
  wgst?: number | null;
  visib?: number | string | null;
  wxString?: string | null;
  clouds?: Array<{ cover?: string; base?: number | null }>;
}

function parseVisibility(visib: AwxTafForecast["visib"]): number | null {
  if (visib === undefined || visib === null) return null;
  if (typeof visib === "number") return visib;
  // "P6SM" surfaces from raw TAF parsing as the numeric "6+" / similar.
  // The JSON feed sometimes ships strings; handle the common cases.
  if (visib === "P6SM" || visib === "6+") return 6;
  const slash = visib.indexOf("/");
  if (slash > 0) {
    const num = Number(visib.slice(0, slash));
    const den = Number(visib.slice(slash + 1));
    if (Number.isFinite(num) && Number.isFinite(den) && den > 0) return num / den;
  }
  const n = Number(visib);
  return Number.isFinite(n) ? n : null;
}

function parseDirection(wdir: AwxTafForecast["wdir"]): number | null {
  if (wdir === undefined || wdir === null) return null;
  if (typeof wdir === "number") return wdir;
  // "VRB" and other strings — null is the right "no fixed direction" signal.
  return null;
}

function deriveCeiling(clouds: AwxTafForecast["clouds"]): number | null {
  if (!clouds || clouds.length === 0) return null;
  // Lowest BKN/OVC/VV layer is the ceiling. Scan all qualifying layers
  // rather than returning the first match — the upstream typically
  // sorts by altitude ascending, but the API doesn't document that
  // contract, and an out-of-order layer would otherwise misreport the
  // ceiling in the popup summary.
  let lowest: number | null = null;
  for (const layer of clouds) {
    const cover = (layer.cover ?? "").toUpperCase();
    if ((cover === "BKN" || cover === "OVC" || cover === "VV") && typeof layer.base === "number") {
      if (lowest === null || layer.base < lowest) lowest = layer.base;
    }
  }
  return lowest;
}

const VALID_CHANGES = new Set(["FM", "TEMPO", "BECMG", "PROB"]);

function parseChange(c: AwxTafForecast["fcstChange"]): TafPeriod["fcstChange"] {
  if (!c) return null;
  const upper = c.toUpperCase();
  // PROB30/PROB40 surface as "PROB" with a probability number alongside.
  if (upper.startsWith("PROB")) return "PROB";
  return VALID_CHANGES.has(upper) ? (upper as TafPeriod["fcstChange"]) : null;
}

function parseForecast(f: AwxTafForecast): TafPeriod | null {
  if (typeof f.timeFrom !== "number" || typeof f.timeTo !== "number") return null;
  const wspdParsed = typeof f.wspd === "number" ? f.wspd : null;
  const wdirParsed = parseDirection(f.wdir);
  // Suppress wind block when neither speed nor direction is present —
  // the upstream omits them on overlay groups (TEMPO/BECMG) that don't
  // change wind from the base period.
  const wind = wspdParsed === null && wdirParsed === null
    ? null
    : {
        directionDeg: wdirParsed,
        speedKt: wspdParsed,
        gustKt: typeof f.wgst === "number" ? f.wgst : null,
      };
  return {
    timeFrom: f.timeFrom * 1000,
    timeTo: f.timeTo * 1000,
    fcstChange: parseChange(f.fcstChange),
    probability: typeof f.probability === "number" ? f.probability : null,
    wind,
    visibilitySm: parseVisibility(f.visib),
    ceilingFt: deriveCeiling(f.clouds),
    wxString: f.wxString && f.wxString.trim().length > 0 ? f.wxString.trim() : null,
  };
}

function parseRecord(rec: AwxTafRecord): TafReport | null {
  if (!rec.icaoId || !rec.issueTime || typeof rec.validTimeFrom !== "number" ||
      typeof rec.validTimeTo !== "number") {
    return null;
  }
  const issuedAtMs = Date.parse(rec.issueTime);
  if (!Number.isFinite(issuedAtMs)) return null;
  const forecasts: TafPeriod[] = [];
  for (const f of rec.fcsts ?? []) {
    const parsed = parseForecast(f);
    if (parsed) forecasts.push(parsed);
  }
  return {
    icao: rec.icaoId,
    issuedAt: issuedAtMs,
    validFrom: rec.validTimeFrom * 1000,
    validTo: rec.validTimeTo * 1000,
    raw: rec.rawTAF ?? "",
    forecasts,
  };
}

export function parseTafResponse(records: AwxTafRecord[], now: number = Date.now()): TafsResponse {
  const reports: Record<string, TafReport> = {};
  for (const rec of records) {
    const parsed = parseRecord(rec);
    if (parsed) reports[parsed.icao] = parsed;
  }
  // Source timestamp = the latest issue time we have. Beats Date.now()
  // because TAFs publish on the 6h cycle.
  let latestIssued = 0;
  for (const r of Object.values(reports)) {
    if (r.issuedAt > latestIssued) latestIssued = r.issuedAt;
  }
  return {
    timestamp: now,
    sourceTimestamp: latestIssued || now,
    reports,
    source: "live",
  };
}

export async function fetchTafSnapshot(): Promise<TafsResponse> {
  const url = `${AVIATIONWEATHER_BASE}?ids=${ALL_ICAO.join(",")}&format=json`;
  let lastErr: unknown = null;
  for (let attempt = 0; attempt < RETRY_DELAYS_MS.length; attempt++) {
    if (RETRY_DELAYS_MS[attempt] > 0) {
      await new Promise((r) => setTimeout(r, RETRY_DELAYS_MS[attempt]));
    }
    try {
      const res = await fetch(url, {
        signal: AbortSignal.timeout(FETCH_TIMEOUT_MS),
        headers: { "User-Agent": USER_AGENT, accept: "application/json" },
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const records = (await res.json()) as AwxTafRecord[];
      return parseTafResponse(records);
    } catch (err) {
      lastErr = err;
    }
  }
  throw lastErr instanceof Error ? lastErr : new Error(String(lastErr));
}

async function pollOnce(): Promise<void> {
  try {
    const next = await fetchTafSnapshot();
    snapshot = next;
    cached = { snapshot: next, cachedAt: next.timestamp };
    console.log(`[taf] ok, ${Object.keys(next.reports).length} reports`);
  } catch (err) {
    if (cached && Date.now() - cached.cachedAt < CACHE_TTL_MS) {
      const ageMin = Math.round((Date.now() - cached.cachedAt) / 60_000);
      console.warn(
        `[taf] fetch failed (${err}); reusing cached snapshot from ${ageMin}m ago`,
      );
      snapshot = { ...cached.snapshot, source: "cached", timestamp: Date.now() };
    } else {
      console.warn(`[taf] fetch failed (${err}); no usable cache`);
      snapshot = null;
    }
  }
}

export function startTafPolling(intervalMs: number): void {
  stopTafPolling();
  console.log(`Starting TAF polling every ${intervalMs / 60_000}m...`);
  pollOnce();
  interval = setInterval(pollOnce, intervalMs);
}

export function stopTafPolling(): void {
  if (interval) clearInterval(interval);
  interval = undefined;
}

export function getCurrentTafSnapshot(): TafsResponse | null {
  return snapshot;
}

/** Test-only — production code never calls this. */
export function _resetTafCache(): void {
  snapshot = null;
  cached = null;
}
