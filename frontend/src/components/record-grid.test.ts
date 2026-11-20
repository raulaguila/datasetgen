import { describe, expect, it } from "vitest";
import { fieldsOf } from "@/components/field-builder";
import { pickColumns } from "@/components/record-grid";

describe("pickColumns", () => {
  const rows = fieldsOf({ type: "object", required: ["titulo"], properties: { notas: { type: "string" }, a: { type: "integer" }, b: { type: "boolean" }, c: { type: "number" }, titulo: { type: "string" } } });

  it("puts the title field first and keeps four by default", () => {
    expect(pickColumns(rows, "", "titulo").map((r) => r.key)).toEqual(["titulo", "notas", "a", "b"]);
  });

  it("puts the title first among chosen columns too", () => {
    expect(pickColumns(rows, "notas,titulo,c", "titulo").map((r) => r.key)).toEqual(["titulo", "notas", "c"]);
  });
});
