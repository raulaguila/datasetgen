import type { Widget } from "@/lib/api/types";

/** A number the way a format reads it: a widget's, or a number field's own (x-format). */
export function formatFigure(v: number, locale: string, format: Widget["format"] = "number", decimals?: number): string {
  const fixed = decimals === undefined ? {} : { minimumFractionDigits: decimals, maximumFractionDigits: decimals };
  if (format === "compact") return new Intl.NumberFormat(locale, { notation: "compact", maximumFractionDigits: decimals ?? 1 }).format(v);
  // shortcut: the money follows the language (BRL / USD), fine while each install bills in one currency.
  if (format === "currency") return new Intl.NumberFormat(locale, { style: "currency", currency: locale === "pt-BR" ? "BRL" : "USD", ...fixed }).format(v);
  // The stored number is already the percentage (12.5 → 12,5%).
  if (format === "percent") return `${new Intl.NumberFormat(locale, { maximumFractionDigits: decimals ?? 1, ...fixed }).format(v)}%`;
  return new Intl.NumberFormat(locale, { maximumFractionDigits: decimals ?? 2, ...fixed }).format(v);
}


/** A number field's value as its own format reads it; `fallback` when the field sets none. */
export function fieldFigure(f: { numberFormat: string; decimals: string; type?: string } | undefined, v: number, locale: string, fallback: (v: number) => string): string {
  // Whole numbers of an integer field are often codes (an order number), where a thousands separator misleads.
  if (!f || (!f.numberFormat && !f.decimals)) return f?.type === "integer" && Number.isInteger(v) ? String(v) : fallback(v);
  return formatFigure(v, locale, (f.numberFormat || "number") as Widget["format"], f.decimals ? Number(f.decimals) : undefined);
}
