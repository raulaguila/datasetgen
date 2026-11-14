import { describe, expect, it } from "vitest";
import { runs } from "./audit-log";

const at = (min: number) => new Date(2026, 10, 14, 4, min).toISOString();
const e = (id: string, min: number, details: Record<string, unknown>, action = "records.imported") => ({ id, created_at: at(min), action, actor_id: "u", target_id: "c", details });

describe("runs", () => {
  it("joins one person's repeated action minutes apart and adds up its counts", () => {
    const out = runs([e("a", 32, { count: 0, updated: 50 }), e("b", 32, { count: 0, updated: 21 }), e("c", 22, { count: 0, updated: 21 }), e("d", 21, { widgets: 2 }, "dashboard.updated")]);
    expect(out.map((g) => g.all.map((r) => r.id))).toEqual([["a", "b"], ["c"], ["d"]]);
    expect(out[0].details).toEqual({ count: 0, updated: 71 });
  });

  it("keeps apart entries whose details say different things", () => {
    const out = runs([e("a", 1, { fields: { added: ["x"] } }, "collection.updated"), e("b", 1, { fields: { added: ["y"] } }, "collection.updated")]);
    expect(out).toHaveLength(2);
  });
});
