import { expect, it } from "vitest";
import { numberStyle, sectionsOf } from "./record-view";

it("splits fields into the form's sections and leaves out the ones whose condition fails", () => {
  const props = {
    a: { type: "string" }, b: { type: "string", "x-section": "Contato" }, c: { type: "string" },
    d: { type: "string", "x-show-when": { field: "a", op: "eq", value: "x" } }, e: { type: "string", "x-section": "Fim" },
  };
  const keys = Object.keys(props);
  expect(sectionsOf(props, keys, { a: "y" })).toEqual([{ title: "", keys: ["a"] }, { title: "Contato", keys: ["b", "c"] }, { title: "Fim", keys: ["e"] }]);
  expect(sectionsOf(props, keys, { a: "x" })[1]).toEqual({ title: "Contato", keys: ["b", "c", "d"] });
});

it("reads a number field's own format", () => {
  expect(numberStyle({ type: "number" })).toBeNull();
  expect(numberStyle({ "x-format": "currency" })).toEqual({ format: "currency", decimals: undefined });
  expect(numberStyle({ "x-decimals": 1 })).toEqual({ format: "number", decimals: 1 });
});
