import { describe, expect, it } from "vitest";
import { collectionInPath, collectionPath, movedPath, recordPath } from "./paths";

describe("collection paths", () => {
  const c = { owner: "ana", slug: "livros" };

  it("builds the collection, section and record addresses", () => {
    expect(collectionPath(c)).toBe("/ana/livros");
    expect(collectionPath(c, "fields")).toBe("/ana/livros/fields");
    expect(recordPath(c, "r1")).toBe("/ana/livros/r/r1");
    expect(recordPath(c, "r1", true)).toBe("/ana/livros/r/r1/edit");
  });

  it("moves an old address to the current one, keeping the rest", () => {
    expect(movedPath("/Ana/Livros-Lidos/r/r1/edit", { owner: "ana", slug: "livros-lidos" }, { owner: "bia", slug: "leituras" })).toBe("/bia/leituras/r/r1/edit");
    expect(movedPath("/x/y", c, { owner: "bia", slug: "z" })).toBe("/x/y");
  });

  it("tells a collection address from the app's own pages", () => {
    expect(collectionInPath("/Ana/livros/r/1")).toEqual({ owner: "ana", slug: "livros" });
    expect(collectionInPath("/admin/users")).toBeUndefined();
    expect(collectionInPath("/collections/new")).toBeUndefined();
    expect(collectionInPath("/profile")).toBeUndefined();
  });
});
