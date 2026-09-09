import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import "maplibre-gl/dist/maplibre-gl.css";
import type { TripPlan, LirrTripPlan, Mode } from "@panoptrain/shared";
import { AppShell } from "./components/Layout/AppShell.js";
import { TransitMap } from "./components/Map/TransitMap.js";
import { FilterPanel } from "./components/Panel/FilterPanel.js";
import { useTrainPositions } from "./hooks/useTrainPositions.js";
import { useTrainFeatures } from "./hooks/useTrainFeatures.js";
import { useRouteShapes } from "./hooks/useRouteShapes.js";
import { useLineFilter } from "./hooks/useLineFilter.js";
import { useView, transitModeFor } from "./hooks/useView.js";
import { useAircraftPositions } from "./hooks/useAircraftPositions.js";
import { useMetars } from "./hooks/useMetars.js";
import { useTafs } from "./hooks/useTafs.js";
import { MOBILE_QUERY } from "./hooks/useIsMobile.js";
import { fetchRoutes, fetchStops } from "./lib/api.js";
import {
  getRoutes as getCachedRoutes,
  getStops as getCachedStops,
  setRoutes as setCachedRoutes,
  setStops as setCachedStops,
} from "./lib/modeCache.js";
import { cancelIdle, scheduleIdle } from "./lib/scheduleIdle.js";

export default function App() {
  const [view, setView] = useView();
  const transitMode = transitModeFor(view);
  // useLineFilter holds per-mode local state (no fetching). It needs *some*
  // Mode at all times even on the airspace view, since hooks can't be
  // skipped. Track the last transit mode the user was on and feed that
  // through; on airspace the line-filter UI is hidden anyway, so this is
  // just keeping the state object intact for the next subway/LIRR visit.
  const lastTransitModeRef = useRef<Mode>(transitMode ?? "subway");
  useEffect(() => {
    if (transitMode !== null) lastTransitModeRef.current = transitMode;
  }, [transitMode]);
  const lineFilterMode = transitMode ?? lastTransitModeRef.current;

  const { data, isStale, lastUpdated } = useTrainPositions(transitMode);
  const { routeShapes, stopsGeoJson, loading: routeShapesLoading } = useRouteShapes(transitMode);
  const { visibleRoutes, toggleRoute, toggleGroup, allOn, allOff } = useLineFilter(lineFilterMode);
  const { aircraft } = useAircraftPositions(view === "airspace");
  // METAR + TAF feeds only matter when the user can open an airport
  // popup, which only happens on the airspace tab. Same gate as aircraft.
  const { reports: metarReports } = useMetars(view === "airspace");
  const { reports: tafReports } = useTafs(view === "airspace");

  // Airport popup state lives at App so the directory in FilterPanel can
  // open the same popup as a map-click would. Token-based fly-to so the
  // directory click pans the camera to the airport while a map-click on
  // a visible dot leaves the camera alone.
  const [popupAirportIata, setPopupAirportIata] = useState<string | null>(null);
  const [flyToToken, setFlyToToken] = useState<{ iata: string; nonce: number } | null>(null);
  const handleSelectAirport = useCallback((iata: string) => {
    setPopupAirportIata(iata);
    // Date.now() makes each click a fresh token even if the user re-clicks
    // the same airport — the effect-watch on the TransitMap side fires
    // every time, so re-clicking re-centers if the user has panned away.
    setFlyToToken({ iata, nonce: Date.now() });
  }, []);
  const handleClearAirport = useCallback(() => setPopupAirportIata(null), []);
  // Default closed on mobile so the bottom sheet doesn't take up 75vh on
  // first paint — users land on the map, then tap to filter. Desktop keeps
  // the sidebar open by default since it doesn't cover the map. One-shot
  // matchMedia (not the reactive useIsMobile hook) so a desktop user
  // resizing down to mobile width doesn't have the panel slammed shut.
  const [panelOpen, setPanelOpen] = useState(() => {
    if (typeof window === "undefined") return true;
    return !window.matchMedia(MOBILE_QUERY).matches;
  });
  // On mobile the airport briefing renders inside the panel, so any
  // path that selects an airport (map-click or directory pick) must
  // also open the panel — otherwise the briefing hides behind a
  // collapsed bottom sheet. Effect rather than wiring into both
  // selection paths so map-click on an airport dot is covered too
  // (TransitMap calls setPopupAirportIata directly via the prop).
  useEffect(() => {
    if (popupAirportIata && typeof window !== "undefined" && window.matchMedia(MOBILE_QUERY).matches) {
      setPanelOpen(true);
    }
  }, [popupAirportIata]);
  // Spotlight state accepts either subway or LIRR plans; downstream consumers
  // (planRouteIds memo, TransitMap) only read the shared subset of fields
  // (segments[].type, .routeId, .path), so a union is sufficient — no adapter
  // layer needed.
  const [planRoute, setPlanRoute] = useState<TripPlan | LirrTripPlan | null>(null);

  // Clear any active plan when the user switches off the transit view that
  // produced it — a subway plan is meaningless on LIRR, and neither plan is
  // meaningful on airspace.
  useEffect(() => {
    setPlanRoute(null);
  }, [transitMode]);

  // Background-preload the inactive mode's routes/stops on idle so the
  // first cross-mode tab switch hits the modeCache instead of paying the
  // multi-MB GeoJSON download + parse on the click. Runs whenever the
  // active mode changes (or on first mount), schedules the fetches
  // through requestIdleCallback so it doesn't fight the current mode's
  // first paint. From airspace we preload subway only — most users
  // landing on airspace are coming from or going to the transit view,
  // and subway is the more common destination of the two.
  useEffect(() => {
    const inactive: Mode[] = transitMode === null
      ? ["subway"]
      : transitMode === "subway" ? ["lirr"] : ["subway"];

    let cancelled = false;
    const handle = scheduleIdle(() => {
      if (cancelled) return;
      for (const mode of inactive) {
        if (!getCachedRoutes(mode)) {
          fetchRoutes(mode)
            .then((r) => {
              // Re-check on resolve: if a foreground useRouteShapes
              // fetch beat us to it, don't overwrite (would change the
              // object identity and bust the WeakMap-cached shape index).
              if (!cancelled && !getCachedRoutes(mode)) setCachedRoutes(mode, r);
            })
            .catch(() => { /* preload is best-effort */ });
        }
        if (!getCachedStops(mode)) {
          fetchStops(mode)
            .then((s) => {
              if (!cancelled && !getCachedStops(mode)) setCachedStops(mode, s);
            })
            .catch(() => { /* preload is best-effort */ });
        }
      }
    }, 1500);

    return () => {
      cancelled = true;
      cancelIdle(handle);
    };
  }, [transitMode]);

  // When a plan is active, surface only the routes that plan rides — these
  // are the trains we want to spotlight on the map (PT-309).
  const planRouteIds = useMemo<Set<string> | null>(() => {
    if (!planRoute) return null;
    const ids = new Set<string>();
    for (const seg of planRoute.segments) {
      if (seg.type === "ride") ids.add(seg.routeId);
    }
    return ids;
  }, [planRoute]);

  const { geojsonRef, interpolateFrame, trains } = useTrainFeatures(
    data, visibleRoutes, routeShapes, planRouteIds, transitMode,
  );

  const togglePanel = useCallback(() => setPanelOpen((p) => !p), []);

  return (
    <AppShell>
      <TransitMap
        geojsonRef={geojsonRef}
        interpolateFrame={interpolateFrame}
        trains={trains}
        routeShapes={routeShapes}
        stops={stopsGeoJson}
        planRoute={planRoute}
        planRouteIds={planRouteIds}
        mode={transitMode}
        panelOpen={panelOpen}
        routeShapesLoading={routeShapesLoading}
        aircraft={aircraft}
        metarReports={metarReports}
        tafReports={tafReports}
        popupAirportIata={popupAirportIata}
        onPopupAirportIataChange={setPopupAirportIata}
        flyToToken={flyToToken}
      />
      <FilterPanel
        open={panelOpen}
        onToggle={togglePanel}
        view={view}
        transitMode={transitMode}
        onViewChange={setView}
        visibleRoutes={visibleRoutes}
        onToggleRoute={toggleRoute}
        onToggleGroup={toggleGroup}
        onAllOn={allOn}
        onAllOff={allOff}
        isStale={isStale}
        lastUpdated={lastUpdated}
        trainCount={data?.count ?? 0}
        stops={stopsGeoJson}
        liveTrains={data?.trains ?? []}
        onPlanFound={setPlanRoute}
        aircraftCount={aircraft.length}
        metarReports={metarReports}
        tafReports={tafReports}
        activeAirportIata={popupAirportIata}
        onSelectAirport={handleSelectAirport}
        onClearAirport={handleClearAirport}
      />
    </AppShell>
  );
}
