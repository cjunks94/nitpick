import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import type { RoutesGeoJSON } from "@panoptrain/shared";
import {
  _resetTrackCachesForTests,
  buildShapeIndex,
  findTrackPath,
  getTrackCacheSizes,
} from "../trackInterpolation.js";

/**
 * Pins the lifecycle of `buildShapeIndex` w.r.t. mode-tab switching:
 * calling it with the *same* routes object (the cache-hit path on tab
 * re-entry) should be a no-op — return the previously-built index, leave
 * the turf snap caches intact. Calling it with a *different* routes object
 * (fresh fetch landing) must rebuild as today.
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

// jsdom doesn't implement requestIdleCallback, so buildShapeIndex routes
// through setTimeout. Run the prewarm synchronously so cache-size assertions
// reflect the post-prewarm state.
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

describe("buildShapeIndex — same routes reference", () => {
  it("returns the same index object on a second call (memoized by ref)", () => {
    const coords: [number, number][] = Array.from({ length: 20 }, (_, i) => [-74 + i * 0.001, 40.7]);
    const routes = makeRoutes([{ routeId: "1", coords }]);
    const first = buildShapeIndex(routes);
    const second = buildShapeIndex(routes);
    expect(second).toBe(first);
  });
});

describe("buildShapeIndex — different routes object", () => {
  it("returns a new index object when given a different routes payload", () => {
    const coords: [number, number][] = Array.from({ length: 20 }, (_, i) => [-74 + i * 0.001, 40.7]);
    const routes1 = makeRoutes([{ routeId: "1", coords }]);
    const routes2 = makeRoutes([{ routeId: "2", coords }]);
    const first = buildShapeIndex(routes1);
    const second = buildShapeIndex(routes2);
    expect(second).not.toBe(first);
  });
});

describe("buildShapeIndex — back-and-forth between two routes payloads", () => {
  // The real subway↔LIRR↔subway tab dance. The first build for each
  // payload is the cold path; subsequent builds for the same payload
  // must return the originally-cached index without rebuilding (so the
  // warmed turf snap caches stay valid).
  it("preserves the original index for each payload across alternating calls", () => {
    const coordsA: [number, number][] = Array.from({ length: 20 }, (_, i) => [-74 + i * 0.001, 40.7]);
    const coordsB: [number, number][] = Array.from({ length: 20 }, (_, i) => [-73 + i * 0.001, 40.8]);
    const routesA = makeRoutes([{ routeId: "A", coords: coordsA }]);
    const routesB = makeRoutes([{ routeId: "B", coords: coordsB }]);

    const a1 = buildShapeIndex(routesA);
    const b1 = buildShapeIndex(routesB);
    const a2 = buildShapeIndex(routesA);
    const b2 = buildShapeIndex(routesB);

    expect(a2).toBe(a1);
    expect(b2).toBe(b1);
    expect(a1).not.toBe(b1);
  });
});

describe("buildShapeIndex — cache lifecycle on rebuild", () => {
  // snapCache is keyed by globally-unique shapeId, so it stays warm
  // across builds. bestShapeCache values are direct ShapeData refs and
  // its keys are routeId-based — and routeIds can collide between
  // subway and LIRR — so a stale ref would route the wrong line through
  // findTrackPath. Verify the asymmetric clear: snap survives, bestShape
  // resets each rebuild.
  it("clears bestShapeCache but preserves snapCache when building a new index", () => {
    const coordsA: [number, number][] = Array.from({ length: 30 }, (_, i) => [-74 + i * 0.001, 40.7]);
    const coordsB: [number, number][] = Array.from({ length: 30 }, (_, i) => [-73 + i * 0.001, 40.8]);
    const routesA = makeRoutes([{ routeId: "A", coords: coordsA }]);
    const routesB = makeRoutes([{ routeId: "B", coords: coordsB }]);

    const indexA = buildShapeIndex(routesA);
    findTrackPath(indexA, "A", coordsA[5]!, coordsA[10]!);
    const after = getTrackCacheSizes();
    expect(after.snap).toBeGreaterThan(0);
    expect(after.bestShape).toBeGreaterThan(0);

    buildShapeIndex(routesB);
    const afterRebuild = getTrackCacheSizes();
    expect(afterRebuild.snap).toBeGreaterThanOrEqual(after.snap); // snap kept + B's prewarm seeded more
    expect(afterRebuild.bestShape).toBe(0); // bestShape cleared on rebuild
  });
});
