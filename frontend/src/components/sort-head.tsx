"use client";

import { ArrowDown, ArrowUp } from "lucide-react";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { TableHead } from "@/components/ui/table";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";
import type { SortOrder } from "@/lib/api/types";

/** A column header that sorts the list; a new column starts ascending, dates newest first. */
export function SortHead({ field, label, sort, order, onSort, className }: {
  field: string;
  label: string;
  sort: string;
  order: SortOrder;
  onSort: (sort: string, order: SortOrder) => void;
  className?: string;
}) {
  const active = sort === field;
  const next = active ? (order === "asc" ? "desc" : "asc") : field.endsWith("_at") ? "desc" : "asc";
  return (
    <TableHead className={className} aria-sort={active ? (order === "asc" ? "ascending" : "descending") : undefined}>
      <button type="button" className="inline-flex items-center gap-1 hover:text-foreground" onClick={() => onSort(field, next)}>
        {label}
        {active && (order === "asc" ? <ArrowUp className="size-3.5" /> : <ArrowDown className="size-3.5" />)}
      </button>
    </TableHead>
  );
}

/** The phone layout hides the table head, so the same sort lives in a select there. */
export function SortSelect({ fields, sort, order, onSort, className }: {
  fields: { field: string; label: string }[];
  sort: string;
  order: SortOrder;
  onSort: (sort: string, order: SortOrder) => void;
  className?: string;
}) {
  const { t } = useI18n();
  const items = Object.fromEntries(
    fields.flatMap(({ field, label }) => (["asc", "desc"] as const).map((o) => [`${field}:${o}`, `${label}: ${t(o === "asc" ? "common.sort_asc" : "common.sort_desc")}`])),
  );
  return (
    <Select value={`${sort}:${order}`} items={items} onValueChange={(v) => { const [s, o] = String(v).split(":"); onSort(s, o as SortOrder); }}>
      <SelectTrigger className={cn("w-full sm:hidden", className)} aria-label={t("common.sort_by")}><SelectValue /></SelectTrigger>
      <SelectContent>
        {Object.entries(items).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}
      </SelectContent>
    </Select>
  );
}
