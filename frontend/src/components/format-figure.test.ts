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

it("compares a period with the one before it", async () => {
  const { periods, change } = await import("@/components/collection-stats");
  const [now, before] = periods({ field: "d", days: 30 }, new Date(2026, 10, 16));
  expect(now).toEqual([{ field: "d", op: "gte", value: "2026-10-18" }]);
  expect(before).toEqual([{ field: "d", op: "gte", value: "2026-09-18" }, { field: "d", op: "lt", value: "2026-10-18" }]);
  expect(change(120, 100)).toBe(20);
  expect(change(50, 100)).toBe(-50);
  expect(change(5, 0)).toBeNull();
});

it("formats a number field as it asks", async () => {
  const { fieldFigure } = await import("@/components/format-figure");
  const plain = (n: number) => `plain ${n}`;
  expect(fieldFigure({ numberFormat: "", decimals: "" }, 3, "pt-BR", plain)).toBe("plain 3");
  expect(fieldFigure({ numberFormat: "currency", decimals: "" }, 3, "pt-BR", plain)).toMatch(/R\$\s?3,00/);
  expect(fieldFigure({ numberFormat: "", decimals: "1" }, 3, "en-US", plain)).toBe("3.0");
});
