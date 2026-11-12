import { expect, it } from "vitest";
import { readListParams } from "./use-list-params";

it("keeps good values and drops bad ones back to the default", () => {
  const defaults = { page: 1, limit: 25, q: "", sort: "name" };
  expect(readListParams(defaults, new URLSearchParams("page=3&q=ana&sort=email"))).toEqual({ page: 3, limit: 25, q: "ana", sort: "email" });
  expect(readListParams(defaults, new URLSearchParams("page=0&limit=abc&other=x"))).toEqual(defaults);
});
