import { expect, it } from "vitest";
import { neighbours } from "./sequence";

it("finds the records around one in the last list", () => {
  expect(neighbours(["a", "b", "c"], "b")).toEqual({ prev: "a", next: "c", index: 1 });
  expect(neighbours(["a", "b"], "a")).toEqual({ prev: undefined, next: "b", index: 0 });
  expect(neighbours(["a"], "z")).toEqual({ index: -1 });
});
