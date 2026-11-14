import { describe, expect, it } from "vitest";
import { fillSeries } from "./collection-stats";

describe("fillSeries", () => {
  it("adds empty months between the first and last", () => {
    expect(fillSeries([{ at: "2026-01-01", count: 2 }, { at: "2026-04-01", count: 1 }], "month")).toEqual([
      { at: "2026-01-01", count: 2 }, { at: "2026-02-01", count: 0 }, { at: "2026-03-01", count: 0 }, { at: "2026-04-01", count: 1 },
    ]);
  });
  it("steps weeks by seven days and keeps a single point", () => {
    expect(fillSeries([{ at: "2026-01-05", count: 1 }, { at: "2026-01-19", count: 3 }], "week").map((p) => p.at)).toEqual(["2026-01-05", "2026-01-12", "2026-01-19"]);
    expect(fillSeries([{ at: "2026-01-05", count: 1 }], "day")).toHaveLength(1);
  });
  it("keeps the latest 400 bars", () => {
    const out = fillSeries([{ at: "2020-01-01", count: 1 }, { at: "2026-01-01", count: 1 }], "day");
    expect(out).toHaveLength(400);
    expect(out[399].at).toBe("2026-01-01");
  });
});
