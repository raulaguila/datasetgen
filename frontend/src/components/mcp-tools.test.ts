import { describe, expect, it } from "vitest";
import { blankField, type FieldRow, type FieldType } from "./field-builder";
import { opsFor } from "./mcp-tools";

const row = (type: FieldType, result = ""): FieldRow => ({ key: "k", field: { ...blankField, key: "k", type, result }, advanced: false });

describe("opsFor", () => {
  it("offers comparisons the backend accepts for the field", () => {
    expect(opsFor(row("string"))).toContain("contains");
    expect(opsFor(row("number"))).not.toContain("contains");
    expect(opsFor(row("number"))).toContain("gte");
    expect(opsFor(row("boolean"))).toEqual(["eq"]);
    expect(opsFor(row("strings"))).toEqual(["has"]);
    expect(opsFor(row("formula", "number"))).toContain("lt");
  });
  it("leaves out fields that cannot be a parameter", () => {
    for (const t of ["file", "files", "objects", "group", "json", "lookup", "rollup"] as FieldType[]) expect(opsFor(row(t))).toEqual([]);
  });
});
