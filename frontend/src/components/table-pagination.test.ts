import { expect, it } from "vitest";
import { emptyPageFallback } from "./table-pagination";

const page = (items: number, total: number) => ({ items: Array(items).fill(0), pagination: { total_items: total } });

it("moves back only when a later page came back empty", () => {
  expect(emptyPageFallback(3, 25, page(0, 50))).toBe(2);
  expect(emptyPageFallback(3, 25, page(0, 0))).toBe(1);
  expect(emptyPageFallback(1, 25, page(0, 0))).toBeNull();
  expect(emptyPageFallback(2, 25, page(5, 30))).toBeNull();
  expect(emptyPageFallback(2, 25, undefined)).toBeNull();
});
