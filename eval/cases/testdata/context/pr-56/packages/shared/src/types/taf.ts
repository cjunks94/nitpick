/**
 * TAF (Terminal Aerodrome Forecast) — 24-30h forecast issued every 6h
 * (or amended ad-hoc) for an airport. Surfaced in the airport popup
 * alongside METAR so a pilot can see both current conditions and the
 * forecast window without leaving the map.
 *
 * Shape mirrors the parsed structure aviationweather.gov returns, with
 * the `fcsts` array (forecast periods) preserved structurally so the
 * client can pick the period covering "now" at render time. Units
 * normalized for US flight ops (knots, statute miles, feet) — same as
 * MetarReport.
 */

/** A single forecast period within a TAF. The TAF is composed of a
 *  base period (fcstChange === null at index 0, or "FM" for subsequent
 *  base shifts) plus optional overlays (TEMPO, BECMG, PROB) that modify
 *  the base during a sub-window.
 *
 *  For v1 display, the client picks the most-recent "base" period
 *  (fcstChange null/FM) covering Date.now() and renders that. TEMPO/
 *  BECMG overlays are ignored at the summary line — they're still
 *  visible in the raw TAF if the pilot expands. */
export interface TafPeriod {
  /** Period start, epoch ms. */
  timeFrom: number;
  /** Period end, epoch ms. */
  timeTo: number;
  /** Change indicator. null = base period at the start of the TAF.
   *  "FM" = base period change at this time. "TEMPO" = temporary
   *  conditions during the window. "BECMG" = becoming, gradual change.
   *  "PROB" = probability group (with `probability` set). */
  fcstChange: "FM" | "TEMPO" | "BECMG" | "PROB" | null;
  /** Probability percent for PROB groups (typically 30 or 40). */
  probability: number | null;
  wind: {
    /** True heading in degrees, 0–360. Null = variable / calm / not in this group. */
    directionDeg: number | null;
    /** Sustained speed in knots. Null when this group inherits wind from
     *  the base (TEMPO/BECMG groups frequently omit fields they don't change). */
    speedKt: number | null;
    /** Gust speed in knots, null if no gusts. */
    gustKt: number | null;
  } | null;
  /** Statute miles. "P6SM" (>6 sm unrestricted) → 6 with the raw text
   *  carrying the precise wording. Null when this group doesn't change vis. */
  visibilitySm: number | null;
  /** Lowest broken/overcast layer in feet AGL, null if none in this group. */
  ceilingFt: number | null;
  /** Weather phenomena string like "-SHRA BR" (light rain, mist) or null. */
  wxString: string | null;
}

export interface TafReport {
  /** ICAO code (e.g. "KJFK") — matches AIRPORTS.icao for popup lookup. */
  icao: string;
  /** Issue time, epoch ms. */
  issuedAt: number;
  /** Validity window start, epoch ms. */
  validFrom: number;
  /** Validity window end, epoch ms. */
  validTo: number;
  /** Verbatim TAF string. The popup offers an expand-to-raw view because
   *  pilots are trained to read the original encoding directly. */
  raw: string;
  /** Forecast periods in upstream order. Includes both base (FM) and
   *  overlay (TEMPO/BECMG/PROB) entries. */
  forecasts: TafPeriod[];
}

export interface TafsResponse {
  /** Server timestamp, epoch ms — when this snapshot was assembled. */
  timestamp: number;
  /** Last successful upstream fetch time, epoch ms. */
  sourceTimestamp: number;
  /** Per-airport reports keyed by ICAO. Missing entry = no current TAF
   *  (uncommon — most major airports always have a current TAF). */
  reports: Record<string, TafReport>;
  source: "live" | "cached";
}
