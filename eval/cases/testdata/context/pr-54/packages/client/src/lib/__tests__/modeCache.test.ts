import { describe, it, expect, beforeEach } from "vitest";
import type {
  RoutesGeoJSON,
  StopsGeoJSON,
  TrainsResponse,
} from "@panoptrain/shared";
import {
  clearMode,
  getLastTrains,
  getRoutes,
  getStops,
  setLastTrains,
  setRoutes,
  setStops,
} from "../modeCache.js";

function makeRoutes(routeId: string): RoutesGeoJSON {
  return {
    type: "FeatureCollection",
    features: [
      {
        type: "Feature",
        properties: { routeId, color: "888888", name: routeId },
        geometry: {
          type: "LineString",
          coordinates: [
            [-74, 40.7],
            [-74.001, 40.7],
          ],
        },
      },
    ],
  };
}

function makeStops(stopId: string): StopsGeoJSON {
  return {
    type: "FeatureCollection",
    features: [
      {
        type: "Feature",
        properties: { stopId, stopName: stopId },
        geometry: { type: "Point", coordinates: [-74, 40.7] },
      },
    ],
  };
}

function makeTrains(routeId: string): TrainsResponse {
  return {
    timestamp: 1_700_000_000_000,
    count: 1,
    trains: [
      {
        tripId: "t1",
        routeId,
        direction: 0,
        latitude: 40.7,
        longitude: -74,
        bearing: null,
        status: "IN_TRANSIT_TO",
        currentStopId: "s1",
        currentStopName: "S1",
        nextStopId: null,
        nextStopName: null,
        destination: "Dest",
        delay: null,
        updatedAt: 1_700_000_000,
      },
    ],
  };
}

beforeEach(() => {
  clearMode("subway");
  clearMode("lirr");
});

describe("modeCache — routes", () => {
  it("returns undefined before any setRoutes call", () => {
    expect(getRoutes("subway")).toBeUndefined();
    expect(getRoutes("lirr")).toBeUndefined();
  });

  it("returns the same reference passed to setRoutes (so enrichCache WeakMap still hits)", () => {
    const data = makeRoutes("1");
    setRoutes("subway", data);
    expect(getRoutes("subway")).toBe(data);
  });

  it("isolates subway and lirr (no cross-mode collision)", () => {
    const subwayRoutes = makeRoutes("1");
    const lirrRoutes = makeRoutes("Babylon");
    setRoutes("subway", subwayRoutes);
    setRoutes("lirr", lirrRoutes);
    expect(getRoutes("subway")).toBe(subwayRoutes);
    expect(getRoutes("lirr")).toBe(lirrRoutes);
  });

  it("overwrites on subsequent setRoutes for the same mode", () => {
    const first = makeRoutes("1");
    const second = makeRoutes("2");
    setRoutes("subway", first);
    setRoutes("subway", second);
    expect(getRoutes("subway")).toBe(second);
  });
});

describe("modeCache — stops", () => {
  it("returns undefined before any setStops call", () => {
    expect(getStops("subway")).toBeUndefined();
  });

  it("returns the same reference passed to setStops", () => {
    const data = makeStops("S1");
    setStops("lirr", data);
    expect(getStops("lirr")).toBe(data);
  });

  it("isolates subway and lirr stops", () => {
    const sub = makeStops("S1");
    const lir = makeStops("Penn");
    setStops("subway", sub);
    setStops("lirr", lir);
    expect(getStops("subway")).toBe(sub);
    expect(getStops("lirr")).toBe(lir);
  });
});

describe("modeCache — last trains", () => {
  it("returns undefined before any setLastTrains call", () => {
    expect(getLastTrains("subway")).toBeUndefined();
  });

  it("wraps the trains payload with a fetchedAt timestamp", () => {
    const trains = makeTrains("1");
    const before = Date.now();
    setLastTrains("subway", trains);
    const after = Date.now();
    const cached = getLastTrains("subway");
    expect(cached).toBeDefined();
    expect(cached!.data).toBe(trains);
    expect(cached!.fetchedAt).toBeGreaterThanOrEqual(before);
    expect(cached!.fetchedAt).toBeLessThanOrEqual(after);
  });

  it("isolates subway and lirr trains", () => {
    const sub = makeTrains("1");
    const lir = makeTrains("Babylon");
    setLastTrains("subway", sub);
    setLastTrains("lirr", lir);
    expect(getLastTrains("subway")!.data).toBe(sub);
    expect(getLastTrains("lirr")!.data).toBe(lir);
  });
});

describe("modeCache — clearMode", () => {
  it("empties only the named mode's slots", () => {
    setRoutes("subway", makeRoutes("1"));
    setStops("subway", makeStops("S1"));
    setLastTrains("subway", makeTrains("1"));
    setRoutes("lirr", makeRoutes("Babylon"));
    setStops("lirr", makeStops("Penn"));
    setLastTrains("lirr", makeTrains("Babylon"));

    clearMode("subway");

    expect(getRoutes("subway")).toBeUndefined();
    expect(getStops("subway")).toBeUndefined();
    expect(getLastTrains("subway")).toBeUndefined();

    expect(getRoutes("lirr")).toBeDefined();
    expect(getStops("lirr")).toBeDefined();
    expect(getLastTrains("lirr")).toBeDefined();
  });
});
