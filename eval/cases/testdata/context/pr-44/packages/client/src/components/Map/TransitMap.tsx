import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import Map, { Source, Layer } from "react-map-gl/maplibre";
import type { MapLayerMouseEvent, MapRef } from "react-map-gl/maplibre";
import type {
  Aircraft,
  Mode,
  RoutesGeoJSON,
  StopsGeoJSON,
  TripPlan,
  LirrTripPlan,
} from "@panoptrain/shared";
import { ROUTE_INFO, AIRPORTS } from "@panoptrain/shared";
import type { TrainInfo } from "../../hooks/useTrainFeatures.js";
import { useAircraftFeatures } from "../../hooks/useAircraftFeatures.js";
import { useIsMobile } from "../../hooks/useIsMobile.js";
import { useViewportHeight } from "../../hooks/useViewportHeight.js";
import { computeFitPadding } from "../../lib/mapPadding.js";
import { popupOffsetPx } from "../../lib/popupPlacement.js";
import { TrainPopup } from "./TrainPopup.js";
import { MapLoadingBadge } from "./MapLoadingBadge.js";
import { AircraftPopup } from "./AircraftPopup.js";
import { AirportPopup } from "./AirportPopup.js";
import type { GeoJSON } from "geojson";

const BASEMAP = "https://basemaps.cartocdn.com/gl/dark-matter-gl-style/style.json";
const NYC_CENTER = { longitude: -73.98, latitude: 40.75, zoom: 12 };

// Airport GeoJSON is constant — built once at module load. Mirrors the
// shape that station-dots / station-labels-* expressions read from on the
// transit modes, so the layer expressions below reuse the same `importance`
// case-by-bucket pattern.
const AIRPORTS_GEOJSON: GeoJSON.FeatureCollection = {
  type: "FeatureCollection",
  features: AIRPORTS.map((a) => ({
    type: "Feature",
    geometry: { type: "Point", coordinates: [a.longitude, a.latitude] },
    properties: {
      iata: a.iata,
      icao: a.icao,
      name: a.name,
      importance: a.importance,
    },
  })),
};

// All airport layer IDs that should open the popup on click. Listed in
// one place so the click-handler dispatch and `interactiveLayerIds`
// can't drift when layers are added or renamed. The label layers are
// included because at zoom 13+ the label is the visual click target.
const AIRPORT_LAYER_IDS = [
  "airport-dots",
  "airport-labels-code",
  "airport-labels-detail",
] as const;

// 30fps. Higher than the previous 15fps because the dirty-flag guard in
// interpolateFrame means we no longer pay the per-frame setData() cost
// during the idle gap between polls — only during the ~30s of active
// motion after each new snapshot. Smoother visible animation, same total
// CPU budget.
const FRAME_INTERVAL = 33;

/** Generate a filled circle as SDF-compatible ImageData */
function createCircleIcon(size: number): ImageData {
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const r = size / 2;
  ctx.beginPath();
  ctx.arc(r, r, r - 1, 0, Math.PI * 2);
  ctx.fillStyle = "#fff";
  ctx.fill();
  return ctx.getImageData(0, 0, size, size);
}

/** Generate a rounded square as SDF-compatible ImageData */
function createSquareIcon(size: number): ImageData {
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const pad = 1;
  const corner = size * 0.15;
  ctx.beginPath();
  ctx.roundRect(pad, pad, size - pad * 2, size - pad * 2, corner);
  ctx.fillStyle = "#fff";
  ctx.fill();
  return ctx.getImageData(0, 0, size, size);
}

/** Generate a delta-shaped plane silhouette as SDF ImageData. Drawn
 *  pointing up (north) so MapLibre's icon-rotate maps directly from the
 *  aircraft's true track in degrees. */
function createPlaneIcon(size: number): ImageData {
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const cx = size / 2;
  const cy = size / 2;
  ctx.fillStyle = "#fff";
  ctx.beginPath();
  // Nose, then sweep down and outward along each wing leading edge,
  // tuck back to the rear notch, and close — gives a fighter/glider look
  // that reads as a plane rather than a generic triangle at small sizes.
  ctx.moveTo(cx, cy - size * 0.42);              // nose
  ctx.lineTo(cx + size * 0.34, cy + size * 0.30); // right wingtip
  ctx.lineTo(cx + size * 0.08, cy + size * 0.18); // right inner trailing edge
  ctx.lineTo(cx + size * 0.08, cy + size * 0.42); // right tail
  ctx.lineTo(cx - size * 0.08, cy + size * 0.42); // left tail
  ctx.lineTo(cx - size * 0.08, cy + size * 0.18); // left inner trailing edge
  ctx.lineTo(cx - size * 0.34, cy + size * 0.30); // left wingtip
  ctx.closePath();
  ctx.fill();
  return ctx.getImageData(0, 0, size, size);
}

interface FeatureSnapshot {
  pos: [number, number];
  /** Geographic bearing in degrees (0=N, 90=E, 180=S, 270=W). Stored on
   *  every train feature by useTrainFeatures. */
  bearing: number;
}

/** Pick a feature out of the train GeoJSON by tripId. Returns its current
 *  animated position and bearing, or null if missing / non-Point. */
function findFeatureById(
  features: GeoJSON.Feature[],
  tripId: string,
): FeatureSnapshot | null {
  const f = features.find((feat) => feat.properties?.tripId === tripId);
  if (!f || f.geometry.type !== "Point") return null;
  const c = f.geometry.coordinates;
  const bearing = typeof f.properties?.bearing === "number" ? f.properties.bearing : 0;
  return { pos: [c[0], c[1]], bearing };
}

/** Position the popup overlay div directly via DOM (no React render). Each
 *  RAF frame:
 *   1. projects the train's lng/lat to screen pixels
 *   2. projects a point slightly ahead of the train along its bearing — the
 *      screen-space delta gives motion direction, automatically correct for
 *      map zoom and rotation
 *   3. computes perpendicular offset via popupOffsetPx, applies it to the
 *      train's screen position, and writes a transform on the overlay
 *
 *  Hides the overlay when the train is off-screen with a small buffer so it
 *  doesn't pop out abruptly when a train is half-off the edge. */
function positionPopupOverlay(
  el: HTMLDivElement,
  map: maplibregl.Map,
  feature: FeatureSnapshot,
): void {
  const trainScreen = map.project(feature.pos);
  const canvas = map.getCanvas();
  if (
    trainScreen.x < -120 || trainScreen.x > canvas.clientWidth + 120 ||
    trainScreen.y < -120 || trainScreen.y > canvas.clientHeight + 120
  ) {
    el.style.display = "none";
    return;
  }

  // Project a point a small distance ahead of the train along its bearing,
  // then take the screen-space delta as the motion vector. Doing perpendicular
  // math purely in lng/lat would be wrong at high zoom where Mercator distorts.
  const bearingRad = (feature.bearing * Math.PI) / 180;
  const aheadLng = feature.pos[0] + Math.sin(bearingRad) * POPUP_AHEAD_DEG;
  const aheadLat = feature.pos[1] + Math.cos(bearingRad) * POPUP_AHEAD_DEG;
  const aheadScreen = map.project([aheadLng, aheadLat]);
  const motion = { x: aheadScreen.x - trainScreen.x, y: aheadScreen.y - trainScreen.y };

  const offset = popupOffsetPx(motion, POPUP_OFFSET_PX);
  const popupX = trainScreen.x + offset.x;
  const popupY = trainScreen.y + offset.y;

  el.style.display = "block";
  // translate(-50%, -50%) centers the popup on (popupX, popupY); the pixel
  // translation comes first so it lands at the computed perpendicular offset.
  el.style.transform = `translate(${popupX}px, ${popupY}px) translate(-50%, -50%)`;
}

interface TransitMapProps {
  geojsonRef: React.MutableRefObject<GeoJSON.FeatureCollection>;
  /** Mutates the GeoJSON in `geojsonRef` and returns whether anything
   *  changed. Caller skips the `setData` upload when it returns false. */
  interpolateFrame: () => boolean;
  trains: TrainInfo[];
  routeShapes: RoutesGeoJSON | null;
  stops: StopsGeoJSON | null;
  planRoute: TripPlan | LirrTripPlan | null;
  /** When set, hide all non-plan route lines and pulse the plan outline. */
  planRouteIds: Set<string> | null;
  /** Active transit mode. Null when on the airspace view — transit layers
   *  (trains, routes, stops, plan highlight) are skipped entirely. */
  mode: Mode | null;
  /** Whether the filter panel is open. Auto-fit padding compensates for
   *  the visible panel: 320px on the left when open on desktop, 75vh
   *  bottom sheet on mobile. */
  panelOpen: boolean;
  /** Routes/stops still in flight. Drives the loading badge so a cold mode
   *  flip doesn't read as a frozen blank map. */
  routeShapesLoading: boolean;
  /** Live aircraft to render on the airspace overlay. Empty array when the
   *  overlay is toggled off, so the layer can mount unconditionally and
   *  the only branch is data presence. */
  aircraft: Aircraft[];
}

/** Popup placement constants. The popup sits perpendicular to the train's
 *  direction of travel (see popupOffsetPx) at a fixed pixel distance away,
 *  far enough that it doesn't visually merge with the marker but close
 *  enough that the relationship reads as "this popup is about that train".
 *  The pointer/tail extends from the popup edge toward the train. */
const POPUP_OFFSET_PX = 120;
/** How many degrees of bearing in lng/lat to project the "ahead" point used
 *  for screen-space motion direction. Small enough to be near the train
 *  position so map curvature doesn't matter. */
const POPUP_AHEAD_DEG = 0.001;

export function TransitMap({ geojsonRef, interpolateFrame, trains, routeShapes, stops, planRoute, planRouteIds, mode, panelOpen, routeShapesLoading, aircraft }: TransitMapProps) {
  const [popupTripId, setPopupTripId] = useState<string | null>(null);
  const [iconsReady, setIconsReady] = useState(false);
  const [followTripId, setFollowTripId] = useState<string | null>(null);
  const [popupAircraftHex, setPopupAircraftHex] = useState<string | null>(null);
  const [popupAirportIata, setPopupAirportIata] = useState<string | null>(null);
  const mapRef = useRef<MapRef>(null);
  const popupOverlayRef = useRef<HTMLDivElement>(null);
  const isMobile = useIsMobile();
  const viewportHeight = useViewportHeight();

  // Tracking ref so the RAF loop's stable closure can always read the latest
  // follow target without needing to re-bind the effect on every change.
  const followTripIdRef = useRef<string | null>(null);
  useEffect(() => {
    followTripIdRef.current = followTripId;
  }, [followTripId]);

  // Same pattern for popup tripId — the RAF loop needs to know which train
  // is being followed by the popup overlay each frame, but we don't want to
  // re-bind the entire animation effect every time the user opens or closes
  // the popup.
  const popupTripIdRef = useRef<string | null>(null);
  useEffect(() => {
    popupTripIdRef.current = popupTripId;
  }, [popupTripId]);

  // fitBounds padding compensates for whichever piece of UI covers the map.
  // The mobile bottom-sheet padding is computed from the live viewport
  // height so it scales correctly across iPhone SE (667h) → Pro Max (932h);
  // a fixed value would under-pad tall phones and hide the southern part
  // of the network behind the sheet.
  //
  // Read by closure inside the auto-fit useEffects — the eslint-disable
  // comments there are intentional. Toggle-driven re-fit is undesirable;
  // the next data change picks up the latest memoized padding.
  const fitPadding = useMemo(
    () => computeFitPadding({ isMobile, panelOpen, viewportHeight }),
    [isMobile, panelOpen, viewportHeight],
  );

  // Smoothed aircraft features. The hook does dead-reckoning between polls
  // (8s cadence is too slow for snap-each-poll motion to read as "flying"),
  // applies a snap-back lerp when each new poll arrives so the marker
  // glides into the corrected position instead of teleporting, and fades
  // out aircraft that drop off ADS-B coverage instead of yanking them off
  // the map. The geojsonRef is mutated in place by interpolateAircraftFrame
  // — the RAF loop pushes setData to the "aircraft" source each frame.
  const { aircraftGeojsonRef, interpolateAircraftFrame } = useAircraftFeatures(aircraft);

  // RAF loop — interpolates coordinates and pushes directly to MapLibre (no
  // React renders). When a train is followed, also re-center the camera on
  // it each frame so the user sees a smooth chase. setCenter (vs panTo) is
  // intentional: we already have a 15fps interpolated position and panTo's
  // own ease curve would fight that, producing visible stutter.
  useEffect(() => {
    let rafId = 0;
    let lastFrame = 0;

    const animate = () => {
      const now = Date.now();
      if (now - lastFrame >= FRAME_INTERVAL) {
        lastFrame = now;
        const map = mapRef.current?.getMap();
        // interpolateFrame returns false when the snapshot is fully consumed
        // and nothing's changed since last frame — skip the setData upload
        // AND the camera-follow recenter in that case. Big win: between
        // polls, RAF still fires but we no longer re-upload the full GeoJSON
        // to MapLibre 30 times per second. Camera-follow is also a no-op
        // when the followed train's coordinates didn't change this frame.
        if (interpolateFrame()) {
          const source = map?.getSource("trains");
          if (source && "setData" in source) {
            (source as { setData: (data: GeoJSON.FeatureCollection) => void }).setData(geojsonRef.current);
          }
          if (map) {
            const followId = followTripIdRef.current;
            const popupId = popupTripIdRef.current;
            // Follow target may differ from popup target — user can click
            // another train to inspect it without breaking an active follow.
            // Two lookups is fine; features array is typically <100 entries.
            if (followId) {
              const f = findFeatureById(geojsonRef.current.features, followId);
              if (f) map.setCenter(f.pos);
            }
            if (popupId && popupOverlayRef.current) {
              const f = findFeatureById(geojsonRef.current.features, popupId);
              if (f) positionPopupOverlay(popupOverlayRef.current, map, f);
              else popupOverlayRef.current.style.display = "none";
            }
          }
        }
        // Aircraft live on a separate animation loop with its own dirty
        // flag — dead-reckon advances the position every frame regardless
        // of whether trains needed an update. Returns false only when no
        // aircraft moved or faded since last frame (e.g. all parked + no
        // stale fades in flight), so we still avoid pointless setData
        // uploads on a quiet airspace view.
        if (interpolateAircraftFrame()) {
          const aircraftSource = map?.getSource("aircraft");
          if (aircraftSource && "setData" in aircraftSource) {
            (aircraftSource as { setData: (data: GeoJSON.FeatureCollection) => void }).setData(aircraftGeojsonRef.current);
          }
        }
      }
      rafId = requestAnimationFrame(animate);
    };

    rafId = requestAnimationFrame(animate);
    return () => cancelAnimationFrame(rafId);
  }, [geojsonRef, interpolateFrame, aircraftGeojsonRef, interpolateAircraftFrame]);

  // User dragging the map breaks follow. dragstart fires only on user input,
  // not on programmatic setCenter, so we don't have to debounce or filter.
  // Wheel/zoom intentionally don't break follow — user might want to zoom
  // in on the chased train.
  useEffect(() => {
    if (!followTripId) return;
    const map = mapRef.current?.getMap();
    if (!map) return;
    const stop = () => setFollowTripId(null);
    map.on("dragstart", stop);
    return () => {
      map.off("dragstart", stop);
    };
  }, [followTripId]);

  // Reposition the popup when the map view changes (zoom, pan, rotate, or
  // resize). Without this the popup detaches from the train during user
  // interaction: the train marker moves with the map (MapLibre handles
  // that), but our popup's screen coordinates are only updated inside the
  // RAF loop's `if (interpolateFrame())` guard — which returns false during
  // the idle gap between polls. So a user dragging the map between polls
  // would see the popup float over wrong territory until the next snapshot.
  // `move` fires on every viewport change including programmatic ones, and
  // is cheap to handle since we only do work when a popup is open.
  useEffect(() => {
    const map = mapRef.current?.getMap();
    if (!map) return;
    const reposition = () => {
      const popupId = popupTripIdRef.current;
      if (!popupId || !popupOverlayRef.current) return;
      const f = findFeatureById(geojsonRef.current.features, popupId);
      if (f) positionPopupOverlay(popupOverlayRef.current, map, f);
      else popupOverlayRef.current.style.display = "none";
    };
    map.on("move", reposition);
    return () => {
      map.off("move", reposition);
    };
  }, [geojsonRef]);

  // If the followed train falls out of the snapshot (5min stale eviction,
  // route filter, mode switch), clear follow so the camera doesn't lock to
  // the last known position forever.
  useEffect(() => {
    if (!followTripId) return;
    const stillThere = trains.some((t) => t.tripId === followTripId);
    if (!stillThere) setFollowTripId(null);
  }, [trains, followTripId]);

  const handleMapLoad = useCallback((e: { target: ReturnType<MapRef["getMap"]> }) => {
    const map = e.target;
    const size = 48;
    map.addImage("marker-circle", createCircleIcon(size), { sdf: true });
    map.addImage("marker-square", createSquareIcon(size), { sdf: true });
    map.addImage("marker-plane", createPlaneIcon(size), { sdf: true });
    setIconsReady(true);
  }, []);

  // Keep train layers on top of everything else. The trains <Source> is
  // unconditional and mounts at t=0, but routes/stops/plan-* sources are
  // conditional and mount later once their data arrives — MapLibre adds new
  // layers above existing ones, so without this re-promotion the route
  // lines, stations, and plan halos would render OVER the train markers
  // until the user reloaded the page.
  useEffect(() => {
    const map = mapRef.current?.getMap();
    if (!map) return;
    for (const id of ["aircraft-markers", "train-glow", "train-rim", "train-markers", "train-carets"]) {
      if (map.getLayer(id)) map.moveLayer(id);
    }
    // Aircraft layer ordering is stable once mounted — only the geometry
    // changes per poll, not the layer ID — so we don't add aircraftGeoJson
    // to the dep array. Re-promoting on data changes would still be correct
    // but would churn each 8s poll for no visible benefit.
  }, [routeShapes, stops, planRoute, iconsReady]);

  // Auto-fit the viewport to the active mode's network on mode switch (PT-507).
  // Subway → NYC; LIRR → Long Island. Without this, switching to LIRR leaves
  // the user staring at Manhattan with the network entirely off-screen.
  //
  // Deps are [routeShapes] alone — NOT [mode, routeShapes]. When the user
  // flips mode, React commits the new mode prop one render before
  // useRouteShapes can finish its setRouteShapes(null) reset, so for one
  // render `routeShapes` is the previous mode's data while `mode` is the new
  // one. With mode in the deps the effect would fire here and fit to the
  // OLD bbox under the NEW mode name — switching to LIRR would zoom the map
  // to NYC and vice versa. Excluding mode means the effect only fires when
  // routeShapes itself actually changes (after the reset, then again when
  // the new mode's data arrives), and the closure picks up the matching
  // mode at that moment.
  // Tracks the last mode the map auto-fit to. The "airspace" sentinel is
  // written when the user enters the airspace view so that LIRR→Airspace
  // →LIRR (and the Subway equivalent) still counts as a mode change and
  // re-fits to the transit bbox — without it, the ref would still equal
  // the returning mode and the map would stay on the airspace bbox.
  const lastFitMode = useRef<Mode | "airspace" | null>(null);
  useEffect(() => {
    if (!routeShapes || routeShapes.features.length === 0) return;
    if (mode === null) return; // airspace view handles its own fit below
    if (lastFitMode.current === null) {
      // First time we have routes — record this as the seed mode and let
      // initialViewState own the camera.
      lastFitMode.current = mode;
      return;
    }
    if (lastFitMode.current === mode) return;

    const map = mapRef.current?.getMap();
    if (!map) return;
    let minLon = Infinity, maxLon = -Infinity, minLat = Infinity, maxLat = -Infinity;
    for (const feature of routeShapes.features) {
      for (const [lon, lat] of feature.geometry.coordinates) {
        if (lon < minLon) minLon = lon;
        if (lon > maxLon) maxLon = lon;
        if (lat < minLat) minLat = lat;
        if (lat > maxLat) maxLat = lat;
      }
    }
    if (minLon === Infinity) return;
    map.fitBounds([[minLon, minLat], [maxLon, maxLat]], {
      padding: fitPadding,
      duration: 800,
      maxZoom: 12,
    });
    lastFitMode.current = mode;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mode + fitPadding intentionally excluded; only routeShapes drives fit
  }, [routeShapes]);

  // Auto-fit the airspace view to a fixed NYC-metro bbox roughly matching
  // the server-side 40 nm aircraft pull. Without this, switching to
  // airspace from a deep zoom on a single station leaves the user staring
  // at empty asphalt with no planes in frame. Only fires on the
  // transition INTO airspace; staying on airspace doesn't refit.
  const lastWasAirspace = useRef(false);
  useEffect(() => {
    const isAirspaceNow = mode === null;
    if (isAirspaceNow && !lastWasAirspace.current) {
      lastFitMode.current = "airspace";
      const map = mapRef.current?.getMap();
      if (map) {
        map.fitBounds(
          [[-74.5, 40.2], [-73.4, 41.1]],
          { padding: fitPadding, duration: 800, maxZoom: 11 },
        );
      }
    }
    lastWasAirspace.current = isAirspaceNow;
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mode is the only relevant trigger; fitPadding read at fit time
  }, [mode]);

  // Auto-fit the viewport to the planned route so users immediately see the
  // whole trip — fixes the case where one segment goes off-screen (e.g. an
  // L-line ride heading east into Brooklyn while the user is zoomed on
  // Manhattan). Padding compensates for the filter panel — see fitPadding
  // above.
  useEffect(() => {
    if (!planRoute) return;
    const map = mapRef.current?.getMap();
    if (!map) return;
    let minLon = Infinity, maxLon = -Infinity, minLat = Infinity, maxLat = -Infinity;
    for (const seg of planRoute.segments) {
      if (seg.type !== "ride") continue;
      for (const [lon, lat] of seg.path) {
        if (lon < minLon) minLon = lon;
        if (lon > maxLon) maxLon = lon;
        if (lat < minLat) minLat = lat;
        if (lat > maxLat) maxLat = lat;
      }
    }
    if (minLon === Infinity) return;
    map.fitBounds([[minLon, minLat], [maxLon, maxLat]], {
      padding: fitPadding,
      duration: 800,
      maxZoom: 14,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps -- only planRoute drives this fit; fitPadding read at fit time
  }, [planRoute]);

  // Pulse the plan-route-outline AND the plan-stop halos in lockstep when a
  // plan is active. Uses a sine wave on opacity + size so the white halo
  // "breathes". No-op when there's no plan; unsubscribes cleanly on change.
  useEffect(() => {
    if (!planRoute) return;
    const map = mapRef.current?.getMap();
    if (!map) return;
    let rafId = 0;
    const start = performance.now();
    const tick = () => {
      const t = (performance.now() - start) / 1000;
      const phase = (Math.sin(t * 1.8) + 1) / 2; // 0..1

      // Wide blurry outer halo
      const outerOpacity = 0.30 + 0.35 * phase; // 0.30..0.65
      const outerWidth = 16 + 8 * phase;        // 16..24
      if (map.getLayer("plan-route-outline")) {
        map.setPaintProperty("plan-route-outline", "line-opacity", outerOpacity);
        map.setPaintProperty("plan-route-outline", "line-width", outerWidth);
      }

      // Station halos — slightly stronger pulse so they stand out from line
      const stopOpacity = 0.25 + 0.40 * phase;       // 0.25..0.65
      const stopRadiusBase = 14 + 8 * phase;         // 14..22  (start/end)
      const stopRadiusTransfer = 10 + 6 * phase;     // 10..16  (transfer)
      if (map.getLayer("plan-stops-glow")) {
        map.setPaintProperty("plan-stops-glow", "circle-opacity", stopOpacity);
        map.setPaintProperty("plan-stops-glow", "circle-radius", [
          "match",
          ["get", "kind"],
          "transfer", stopRadiusTransfer,
          stopRadiusBase,
        ]);
      }

      rafId = requestAnimationFrame(tick);
    };
    rafId = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafId);
  }, [planRoute]);

  // Route shapes are always visible — toggles only control train visibility
  const allShapes = useMemo<GeoJSON.FeatureCollection | null>(() => {
    if (!routeShapes) return null;
    return { type: "FeatureCollection", features: routeShapes.features };
  }, [routeShapes]);

  // Build GeoJSON for planned route highlight. Each ride becomes a colored
  // line; between consecutive rides we add a short white "transfer connector"
  // that bridges the geometric gap where two perpendicular subway lines meet
  // at a transfer station (e.g. N going south meeting L going east at
  // 14 St-Union Sq). Without this the user sees a visual gap at every
  // transfer.
  const planGeoJson = useMemo<GeoJSON.FeatureCollection | null>(() => {
    if (!planRoute) return null;
    const rides = planRoute.segments.filter(
      (s): s is Extract<typeof s, { type: "ride" }> => s.type === "ride" && s.path.length >= 2,
    );
    if (rides.length === 0) return null;

    const features: GeoJSON.Feature[] = [];
    for (let i = 0; i < rides.length; i++) {
      const r = rides[i];
      features.push({
        type: "Feature",
        properties: {
          routeId: r.routeId,
          color: ROUTE_INFO[r.routeId]?.color ?? "#fff",
          isTransfer: false,
        },
        geometry: { type: "LineString", coordinates: r.path },
      });
      if (i < rides.length - 1) {
        const next = rides[i + 1];
        features.push({
          type: "Feature",
          properties: { routeId: "transfer", color: "#ffffff", isTransfer: true },
          geometry: {
            type: "LineString",
            coordinates: [r.path[r.path.length - 1], next.path[0]],
          },
        });
      }
    }
    return { type: "FeatureCollection", features };
  }, [planRoute]);

  // Plan key stations: start (first ride's board), end (last ride's alight),
  // transfers (each boundary between consecutive ride segments). We pull
  // coordinates straight from each segment's path geometry instead of looking
  // up by stopId — segments use platform-level IDs (e.g. "127S") which the
  // /api/stops payload (parent stops only) doesn't include.
  const planStopsGeoJson = useMemo<GeoJSON.FeatureCollection | null>(() => {
    if (!planRoute) return null;
    const rides = planRoute.segments.filter(
      (s): s is Extract<typeof s, { type: "ride" }> => s.type === "ride" && s.path.length >= 2,
    );
    if (rides.length === 0) return null;

    const features: GeoJSON.Feature[] = [];
    const firstRide = rides[0];
    const lastRide = rides[rides.length - 1];

    features.push({
      type: "Feature",
      properties: { kind: "start", stopName: firstRide.boardAt.stopName },
      geometry: { type: "Point", coordinates: firstRide.path[0] },
    });

    for (let i = 0; i < rides.length - 1; i++) {
      const ride = rides[i];
      features.push({
        type: "Feature",
        properties: { kind: "transfer", stopName: ride.alightAt.stopName },
        geometry: { type: "Point", coordinates: ride.path[ride.path.length - 1] },
      });
    }

    features.push({
      type: "Feature",
      properties: { kind: "end", stopName: lastRide.alightAt.stopName },
      geometry: { type: "Point", coordinates: lastRide.path[lastRide.path.length - 1] },
    });

    return { type: "FeatureCollection", features };
  }, [planRoute]);

  const handleClick = useCallback(
    (e: MapLayerMouseEvent) => {
      const feature = e.features?.[0];
      if (!feature || !feature.properties) {
        setPopupTripId(null);
        setPopupAircraftHex(null);
        setPopupAirportIata(null);
        return;
      }
      // Layer dispatch — aircraft markers, train markers, and airport
      // dots all flow through the same click event but live in different
      // layers and key off different properties.
      const layerId = feature.layer?.id;
      if (layerId === "aircraft-markers") {
        const hex = feature.properties.hex as string | undefined;
        if (hex) {
          setPopupAircraftHex(hex);
          setPopupTripId(null);
          setPopupAirportIata(null);
        }
        return;
      }
      // At airspace zoom 13+ the label is usually the click target, not
      // the dot — all three layers carry the same iata so any hit
      // dispatches identically. Membership check against the shared
      // AIRPORT_LAYER_IDS list keeps this in sync with interactiveLayerIds.
      if (layerId && (AIRPORT_LAYER_IDS as readonly string[]).includes(layerId)) {
        const iata = feature.properties.iata as string | undefined;
        if (iata) {
          setPopupAirportIata(iata);
          setPopupAircraftHex(null);
          setPopupTripId(null);
          // Clear any active train follow — the RAF loop would otherwise
          // keep recentering on the followed train and yank the camera
          // away from the airport the user just clicked.
          setFollowTripId(null);
        }
        return;
      }
      const tripId = feature.properties.tripId as string | undefined;
      if (!tripId) {
        setPopupTripId(null);
        return;
      }
      // Verify the train still exists in the snapshot before opening — guards
      // against a click event firing on a stale geojson feature whose train
      // has aged out via the 5min TTL filter between paint and click.
      if (trains.some((t) => t.tripId === tripId)) {
        setPopupTripId(tripId);
        setPopupAircraftHex(null);
        setPopupAirportIata(null);
      }
    },
    [trains],
  );

  // Look up the active popup's train data once per render. Reading from
  // `trains` (state) is fine here because popup CONTENT only changes on
  // poll cycles or filter toggles; per-frame position updates happen via
  // direct DOM in the RAF loop and don't go through React.
  const popupTrain = useMemo(
    () => (popupTripId ? trains.find((t) => t.tripId === popupTripId) ?? null : null),
    [popupTripId, trains],
  );

  // Same lookup for the active aircraft popup. Aircraft are keyed by hex,
  // which is stable across polls (it's the airframe's ICAO 24-bit address).
  const popupAircraft = useMemo(
    () => (popupAircraftHex ? aircraft.find((a) => a.hex === popupAircraftHex) ?? null : null),
    [popupAircraftHex, aircraft],
  );

  // Airport popup lookup. AIRPORTS is a static const so the find is cheap
  // (11 entries) and doesn't need to be in the dep array.
  const popupAirport = useMemo(
    () => (popupAirportIata ? AIRPORTS.find((a) => a.iata === popupAirportIata) ?? null : null),
    [popupAirportIata],
  );

  // If the popup's train falls out of the snapshot (TTL eviction, mode flip,
  // route filter), close the popup so it doesn't keep tracking a ghost.
  useEffect(() => {
    if (popupTripId && !popupTrain) setPopupTripId(null);
  }, [popupTripId, popupTrain]);

  // Same for aircraft: if the plane leaves the bbox or the overlay is
  // toggled off, close any open popup.
  useEffect(() => {
    if (popupAircraftHex && !popupAircraft) setPopupAircraftHex(null);
  }, [popupAircraftHex, popupAircraft]);

  // Close train-only state when entering the airspace tab, and the
  // airport popup when leaving it. Without the airspace clear the RAF
  // loop would keep recentering on a followed train (whose layer has
  // unmounted), and a stale train popup would float over empty space.
  useEffect(() => {
    if (mode === null) {
      setFollowTripId(null);
      setPopupTripId(null);
      return;
    }
    if (popupAirportIata) setPopupAirportIata(null);
  }, [mode, popupAirportIata]);

  return (
    <>
    {routeShapesLoading && mode !== null && <MapLoadingBadge mode={mode} />}
    <Map
      ref={mapRef}
      initialViewState={NYC_CENTER}
      style={{ width: "100%", height: "100%" }}
      mapStyle={BASEMAP}
      interactiveLayerIds={["train-markers", "aircraft-markers", ...AIRPORT_LAYER_IDS]}
      onClick={handleClick}
      onLoad={handleMapLoad}
      cursor="pointer"
    >
      {/* Route lines — when a plan is active, hide every line that isn't on
          one of the planned routes so the user's chosen path stands alone.
          LIRR rails render slightly thicker than subway to match the
          commuter-rail convention and because LIRR's much-larger geographic
          spread means each line renders at lower screen-space density. */}
      {mode !== null && allShapes && (
        <Source id="routes" type="geojson" data={allShapes}>
          <Layer
            id="route-lines"
            type="line"
            filter={planRouteIds
              ? ["in", ["get", "routeId"], ["literal", Array.from(planRouteIds)]]
              : ["literal", true]
            }
            paint={{
              "line-color": ["get", "color"],
              "line-width": mode === "lirr" ? 3.5 : 2.5,
              "line-opacity": mode === "lirr" ? 0.85 : 0.6,
            }}
          />
        </Source>
      )}

      {/* Planned route highlight — outline + colored line. Outline pulses
          (driven by the RAF effect above). The colored line is bumped to
          width 6 and uses a line-cap of "round" so adjacent segments meet
          cleanly through transfer points. */}
      {mode !== null && planGeoJson && (
        <Source id="plan-route" type="geojson" data={planGeoJson}>
          {/* Wide soft white glow underneath — visible even when the route's
              own color is low-contrast against the dark map (e.g. L grey). */}
          <Layer
            id="plan-route-outline"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": "#ffffff",
              "line-width": 18,
              "line-opacity": 0.35,
              "line-blur": 4,
            }}
          />
          {/* Inner white core — guarantees the path is readable regardless of
              the route's color. The colored layer on top tints it. */}
          <Layer
            id="plan-route-core"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": "#ffffff",
              "line-width": 8,
              "line-opacity": 0.85,
            }}
          />
          <Layer
            id="plan-route-line"
            type="line"
            layout={{ "line-cap": "round", "line-join": "round" }}
            paint={{
              "line-color": ["get", "color"],
              "line-width": 5,
              "line-opacity": 1,
            }}
          />
        </Source>
      )}

      {/* Stations */}
      {mode !== null && stops && (
        <Source id="stops" type="geojson" data={stops}>
          {/* Station markers — radius scales with serving-route count so
              major hubs stand out. Every dot gets a dark halo so the white
              core punches through against colored route lines beneath. */}
          <Layer
            id="station-dots"
            type="circle"
            paint={{
              // Dot size + halo scale with the API-emitted `importance`
              // bucket (0/1/2). Per-mode thresholds live server-side so
              // subway's "8+ routes = hub" logic doesn't dictate LIRR
              // (where Jamaica + Penn matter regardless of count).
              //
              // `coalesce` defaults missing `importance` to 0 — the field
              // was added in this PR and /api/<mode>/stops is cached for
              // 24h; any client with a pre-deploy cached payload would
              // otherwise see every == check evaluate false and render
              // every station at the smallest size.
              "circle-radius": [
                "interpolate", ["linear"], ["zoom"],
                11, [
                  "case",
                  ["==", ["coalesce", ["get", "importance"], 0], 2], 4,
                  ["==", ["coalesce", ["get", "importance"], 0], 1], 2.5,
                  2,
                ],
                14, [
                  "case",
                  ["==", ["coalesce", ["get", "importance"], 0], 2], 7,
                  ["==", ["coalesce", ["get", "importance"], 0], 1], 5,
                  4,
                ],
                16, [
                  "case",
                  ["==", ["coalesce", ["get", "importance"], 0], 2], 9,
                  ["==", ["coalesce", ["get", "importance"], 0], 1], 7,
                  6,
                ],
              ],
              "circle-color": "#ffffff",
              "circle-opacity": [
                "interpolate", ["linear"], ["zoom"],
                11, 0.85,
                13, 1,
              ],
              "circle-stroke-width": [
                "interpolate", ["linear"], ["zoom"],
                11, [
                  "case",
                  ["==", ["coalesce", ["get", "importance"], 0], 2], 2,
                  ["==", ["coalesce", ["get", "importance"], 0], 1], 1.5,
                  1,
                ],
                14, [
                  "case",
                  ["==", ["coalesce", ["get", "importance"], 0], 2], 2.5,
                  ["==", ["coalesce", ["get", "importance"], 0], 1], 1.8,
                  1.2,
                ],
              ],
              // Dark halo for normal stations (separates from colored routes);
              // bright halo for hubs to make them visually "premium".
              "circle-stroke-color": [
                "case",
                ["==", ["coalesce", ["get", "importance"], 0], 2], "#ffffff",
                "#0a0a1a",
              ],
              "circle-stroke-opacity": 1,
            }}
            minzoom={11}
          />

          {/* Major-hub labels — shown earlier so the map is readable at
              default zoom. Filter on importance >= 1: subway suppresses
              local stops at this zoom (avoids clutter); LIRR shows every
              station because its network is geographically sparse and 127
              stations spread over 100+ miles need labels at wider zooms. */}
          <Layer
            id="station-labels-major"
            type="symbol"
            minzoom={12}
            maxzoom={14}
            filter={[">=", ["coalesce", ["get", "importance"], 0], 1]}
            layout={{
              "text-field": ["get", "stopName"],
              "text-size": [
                "interpolate", ["linear"], ["zoom"],
                12, 9,
                14, 11,
              ],
              "text-font": ["Open Sans Regular", "Arial Unicode MS Regular"],
              "text-offset": [0, 1.0],
              "text-anchor": "top",
              "text-max-width": 8,
              "text-optional": true,
              "symbol-sort-key": ["-", 0, ["coalesce", ["get", "importance"], 0]],
            }}
            paint={{
              "text-color": "#d8d8e0",
              "text-halo-color": "#1a1a2e",
              "text-halo-width": 1.5,
              "text-opacity": [
                "interpolate", ["linear"], ["zoom"],
                12, 0.5,
                14, 0.85,
              ],
            }}
          />

          {/* Detailed labels — at zoom 14+, every station shows its name plus
              the routes serving it (PT-203). Sorted by importance so hubs win
              collision contests. */}
          <Layer
            id="station-labels-detailed"
            type="symbol"
            minzoom={14}
            layout={{
              "text-field": ["get", "labelText"],
              "text-size": [
                "interpolate", ["linear"], ["zoom"],
                14, 10,
                16, 12,
              ],
              "text-font": ["Open Sans Regular", "Arial Unicode MS Regular"],
              "text-offset": [0, 1.2],
              "text-anchor": "top",
              "text-max-width": 10,
              "text-optional": true,
              "symbol-sort-key": ["-", 0, ["coalesce", ["get", "importance"], 0]],
            }}
            paint={{
              "text-color": "#d8d8e0",
              "text-halo-color": "#1a1a2e",
              "text-halo-width": 1.5,
              "text-opacity": [
                "interpolate", ["linear"], ["zoom"],
                14, 0.7,
                16, 1,
              ],
            }}
          />
        </Source>
      )}

      {/* Plan key stations — pulsing halo + solid center on the start, end,
          and any transfer points along the active plan. Pulse params live
          in the RAF effect above. */}
      {mode !== null && planStopsGeoJson && (
        <Source id="plan-stops" type="geojson" data={planStopsGeoJson}>
          <Layer
            id="plan-stops-glow"
            type="circle"
            paint={{
              "circle-radius": 18,
              "circle-color": "#ffffff",
              "circle-opacity": 0.4,
              "circle-blur": 0.6,
            }}
          />
          <Layer
            id="plan-stops-center"
            type="circle"
            paint={{
              "circle-radius": [
                "match",
                ["get", "kind"],
                "transfer", 4,
                6,
              ],
              "circle-color": "#ffffff",
              "circle-opacity": 1,
              "circle-stroke-width": [
                "match",
                ["get", "kind"],
                "transfer", 1.5,
                2.5,
              ],
              "circle-stroke-color": [
                "match",
                ["get", "kind"],
                "transfer", "#cbd5e1",
                "#0a0a1a",
              ],
            }}
          />
        </Source>
      )}

      {/* Aircraft (airspace overlay). Empty FeatureCollection when the
          overlay is toggled off so we don't conditionally mount the layer
          and pay the addLayer/removeLayer churn on each toggle. The
          re-promote effect ordering keeps this layer below train markers
          so trains stay legible on top during the most common view
          (zoomed in on Manhattan).

          Declared BEFORE the airports block on purpose: airport layers
          use `beforeId="aircraft-markers"`, and MapLibre's addLayer
          throws when that reference doesn't exist yet. react-map-gl
          processes JSX children top-down in a single commit, so
          aircraft-markers must appear first or the airport mount fails. */}
      {iconsReady && (
        <Source id="aircraft" type="geojson" data={aircraftGeojsonRef.current}>
          <Layer
            id="aircraft-markers"
            type="symbol"
            layout={{
              "icon-image": "marker-plane",
              "icon-size": [
                "interpolate", ["linear"], ["zoom"],
                9, 0.35,
                14, 0.55,
              ],
              "icon-rotate": ["get", "track"],
              "icon-rotation-alignment": "map",
              "icon-allow-overlap": true,
              "icon-ignore-placement": true,
            }}
            paint={{
              "icon-color": [
                "match",
                ["get", "kind"],
                "helicopter", "#fb923c", // orange — visually distinct from jet
                "fixed-wing", "#fef08a", // pale yellow — readable on dark basemap
                "#cbd5e1",                // slate — unknown / surface
              ],
              // Multiplied by per-feature opacity so stale aircraft (off
              // ADS-B coverage) fade out gracefully rather than vanish.
              "icon-opacity": ["*", 0.9, ["get", "opacity"]],
              "icon-halo-color": "#0a0a1a",
              "icon-halo-width": 1,
            }}
          />
        </Source>
      )}

      {/* Airports — the airspace equivalent of transit stations. Mirrors
          the importance-bucket pattern (2=hubs JFK/LGA/EWR, 1=major
          secondaries, 0=regional GA): hubs visible earliest, regional only
          labeled at high zoom. Bigger dots than transit stations because
          there are far fewer of them.

          Each airport layer uses `beforeId="aircraft-markers"` so it
          renders below the planes. That reference requires
          aircraft-markers to already be mounted — see the comment on
          the aircraft <Source> above for why this block must stay AFTER
          aircraft in JSX order. */}
      {mode === null && iconsReady && (
        <Source id="airports" type="geojson" data={AIRPORTS_GEOJSON}>
          <Layer
            id="airport-dots"
            beforeId="aircraft-markers"
            type="circle"
            paint={{
              "circle-radius": [
                "interpolate", ["linear"], ["zoom"],
                9, [
                  "case",
                  ["==", ["get", "importance"], 2], 6,
                  ["==", ["get", "importance"], 1], 4,
                  3,
                ],
                12, [
                  "case",
                  ["==", ["get", "importance"], 2], 9,
                  ["==", ["get", "importance"], 1], 6,
                  4,
                ],
                14, [
                  "case",
                  ["==", ["get", "importance"], 2], 12,
                  ["==", ["get", "importance"], 1], 8,
                  5,
                ],
              ],
              "circle-color": "#ffffff",
              "circle-opacity": 0.95,
              "circle-stroke-width": [
                "case",
                ["==", ["get", "importance"], 2], 2.5,
                ["==", ["get", "importance"], 1], 2,
                1.5,
              ],
              // Hubs get a bright halo to read as "premium"; secondaries
              // and regional fields get the dark halo that punches the
              // white core out against colored aircraft / dark basemap.
              "circle-stroke-color": [
                "case",
                ["==", ["get", "importance"], 2], "#ffffff",
                "#0a0a1a",
              ],
              "circle-stroke-opacity": 1,
            }}
          />
          {/* Code-only labels at low zoom — IATA is universal, fits in a
              tight footprint, and is what most users recognize. Filter
              importance >= 1 so the regional GA fields don't clutter the
              wide airspace view (default zoom 11). */}
          <Layer
            id="airport-labels-code"
            beforeId="aircraft-markers"
            type="symbol"
            maxzoom={13}
            filter={[">=", ["get", "importance"], 1]}
            layout={{
              "text-field": ["get", "iata"],
              "text-size": [
                "interpolate", ["linear"], ["zoom"],
                9, 10,
                12, 13,
              ],
              "text-font": ["Open Sans Bold", "Arial Unicode MS Bold"],
              "text-offset": [0, 1.2],
              "text-anchor": "top",
              "text-optional": true,
              "symbol-sort-key": ["-", 0, ["get", "importance"]],
            }}
            paint={{
              "text-color": "#e2e8f0",
              "text-halo-color": "#0a0a1a",
              "text-halo-width": 2,
            }}
          />
          {/* Detailed labels at zoom 13+ — every airport gets full name
              ("JFK · John F. Kennedy"). Sort key prefers hubs in collision
              contests so a regional field never wins a layout fight against
              a major hub at marginal zooms. */}
          <Layer
            id="airport-labels-detail"
            beforeId="aircraft-markers"
            type="symbol"
            minzoom={13}
            layout={{
              "text-field": ["concat", ["get", "iata"], " · ", ["get", "name"]],
              "text-size": [
                "interpolate", ["linear"], ["zoom"],
                13, 11,
                16, 13,
              ],
              "text-font": ["Open Sans Bold", "Arial Unicode MS Bold"],
              "text-offset": [0, 1.4],
              "text-anchor": "top",
              "text-max-width": 12,
              "text-optional": true,
              "symbol-sort-key": ["-", 0, ["get", "importance"]],
            }}
            paint={{
              "text-color": "#e2e8f0",
              "text-halo-color": "#0a0a1a",
              "text-halo-width": 2,
            }}
          />
        </Source>
      )}

      {/* Train markers — data pushed by RAF loop via source.setData(). The
          source is declared last so its layers render on top of routes,
          stops, and plan highlights. Mounted only when a transit mode is
          active so the airspace view doesn't carry train-layer overhead. */}
      {mode !== null && (
      <Source id="trains" type="geojson" data={geojsonRef.current}>
        {/* Soft outer glow */}
        <Layer
          id="train-glow"
          type="circle"
          paint={{
            "circle-radius": 14,
            "circle-color": ["get", "color"],
            "circle-opacity": ["*", 0.2, ["get", "opacity"]],
            "circle-blur": 1,
          }}
        />
        {/* Dark rim — same SDF icons as the marker but slightly larger and
            dark-tinted, drawn underneath. The 0.55-vs-0.50 size delta shows
            as a 1-2px dark ring around the colored bullet so trains stand
            out against same-colored route lines beneath them. We use a
            symbol layer (not circle) because circle-translate doesn't
            support data-driven expressions, so cluster fan-out can only be
            mirrored via icon-offset.
            icon-offset is multiplied by icon-size for the final screen
            offset, so the rim's offset values are scaled down to match the
            marker's screen position: 78px / 0.55 ≈ 142. */}
        {iconsReady && (
          <Layer
            id="train-rim"
            type="symbol"
            layout={{
              "icon-image": ["get", "iconImage"],
              "icon-size": 0.55,
              "icon-allow-overlap": true,
              "icon-ignore-placement": true,
              "icon-offset": [
                "interpolate", ["linear"], ["get", "clusterOffset"],
                -3, ["literal", [-142, 0]],
                0, ["literal", [0, 0]],
                3, ["literal", [142, 0]],
              ],
            }}
            paint={{
              "icon-color": "#0a0a1a",
              "icon-opacity": ["*", 0.95, ["get", "opacity"]],
            }}
          />
        )}
        {/* Route bullet — colored shape with route label inside. Both
            iconImage (circle/square) and label come from per-mode feature
            properties so the layer config is mode-agnostic. */}
        {iconsReady && (
          <Layer
            id="train-markers"
            type="symbol"
            layout={{
              "icon-image": ["get", "iconImage"],
              "icon-size": 0.5,
              "icon-allow-overlap": true,
              "icon-ignore-placement": true,
              "icon-offset": [
                "interpolate", ["linear"], ["get", "clusterOffset"],
                -3, ["literal", [-156, 0]],
                0, ["literal", [0, 0]],
                3, ["literal", [156, 0]],
              ],
              "text-field": ["get", "label"],
              "text-size": 11,
              "text-font": ["Open Sans Bold", "Arial Unicode MS Bold"],
              "text-allow-overlap": true,
              "text-ignore-placement": true,
              "text-offset": [
                "interpolate", ["linear"], ["get", "clusterOffset"],
                -3, ["literal", [-7.09, 0]],
                0, ["literal", [0, 0]],
                3, ["literal", [7.09, 0]],
              ],
            }}
            paint={{
              "icon-color": ["get", "color"],
              "icon-opacity": ["get", "opacity"],
              "text-color": ["get", "textColor"],
              "text-opacity": ["get", "opacity"],
              // Halo around the route number so it stays readable even if
              // the icon is rendered against a same-color route line.
              "text-halo-color": "#0a0a1a",
              "text-halo-width": 1,
            }}
          />
        )}
        {/* Direction carets — rotated by actual bearing along the track */}
        <Layer
          id="train-carets"
          type="symbol"
          layout={{
            "text-field": "▲",
            "text-size": 10,
            "text-offset": [
              "interpolate", ["linear"], ["get", "clusterOffset"],
              -3, ["literal", [-7.8, -1.5]],
              0, ["literal", [0, -1.5]],
              3, ["literal", [7.8, -1.5]],
            ],
            "text-rotate": ["get", "bearing"],
            "text-rotation-alignment": "map",
            "text-allow-overlap": true,
            "text-ignore-placement": true,
          }}
          paint={{
            "text-color": "#ffffff",
            "text-opacity": ["*", 0.85, ["get", "opacity"]],
          }}
        />
      </Source>
      )}

      {/* Aircraft popup uses react-map-gl's Popup which auto-positions
          relative to the given lng/lat — much simpler than the train
          popup's per-frame DOM dance, and aircraft don't need follow
          mode or sub-poll motion smoothing anyway. */}
      {popupAircraft && (
        <AircraftPopup
          aircraft={popupAircraft}
          onClose={() => setPopupAircraftHex(null)}
        />
      )}

      {/* Airport briefing popup — frequencies, runways, elevation. Same
          react-map-gl Popup pattern as aircraft since airport position is
          static (no per-frame repositioning needed). */}
      {popupAirport && (
        <AirportPopup
          airport={popupAirport}
          onClose={() => setPopupAirportIata(null)}
        />
      )}

    </Map>
    {/* Popup is a sibling of <Map>, not a child — it's a custom HTML
        overlay (not react-map-gl's Popup) so we can update its position
        per-frame via direct DOM mutation in the RAF loop without React
        re-rendering the popup contents 30 times per second. */}
    {popupTrain && (
      <TrainPopup
        ref={popupOverlayRef}
        train={popupTrain}
        following={followTripId === popupTrain.tripId}
        onClose={() => {
          setPopupTripId(null);
          // Closing the popup is an explicit "I'm done with this train"
          // signal — stop following too, otherwise the camera keeps
          // chasing a train the user can't see details on anymore.
          setFollowTripId(null);
        }}
        onToggleFollow={() =>
          setFollowTripId(followTripId === popupTrain.tripId ? null : popupTrain.tripId)
        }
      />
    )}
    </>
  );
}
