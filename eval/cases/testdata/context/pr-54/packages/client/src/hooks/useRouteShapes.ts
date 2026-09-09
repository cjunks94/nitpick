import { useState, useEffect } from "react";
import type { Mode, RoutesGeoJSON, StopsGeoJSON } from "@panoptrain/shared";
import { fetchRoutes, fetchStops } from "../lib/api.js";
import {
  getRoutes as getCachedRoutes,
  setRoutes as setCachedRoutes,
  getStops as getCachedStops,
  setStops as setCachedStops,
} from "../lib/modeCache.js";

interface UseRouteShapesResult {
  routeShapes: RoutesGeoJSON | null;
  stopsGeoJson: StopsGeoJSON | null;
  loading: boolean;
}

export function useRouteShapes(mode: Mode | null): UseRouteShapesResult {
  const [routeShapes, setRouteShapes] = useState<RoutesGeoJSON | null>(null);
  const [stopsGeoJson, setStopsGeoJson] = useState<StopsGeoJSON | null>(null);

  useEffect(() => {
    let cancelled = false;

    if (mode === null) {
      // Airspace view — clear transit shapes (no place for them here).
      setRouteShapes(null);
      setStopsGeoJson(null);
      return;
    }

    // Cache hit replaces the prior mode's payload synchronously, so there's
    // no need to flash null first. On miss, set null then fetch — preserves
    // the original "don't show subway shapes briefly while LIRR loads"
    // guarantee. Stops and routes resolve independently (PT-104).
    const cachedRoutes = getCachedRoutes(mode);
    setRouteShapes(cachedRoutes ?? null);

    const cachedStops = getCachedStops(mode);
    setStopsGeoJson(cachedStops ? enrichStops(cachedStops) : null);

    if (!cachedStops) {
      fetchStops(mode)
        .then((stops) => {
          if (cancelled) return;
          setCachedStops(mode, stops);
          setStopsGeoJson(enrichStops(stops));
        })
        .catch((err) => console.error("Failed to load stops:", err));
    }

    if (!cachedRoutes) {
      fetchRoutes(mode)
        .then((routes) => {
          if (cancelled) return;
          setCachedRoutes(mode, routes);
          setRouteShapes(routes);
        })
        .catch((err) => console.error("Failed to load routes:", err));
    }

    return () => {
      cancelled = true;
    };
  }, [mode]);

  // Derive `loading` from state so it accurately reflects "any payload still
  // pending". With the parallel-fetch pattern a single useState flag would
  // either lie (flips early when one fetch finishes) or hang forever (waits
  // only for one). Consumers can also gate on the individual values directly.
  // On airspace there's nothing to load, so loading is always false there.
  const loading = mode !== null && (routeShapes === null || stopsGeoJson === null);
  return { routeShapes, stopsGeoJson, loading };
}

const MAX_LABEL_ROUTES = 6;

// Defends against React StrictMode double-effects and rapid mode flips
// handing back the same payload — without it, ~127 LIRR features re-do the
// label string ops on every call.
const enrichCache = new WeakMap<StopsGeoJSON, StopsGeoJSON>();

/** Augment each stop feature with derived properties consumed by map layer
 *  expressions: numeric `routeCount` for hub sizing/filtering, and a
 *  preformatted `labelText` ("Times Sq-42 St · 1 2 3 N Q R W") for the
 *  detailed label layer. Doing this once at load avoids reasoning about
 *  array operations inside MapLibre expressions. */
function enrichStops(stops: StopsGeoJSON): StopsGeoJSON {
  const cached = enrichCache.get(stops);
  if (cached) return cached;
  const enriched: StopsGeoJSON = {
    ...stops,
    features: stops.features.map((f) => {
      const routes = f.properties.routes ?? [];
      const shown = routes.slice(0, MAX_LABEL_ROUTES).join(" ");
      const overflow = routes.length > MAX_LABEL_ROUTES ? ` +${routes.length - MAX_LABEL_ROUTES}` : "";
      const labelText = routes.length > 0
        ? `${f.properties.stopName} · ${shown}${overflow}`
        : f.properties.stopName;
      return {
        ...f,
        properties: {
          ...f.properties,
          routeCount: routes.length,
          labelText,
        },
      };
    }),
  };
  enrichCache.set(stops, enriched);
  return enriched;
}
