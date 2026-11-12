import { describe, expect, it } from "vitest";
import { COLLECTION_ICON_GROUPS, COLLECTION_ICONS } from "./collection-icons";

describe("collection icon groups", () => {
  it("hold every icon exactly once", () => {
    const grouped = COLLECTION_ICON_GROUPS.flatMap((g) => g.slugs);
    expect(grouped.sort()).toEqual(Object.keys(COLLECTION_ICONS).sort());
  });
});
