import { describe, expect, it } from "vitest";
import { andWhere, bothWhere, cellRules, fromDraft, kindOf, opsFor, parseWhere, toDraft, whereEmpty, type DraftRule, type Rule } from "@/components/record-rules";
import { fieldsOf } from "@/components/field-builder";

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

const draft = (p: Partial<DraftRule>): DraftRule => ({ field: "f", op: "eq", raw: "", raw2: "", list: [], ...p });

describe("rules in the editor", () => {
  it("picks operators by what a field holds", () => {
    expect(kindOf({ type: "autonumber", result: "" })).toBe("number");
    expect(kindOf({ type: "formula", result: "date" })).toBe("date");
    expect(kindOf({ type: "created_by", result: "" })).toBe("text");
    expect(opsFor("refs")).toEqual(["has", "has_any", "empty", "not_empty"]);
    expect(opsFor("date")).toContain("last_days");
  });
  it("saves relative dates as today plus days", () => {
    expect(fromDraft(draft({ op: "lt", raw: "@0" }), "date")).toEqual({ field: "f", op: "lt", value: { today: 0 } });
    expect(fromDraft(draft({ op: "last_days", raw: "7" }), "date")).toEqual({ field: "f", op: "between", value: [{ today: -7 }, { today: 0 }] });
    expect(fromDraft(draft({ op: "between", raw: "2026-01-01", raw2: "@-1" }), "date")).toEqual({ field: "f", op: "between", value: ["2026-01-01", { today: -1 }] });
  });
  it("reads them back the same way", () => {
    expect(toDraft({ field: "f", op: "between", value: [{ today: 0 }, { today: 30 }] }, "date")).toMatchObject({ op: "next_days", raw: "30" });
    expect(toDraft({ field: "f", op: "gte", value: "2026-01-02T10:00:00Z" }, "date")).toMatchObject({ raw: "2026-01-02" });
    expect(toDraft({ field: "f", op: "not_in", value: ["a", "b"] }, "enum")).toMatchObject({ list: ["a", "b"] });
    expect(toDraft({ field: "f", op: "has_any", value: ["a", "b"] }, "strings")).toMatchObject({ raw: "a, b" });
  });
  it("waits for a value before saving", () => {
    expect(fromDraft(draft({ op: "between", raw: "1" }), "number")).toBeNull();
    expect(fromDraft(draft({ op: "in" }), "enum")).toBeNull();
    expect(fromDraft(draft({ op: "between", raw: "1", raw2: "5" }), "number")).toEqual({ field: "f", op: "between", value: [1, 5] });
    expect(fromDraft(draft({ op: "has_any", raw: "x, y" }), "strings")).toEqual({ field: "f", op: "has_any", value: ["x", "y"] });
  });
});

describe("cellRules", () => {
  const rows = fieldsOf({ type: "object", properties: { n: { type: "number" }, d: { type: "string", format: "date-time" }, tags: { type: "array", items: { type: "string" } }, j: { type: "object" } } });
  const row = (k: string) => rows.find((r) => r.key === k)!;
  it("keeps or drops a cell's value", () => {
    expect(cellRules(row("n"), 3)).toEqual({ keep: { field: "n", op: "eq", value: 3 }, drop: { field: "n", op: "neq", value: 3 } });
    expect(cellRules(row("d"), "2026-01-02T10:00:00Z")?.keep).toEqual({ field: "d", op: "eq", value: "2026-01-02" });
    expect(cellRules(row("tags"), ["x", "y"])).toEqual({ keep: { field: "tags", op: "has", value: "x" } });
    expect(cellRules(row("n"), undefined)?.keep.op).toBe("empty");
    expect(cellRules(row("j"), { a: 1 })).toBeNull();
  });
});
