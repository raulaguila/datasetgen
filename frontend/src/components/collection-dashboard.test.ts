import { describe, expect, it } from "vitest";
import { packSpans } from "./collection-dashboard";

describe("packSpans", () => {
  it("lets the widget before a break and the last one fill their rows", () => {
    expect(packSpans([1, 2], 2)).toEqual([2, 2]);
    expect(packSpans([1, 2], 4)).toEqual([1, 3]);
    expect(packSpans([1, 1, 1], 2)).toEqual([1, 1, 2]);
    expect(packSpans([2, 1, 2], 4)).toEqual([2, 2, 4]);
    expect(packSpans([1, 1, 1, 1], 4)).toEqual([1, 1, 1, 1]);
  });
});
