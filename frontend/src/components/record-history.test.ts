import { describe, expect, it } from "vitest";
import { changes } from "./record-history";
import { isChip, isShort } from "./record-view";

describe("changes", () => {
  it("lists the fields that differ and, for lists, the items that left or arrived", () => {
    const before = { nome: "A", tags: ["x", "y"], fontes: [{ bloco: "Mobi" }], igual: 1 };
    const after = { nome: "B", tags: ["y", "z"], fontes: [{ bloco: "Mobi" }, { bloco: "Usuário" }], igual: 1, novo: true };
    expect(changes(before, after)).toEqual([
      { key: "nome", before: "A", after: "B" },
      { key: "tags", before: ["x", "y"], after: ["y", "z"], removed: ["x"], added: ["z"] },
      { key: "fontes", before: before.fontes, after: after.fontes, removed: [], added: [{ bloco: "Usuário" }] },
      { key: "novo", before: undefined, after: true },
    ]);
  });
});

describe("record view layout", () => {
  it("keeps one-line values for the summary", () => {
    expect(isShort({ type: "string" }, "curto")).toBe(true);
    expect(isShort({ type: "string", maxLength: 5000 }, "curto")).toBe(false);
    expect(isShort({ type: "string" }, "linha\noutra")).toBe(false);
    expect(isShort({ type: "array" }, ["a", "b"])).toBe(true);
    expect(isShort({ type: "array" }, [{ a: 1 }])).toBe(false);
    expect(isShort({ type: "integer" }, 3)).toBe(true);
  });

  it("puts unique values and list picks in the header", () => {
    expect(isChip({ type: "string", "x-unique": true }, "wf_x")).toBe(true);
    expect(isChip({ type: "string", enum: ["a"] }, "a")).toBe(true);
    expect(isChip({ type: "string" }, "a")).toBe(false);
  });
});
