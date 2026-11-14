import { expect, it } from "vitest";
import { formatFigure, recordsHref } from "@/components/collection-stats";

it("formats a widget's figure", () => {
  expect(formatFigure(1234.567, "pt-BR")).toBe("1.234,57");
  expect(formatFigure(1234.567, "pt-BR", "number", 0)).toBe("1.235");
  expect(formatFigure(12.5, "pt-BR", "percent")).toBe("12,5%");
  expect(formatFigure(1500, "en-US", "compact")).toBe("1.5K");
  expect(formatFigure(10, "en-US", "currency")).toBe("$10.00");
});

it("links a figure to its records", () => {
  expect(recordsHref("c1", { any: false, rules: [] })).toBe("/collections/c1");
  expect(recordsHref("c1", { any: false, rules: [{ field: "a", op: "empty" }] })).toMatch(/^\/collections\/c1\?where=%7B/);
});
