import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type { RoutesGeoJSON } from "@panoptrain/shared";
import {
  _resetTrackCachesForTests,
  buildShapeIndex,
  prewarmTrackCaches,
  getTrackCacheSizes,
  findTrackPath,
} from "../trackInterpolation.js";

/**
 * `prewarmTrackCaches` exists to seed `snapCache` with grid cells the train
 * will actually traverse, so the first poll's hundreds of trains don't all
 * stall the main thread on Turf nearestPointOnLine. Asserting on cache size
 * is the only behavioral signal — the cache is otherwise opaque — so the
 * tests below verify (a) prewarm populates it, (b) it tolerates degenerate
 * shapes, (c) it respects the cache-size cap.
 */
function makeRoutes(features: Array<{ routeId: string; coords: [number, number][] }>): RoutesGeoJSON {
  return {
    type: "FeatureCollection",
    features: features.map(({ routeId, coords }) => ({
      type: "Feature",
      properties: { routeId, color: "888888", name: routeId },
      geometry: { type: "LineString", coordinates: coords },
    })),
  };
}

// Use requestIdleCallback's polyfill path: jsdom doesn't define it, so
// `buildShapeIndex` already routes through setTimeout. Stub setTimeout so
// the prewarm runs synchronously inside this test. Cache reset is needed
// now that buildShapeIndex no longer clears snap/bestShape caches itself
// (globally-unique shape IDs make per-build clears unnecessary in prod
// but tests need explicit isolation).
beforeEach(() => {
  vi.unstubAllGlobals();
  vi.stubGlobal("setTimeout", ((fn: () => void) => {
    fn();
    return 0;
  }) as unknown as typeof setTimeout);
  _resetTrackCachesForTests();
});

// Restore real timers so later tests in the same worker don't inherit
// the synchronous setTimeout stub (vitest reuses workers across files).
afterEach(() => {
  vi.unstubAllGlobals();
});

describe("prewarmTrackCaches", () => {
  it("populates snapCache when buildShapeIndex schedules it", () => {
    // 50-coord straight line; with COORD_STRIDE=5 we expect 10 samples seeded.
    const coords: [number, number][] = Array.from({ length: 50 }, (_, i) => [-74 + i * 0.001, 40.7]);
    buildShapeIndex(makeRoutes([{ routeId: "1", coords }]));
    const { snap } = getTrackCacheSizes();
    expect(snap).toBeGreaterThanOrEqual(10);
  });

  it("seeds entries that subsequent findTrackPath calls reuse", () => {
    const coords: [number, number][] = Array.from({ length: 20 }, (_, i) => [-74 + i * 0.001, 40.7]);
    const index = buildShapeIndex(makeRoutes([{ routeId: "A", coords }]));
    const before = getTrackCacheSizes().snap;
    // A position that maps onto the line — same shape, different coords would
    // also hit if they're in a previously-sampled grid cell.
    const path = findTrackPath(index, "A", coords[5]!, coords[10]!);
    expect(path).not.toBeNull();
    // Lookups for already-sampled grid cells should not grow the cache.
    expect(getTrackCacheSizes().snap).toBe(before);
  });

  it("schedules independent prewarms when buildShapeIndex runs twice before idle fires", () => {
    // The earlier shape-id-reset poisoning hazard required cancelling the
    // pending prewarm before scheduling a new one. With shapeIdCounter
    // now globally unique, both prewarms run safely back-to-back, and
    // dropping the cancellation also fixes the WeakMap-cached re-entry
    // path (a cancelled-but-never-rescheduled prewarm leaves the first
    // mode permanently cold).
    const queued: Array<() => void> = [];
    const cleared: number[] = [];
    vi.stubGlobal("setTimeout", ((fn: () => void) => {
      queued.push(fn);
      return queued.length;
    }) as unknown as typeof setTimeout);
    vi.stubGlobal("clearTimeout", ((id: number) => { cleared.push(id); }) as unknown as typeof clearTimeout);

    const coords1: [number, number][] = Array.from({ length: 30 }, (_, i) => [-74 + i * 0.001, 40.7]);
    buildShapeIndex(makeRoutes([{ routeId: "A", coords: coords1 }]));
    expect(queued).toHaveLength(1);

    // Mode flip / second load before the first idle fires — second
    // prewarm queues alongside the first; neither is cancelled.
    const coords2: [number, number][] = Array.from({ length: 30 }, (_, i) => [-73 + i * 0.001, 40.8]);
    buildShapeIndex(makeRoutes([{ routeId: "B", coords: coords2 }]));

    expect(cleared).toEqual([]);
    expect(queued).toHaveLength(2);

    for (const fn of queued) fn();

    // Coords1 (~lat 40.7, lon -74) and coords2 (~lat 40.8, lon -73) live
    // in disjoint grid cells, so both prewarms' 6 samples each (every
    // 5th of 30 coords) seed the cache without colliding.
    expect(getTrackCacheSizes().snap).toBe(12);
  });

  it("tolerates degenerate shapes without throwing", () => {
    // Two-coord shape (the minimum buildShapeIndex accepts) at the same point
    // exercises the zero-length / single-grid-cell edge.
    const index = buildShapeIndex(makeRoutes([{ routeId: "X", coords: [[-74, 40.7], [-74, 40.7]] }]));
    expect(() => prewarmTrackCaches(index)).not.toThrow();
    // And an entirely empty index is a valid call too — defensive against
    // mode flips before any shapes arrive.
    expect(() => prewarmTrackCaches({})).not.toThrow();
  });
});
