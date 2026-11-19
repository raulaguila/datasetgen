import { describe, expect, it } from "vitest";
import { targetHref } from "./audit";
import { isActive } from "./navigation";
import type { AuditRecord } from "./api/types";

describe("collection pages in the navigation", () => {
  it("counts /<owner>/<slug> pages as the collections area", () => {
    expect(isActive("/ana/livros/fields", "/collections")).toBe(true);
    expect(isActive("/collections/new", "/collections")).toBe(true);
    expect(isActive("/admin/users", "/collections")).toBe(false);
  });

  it("links an audit entry to the collection's address, a record to its permalink", () => {
    const base: AuditRecord = { id: "a", created_at: "", action: "collection.update", target_id: "c1", details: {} };
    expect(targetHref({ ...base, target_owner: "ana", target_slug: "livros" })).toBe("/ana/livros");
    expect(targetHref({ ...base, action: "record.update", target_id: "r1" })).toBe("/records/r1");
    expect(targetHref({ ...base, target_gone: true, target_owner: "ana", target_slug: "livros" })).toBeUndefined();
  });
});
