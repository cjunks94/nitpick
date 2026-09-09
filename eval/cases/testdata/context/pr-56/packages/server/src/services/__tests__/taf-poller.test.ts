import { describe, it, expect } from "vitest";
import { parseTafResponse } from "../taf-poller.js";

/**
 * Pins the parser shape — the rest of the poller (fetch, cache, retry,
 * polling cadence) is structural and mirrors the METAR poller. The
 * value here is in the parser handling all the upstream's edge cases
 * cleanly, since TAF records carry more shape variation than METAR
 * (overlay groups omit fields, visibility ships as "P6SM" or
 * fractional strings, fcstChange variants).
 */

const NOW = 1_780_000_000_000;

function makeRecord(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    icaoId: "KJFK",
    issueTime: "2026-05-09T22:00:00.000Z",
    validTimeFrom: 1_778_400_000,
    validTimeTo: 1_778_500_000,
    rawTAF: "TAF KJFK ...",
    fcsts: [],
    ...overrides,
  };
}

describe("parseTafResponse — record-level", () => {
  it("keys reports by ICAO and converts unix-second times to ms", () => {
    const out = parseTafResponse([makeRecord()], NOW);
    expect(out.reports.KJFK).toBeDefined();
    expect(out.reports.KJFK.validFrom).toBe(1_778_400_000_000);
    expect(out.reports.KJFK.validTo).toBe(1_778_500_000_000);
    expect(out.reports.KJFK.issuedAt).toBe(Date.parse("2026-05-09T22:00:00.000Z"));
  });

  it("drops records missing required fields", () => {
    const out = parseTafResponse([
      makeRecord({ icaoId: undefined }),
      makeRecord({ icaoId: "KLGA", issueTime: undefined }),
      makeRecord({ icaoId: "KEWR", validTimeFrom: undefined }),
      makeRecord({ icaoId: "KHPN" }),
    ], NOW);
    expect(Object.keys(out.reports)).toEqual(["KHPN"]);
  });

  it("sets sourceTimestamp to the latest issueTime across reports", () => {
    const earlier = "2026-05-09T18:00:00.000Z";
    const later = "2026-05-09T22:00:00.000Z";
    const out = parseTafResponse([
      makeRecord({ icaoId: "KJFK", issueTime: earlier }),
      makeRecord({ icaoId: "KLGA", issueTime: later }),
    ], NOW);
    expect(out.sourceTimestamp).toBe(Date.parse(later));
  });

  it("falls back sourceTimestamp to `now` when no records parsed", () => {
    const out = parseTafResponse([], NOW);
    expect(out.sourceTimestamp).toBe(NOW);
    expect(Object.keys(out.reports)).toEqual([]);
  });
});

describe("parseTafResponse — forecast group parsing", () => {
  it("parses a base FM group with full conditions", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [{
        timeFrom: 1_778_400_000,
        timeTo: 1_778_410_800,
        fcstChange: "FM",
        wdir: 270,
        wspd: 15,
        wgst: 22,
        visib: 6,
        wxString: "-SHRA BR",
        clouds: [{ cover: "BKN", base: 3000 }],
      }],
    })], NOW);
    const f = out.reports.KJFK.forecasts[0];
    expect(f.fcstChange).toBe("FM");
    expect(f.wind).toEqual({ directionDeg: 270, speedKt: 15, gustKt: 22 });
    expect(f.visibilitySm).toBe(6);
    expect(f.ceilingFt).toBe(3000);
    expect(f.wxString).toBe("-SHRA BR");
  });

  it("treats null fcstChange as the base period", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [{ timeFrom: 1_778_400_000, timeTo: 1_778_500_000, fcstChange: null, wdir: 180, wspd: 10 }],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].fcstChange).toBeNull();
  });

  it("normalizes PROB30 / PROB40 variants to the PROB sentinel", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [{ timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "PROB30", probability: 30 }],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].fcstChange).toBe("PROB");
    expect(out.reports.KJFK.forecasts[0].probability).toBe(30);
  });

  it("nulls wind when neither speed nor direction present (TEMPO/BECMG inheriting from base)", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [{ timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "TEMPO", visib: 3, wxString: "SHRA" }],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].wind).toBeNull();
  });

  it("treats VRB direction as null", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [{ timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "FM", wdir: "VRB", wspd: 3 }],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].wind).toEqual({ directionDeg: null, speedKt: 3, gustKt: null });
  });

  it("converts P6SM and 6+ to numeric 6", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [
        { timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "FM", visib: "P6SM" },
        { timeFrom: 1_778_410_000, timeTo: 1_778_420_000, fcstChange: "FM", visib: "6+" },
      ],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].visibilitySm).toBe(6);
    expect(out.reports.KJFK.forecasts[1].visibilitySm).toBe(6);
  });

  it("parses fractional visibility (1/2 sm fog)", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [{ timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "FM", visib: "1/2" }],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].visibilitySm).toBe(0.5);
  });

  it("derives ceiling from lowest BKN/OVC/VV layer (ignores FEW/SCT)", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [{
        timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "FM",
        clouds: [
          { cover: "FEW", base: 500 },
          { cover: "SCT", base: 1500 },
          { cover: "BKN", base: 3000 },
          { cover: "OVC", base: 5000 },
        ],
      }],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].ceilingFt).toBe(3000);
  });

  it("picks the lowest qualifying layer regardless of array order", () => {
    // Defensive — the upstream typically sorts by altitude ascending,
    // but the API doesn't document that contract. Out-of-order layers
    // must still produce the correct (lowest) ceiling.
    const out = parseTafResponse([makeRecord({
      fcsts: [{
        timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "FM",
        clouds: [
          { cover: "OVC", base: 8000 },
          { cover: "BKN", base: 2500 },
        ],
      }],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].ceilingFt).toBe(2500);
  });

  it("returns null ceiling when only FEW/SCT layers are present", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [{
        timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "FM",
        clouds: [{ cover: "FEW", base: 5000 }, { cover: "SCT", base: 8000 }],
      }],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].ceilingFt).toBeNull();
  });

  it("trims and normalizes empty wxString to null", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [
        { timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "FM", wxString: "  " },
        { timeFrom: 1_778_410_000, timeTo: 1_778_420_000, fcstChange: "FM", wxString: "  -SHRA  " },
      ],
    })], NOW);
    expect(out.reports.KJFK.forecasts[0].wxString).toBeNull();
    expect(out.reports.KJFK.forecasts[1].wxString).toBe("-SHRA");
  });

  it("drops forecast groups missing timeFrom/timeTo", () => {
    const out = parseTafResponse([makeRecord({
      fcsts: [
        { fcstChange: "FM", wdir: 270, wspd: 10 },
        { timeFrom: 1_778_400_000, timeTo: 1_778_410_000, fcstChange: "FM", wdir: 180, wspd: 5 },
      ],
    })], NOW);
    expect(out.reports.KJFK.forecasts).toHaveLength(1);
    expect(out.reports.KJFK.forecasts[0].wind?.directionDeg).toBe(180);
  });
});
