import { useEffect, useState } from "react";
import type { TafsResponse } from "@panoptrain/shared";
import { fetchTafs } from "../lib/api.js";

// TAFs amend mid-cycle but the base period changes only every 6 hours.
// 10 minutes is a comfortable middle ground — fresh enough to surface
// amendments soon after they're published, slow enough to not waste
// server cycles when nothing's changed.
const POLL_INTERVAL_MS = 10 * 60 * 1000;

interface UseTafsResult {
  /** Map keyed by ICAO ("KJFK"). Empty until the first poll lands. */
  reports: TafsResponse["reports"];
  source: "live" | "cached" | null;
  error: Error | null;
}

/**
 * Polls /api/airspace/taf at a slow cadence and exposes the parsed
 * report map. Single bulk request for all airports — the popup looks
 * up by ICAO. The 503 path (poller hasn't produced a snapshot yet) is
 * treated as "no reports yet" rather than an error so the popup can
 * render the rest of the briefing without weather instead of showing
 * a failure state during cold boot.
 */
export function useTafs(enabled: boolean): UseTafsResult {
  const [reports, setReports] = useState<TafsResponse["reports"]>({});
  const [source, setSource] = useState<"live" | "cached" | null>(null);
  const [error, setError] = useState<Error | null>(null);

  useEffect(() => {
    if (!enabled) {
      setReports({});
      setSource(null);
      setError(null);
      return;
    }
    let cancelled = false;
    // Guard against overlapping polls — the TAF payload is ~25KB so a
    // slow response could overlap a fresh interval tick, and a stale
    // older response landing after a newer one would clobber fresh
    // data. Skip the tick if the previous poll is still in flight.
    let inFlight = false;
    const poll = async () => {
      if (inFlight) return;
      inFlight = true;
      try {
        const res = await fetchTafs();
        if (cancelled) return;
        setReports(res.reports);
        setSource(res.source);
        setError(null);
      } catch (err) {
        if (cancelled) return;
        const msg = err instanceof Error ? err.message : String(err);
        if (msg.startsWith("API 503")) {
          // Poller hasn't run yet (server just started). Surface as
          // "no reports" rather than an error — the popup falls back
          // to its non-TAF rows cleanly.
          setReports({});
          setSource(null);
          setError(null);
          return;
        }
        setError(err instanceof Error ? err : new Error(msg));
      } finally {
        inFlight = false;
      }
    };
    poll();
    const interval = setInterval(poll, POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [enabled]);

  return { reports, source, error };
}
