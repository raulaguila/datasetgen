import { describe, expect, it } from "vitest";
import { safeNext } from "./utils";

describe("safeNext", () => {
  it.each([
    ["/admin/users?page=2", "/admin/users?page=2"],
    [null, "/"],
    ["https://evil.example", "/"],
    ["//evil.example", "/"],
    ["/\\evil.example", "/"],
  ])("%s -> %s", (raw, want) => expect(safeNext(raw)).toBe(want));
});
