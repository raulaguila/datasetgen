import { cn } from "@/lib/utils";

// An option's color, as a list of options sets it: the same tint on the record
// list, the record page and the board, light and dark.
const CLS: Record<string, { chip: string; dot: string }> = {
  gray: { chip: "border-zinc-500/30 bg-zinc-500/10 text-zinc-700 dark:text-zinc-300", dot: "bg-zinc-500" },
  red: { chip: "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300", dot: "bg-red-500" },
  orange: { chip: "border-orange-500/30 bg-orange-500/10 text-orange-700 dark:text-orange-300", dot: "bg-orange-500" },
  amber: { chip: "border-amber-500/30 bg-amber-500/10 text-amber-800 dark:text-amber-300", dot: "bg-amber-500" },
  green: { chip: "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300", dot: "bg-emerald-500" },
  teal: { chip: "border-teal-500/30 bg-teal-500/10 text-teal-700 dark:text-teal-300", dot: "bg-teal-500" },
  blue: { chip: "border-sky-500/30 bg-sky-500/10 text-sky-700 dark:text-sky-300", dot: "bg-sky-500" },
  violet: { chip: "border-violet-500/30 bg-violet-500/10 text-violet-700 dark:text-violet-300", dot: "bg-violet-500" },
  pink: { chip: "border-pink-500/30 bg-pink-500/10 text-pink-700 dark:text-pink-300", dot: "bg-pink-500" },
};
const NONE = { chip: "", dot: "bg-transparent border border-dashed border-muted-foreground/50" };

export const optionColor = (color: string | undefined) => (color && CLS[color]) || NONE;

/** The color an option has in a field's schema, if any. */
export function colorOf(schema: Record<string, unknown> | undefined, value: unknown): string | undefined {
  const colors = schema?.["x-colors"];
  return colors && typeof colors === "object" && typeof value === "string" ? (colors as Record<string, string>)[value] : undefined;
}

export function OptionDot({ color, className }: { color: string | undefined; className?: string }) {
  return color && CLS[color] ? <span className={cn("inline-block size-2 shrink-0 rounded-full", CLS[color].dot, className)} aria-hidden /> : null;
}
