import nearestPointOnLine from "@turf/nearest-point-on-line";
import along from "@turf/along";
import turfLength from "@turf/length";
import { lineString } from "@turf/helpers";
import type { RoutesGeoJSON } from "@panoptrain/shared";

interface ShapeData {
  line: ReturnType<typeof lineString>;
  totalLength: number;
  id: number; // unique index for cache keying
}

export interface TrackPath {
  shape: ShapeData;
  startDist: number;
  endDist: number;
}

// Cache nearestPointOnLine results — trains follow repeatable routes,
// so the same position on the same shape yields the same distance.
// Key: "shapeId:roundedLon,roundedLat" → distance along shape
const snapCache = new Map<string, number>();
const SNAP_GRID = 0.0001; // ~11m resolution — fine enough for smooth animation
const MAX_CACHE_SIZE = 5000;

function cachedSnap(shape: ShapeData, pos: [number, number]): number {
  const key = `${shape.id}:${Math.round(pos[0] / SNAP_GRID)},${Math.round(pos[1] / SNAP_GRID)}`;
  const cached = snapCache.get(key);
  if (cached !== undefined) return cached;

  const snap = nearestPointOnLine(shape.line, pos);
  const dist = snap.properties.location ?? 0;

  // Evict oldest entries if cache is full
  if (snapCache.size >= MAX_CACHE_SIZE) {
    const first = snapCache.keys().next().value!;
    snapCache.delete(first);
  }
  snapCache.set(key, dist);
  return dist;
}

// Cache which shape is best for a given route + position grid cell
const bestShapeCache = new Map<string, ShapeData | null>();

function cachedBestShape(
  shapes: ShapeData[],
  routeId: string,
  pos: [number, number],
): { shape: ShapeData | null; tooFar: boolean } {
  const key = `${routeId}:${Math.round(pos[0] / SNAP_GRID)},${Math.round(pos[1] / SNAP_GRID)}`;
  if (bestShapeCache.has(key)) {
    return { shape: bestShapeCache.get(key)!, tooFar: false };
  }

  let best: ShapeData | null = null;
  let bestDist = Infinity;
  for (const shape of shapes) {
    const snap = nearestPointOnLine(shape.line, pos);
    const dist = snap.properties.dist ?? Infinity;
    if (dist < bestDist) {
      bestDist = dist;
      best = shape;
    }
  }

  const result = bestDist <= 0.5 ? best : null; // > 500m → no match
  if (bestShapeCache.size >= MAX_CACHE_SIZE) {
    const first = bestShapeCache.keys().next().value!;
    bestShapeCache.delete(first);
  }
  bestShapeCache.set(key, result);
  return { shape: result, tooFar: bestDist > 0.5 };
}

// Memoize indexes per routes payload. WeakMap-keyed by the cached routes
// object (from modeCache), so subway → LIRR → subway re-uses the original
// subway index instead of rebuilding it. The cached routes object is
// stable across remounts of useRouteShapes.
const indexByRoutes = new WeakMap<RoutesGeoJSON, Record<string, ShapeData[]>>();

// Globally-unique shape IDs. Per-build counters would collide across the
// snap/bestShape caches when two indexes are alive simultaneously
// (subway + LIRR after the first tab switch). With a global counter we
// can keep both caches warm without the per-build clear that defeated
// the purpose of memoizing indexes in the first place.
let shapeIdCounter = 0;

/**
 * Index route shapes by routeId for quick lookup.
 * Each route may have multiple shapes (directions/branches).
 */
export function buildShapeIndex(routes: RoutesGeoJSON): Record<string, ShapeData[]> {
  const cached = indexByRoutes.get(routes);
  if (cached) return cached;

  // bestShapeCache values are direct ShapeData references; its keys are
  // `${routeId}:gridCell`. snapCache values are numeric distances under
  // globally-unique shapeId keys, so it stays warm across builds. But
  // routeIds can collide between subway and LIRR (per the documented
  // numeric-routeId overlap), and a stale ShapeData ref would route a
  // LIRR train through the matching subway shape's geometry. Clear here
  // — it'll re-warm naturally as cachedBestShape is exercised.
  bestShapeCache.clear();

  const index: Record<string, ShapeData[]> = {};
  for (const feature of routes.features) {
    const routeId = feature.properties.routeId;
    if (!index[routeId]) index[routeId] = [];
    const coords = feature.geometry.coordinates;
    if (coords.length < 2) continue;
    const line = lineString(coords);
    index[routeId].push({ line, totalLength: turfLength(line), id: shapeIdCounter++ });
  }
  // Schedule a snap-cache prewarm for idle time. Without it the first poll's
  // hundreds of trains all hit cold caches and stall the main thread on Turf
  // nearestPointOnLine. Idle scheduling keeps it off the first-paint path.
  scheduleIdle(() => prewarmTrackCaches(index));
  indexByRoutes.set(routes, index);
  return index;
}

/** Test helper — clear the snap/bestShape caches so each test starts
 *  from a known empty state. The index WeakMap doesn't need explicit
 *  clearing in tests because each test creates fresh routes objects
 *  (always cache misses). Production code shouldn't call this; LRU
 *  eviction handles cache growth and the WeakMap drops indexes when
 *  the routes object is GC'd. */
export function _resetTrackCachesForTests(): void {
  snapCache.clear();
  bestShapeCache.clear();
}

// Idle scheduler — fires the callback when the browser is idle, falls
// back to setTimeout where requestIdleCallback isn't available.
//
// No cancellation here. The previous single-slot cancel-and-replace
// existed to defend against shape-id reuse across rebuilds (each build
// reset shapeId to 0, so a stale prewarm could poison snapCache with
// distances under IDs the new index had reused). With shapeIdCounter
// now globally unique, that hazard is gone — and cancelling actively
// hurts: a user who flips subway → LIRR before subway's prewarm runs
// would lose subway's prewarm permanently, since the WeakMap-cached
// subway index returns instantly on re-entry without rescheduling.
const hasIdleApi = typeof globalThis.requestIdleCallback === "function";

function scheduleIdle(cb: () => void): void {
  if (hasIdleApi) {
    globalThis.requestIdleCallback!(cb);
    return;
  }
  setTimeout(cb, 1);
}

/** Expose snap/bestShape cache sizes for diagnostics and tests. */
export function getTrackCacheSizes(): { snap: number; bestShape: number } {
  return { snap: snapCache.size, bestShape: bestShapeCache.size };
}

/**
 * Prime `snapCache` by walking each shape's coordinates and recording the
 * distance-along-shape for grid cells the train will actually traverse.
 *
 * Only seeds snap (cheap, one Turf call per sample) — bestShape warms
 * naturally as trains repeat through the same grid cells. Bails out if the
 * cache fills so we don't displace warm entries with cold ones.
 */
export function prewarmTrackCaches(shapeIndex: Record<string, ShapeData[]>): void {
  const COORD_STRIDE = 5; // sample every 5th coord; finer detail fills in via live snaps
  for (const shapes of Object.values(shapeIndex)) {
    for (const shape of shapes) {
      const coords = shape.line.geometry.coordinates;
      for (let i = 0; i < coords.length; i += COORD_STRIDE) {
        const pos = coords[i] as [number, number];
        cachedSnap(shape, pos);
        if (snapCache.size >= MAX_CACHE_SIZE) return;
      }
    }
  }
}

/**
 * Find the track path between two positions on a route's shape.
 * Returns the shape and start/end distances, or null if no shape matches.
 */
export function findTrackPath(
  shapeIndex: Record<string, ShapeData[]>,
  routeId: string,
  prevPos: [number, number],
  currPos: [number, number],
): TrackPath | null {
  const shapes = shapeIndex[routeId];
  if (!shapes || shapes.length === 0) return null;

  const { shape, tooFar } = cachedBestShape(shapes, routeId, currPos);
  if (!shape || tooFar) return null;

  const startDist = cachedSnap(shape, prevPos);
  const endDist = cachedSnap(shape, currPos);

  return { shape, startDist, endDist };
}

/**
 * Interpolate a position along a track path at a given fraction.
 */
export function interpolateAlongPath(
  path: TrackPath,
  fraction: number,
): [number, number] {
  const dist = path.startDist + (path.endDist - path.startDist) * fraction;
  const clamped = Math.max(0, Math.min(path.shape.totalLength, dist));
  const pt = along(path.shape.line, clamped);
  return pt.geometry.coordinates as [number, number];
}
