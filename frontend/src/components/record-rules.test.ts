import { describe, expect, it } from "vitest";
import { andWhere, bothWhere, parseWhere, whereEmpty, type Rule } from "@/components/record-rules";

const r = (field: string): Rule => ({ field, op: "eq", value: 1 });

describe("andWhere", () => {
  it("adds to an AND list", () => {
    expect(andWhere({ any: false, rules: [r("a")] }, [r("x")])).toEqual({ any: false, rules: [r("a"), r("x")] });
  });
  it("brackets a plain OR", () => {
    expect(andWhere({ any: true, rules: [r("a"), r("b")] }, [r("x")])).toEqual({ any: false, rules: [r("x")], groups: [{ any: true, rules: [r("a"), r("b")] }] });
  });
  it("spreads over OR groups: (a) or (b and c) or (d or e)", () => {
    const w = andWhere({ any: true, rules: [r("a")], groups: [{ any: false, rules: [r("b"), r("c")] }, { any: true, rules: [r("d"), r("e")] }] }, [r("x")]);
    expect(w).toEqual({ any: true, rules: [], groups: [
      { any: false, rules: [r("a"), r("x")] },
      { any: false, rules: [r("b"), r("c"), r("x")] },
      { any: false, rules: [r("d"), r("x")] },
      { any: false, rules: [r("e"), r("x")] },
    ] });
  });
  it("keeps groups under AND", () => {
    const g = [{ any: true, rules: [r("a"), r("b")] }];
    expect(andWhere({ any: false, rules: [], groups: g }, [r("x")])).toEqual({ any: false, rules: [r("x")], groups: g });
  });
});

describe("parseWhere", () => {
  it("reads groups and treats junk as none", () => {
    expect(parseWhere('{"any":true,"rules":[],"groups":[{"rules":[{"field":"a","op":"empty"}]}]}')).toEqual({ any: true, rules: [], groups: [{ any: false, rules: [{ field: "a", op: "empty" }] }] });
    expect(whereEmpty(parseWhere("nope"))).toBe(true);
  });
});

describe("bothWhere", () => {
  it("joins a plain filter onto any other", () => {
    expect(bothWhere({ any: true, rules: [r("a"), r("b")] }, { any: false, rules: [r("x")] })).toEqual({ any: false, rules: [r("x")], groups: [{ any: true, rules: [r("a"), r("b")] }] });
    expect(bothWhere({ any: false, rules: [r("x")] }, { any: true, rules: [r("a"), r("b")] })).toEqual({ any: false, rules: [r("x")], groups: [{ any: true, rules: [r("a"), r("b")] }] });
    expect(bothWhere({ any: false, rules: [] }, { any: false, rules: [] })).toEqual({ any: false, rules: [] });
  });
  it("refuses two OR filters", () => {
    expect(bothWhere({ any: true, rules: [r("a"), r("b")] }, { any: true, rules: [r("c"), r("d")] })).toBeNull();
  });
});
