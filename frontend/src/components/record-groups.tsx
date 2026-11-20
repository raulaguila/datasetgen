"use client";

import type { CollectionRef } from "@/lib/paths";
import { useQuery } from "@tanstack/react-query";
import { type FieldRow } from "@/components/field-builder";
import { fieldFigure } from "@/components/format-figure";
import { records } from "@/lib/api/endpoints";
import { useI18n } from "@/lib/i18n/provider";

type Selection = { collection: CollectionRef; search?: string; filter?: string; where?: string };

/** Numeric fields among these, up to the five the server sums. */
export const numericOf = (rows: FieldRow[]) => rows.filter((r) => !r.advanced && (r.field.type === "number" || r.field.type === "integer")).slice(0, 5);

/** The fields a list can group by: values that repeat across records. */
export const groupableOf = (rows: FieldRow[]) => rows.filter((r) => !r.advanced && ["string", "enum", "boolean", "date", "integer", "email", "reference"].includes(r.field.type));

export function useNumberFormat() {
  const { locale } = useI18n();
  const nf = new Intl.NumberFormat(locale, { maximumFractionDigits: 2 });
  // A fraction always shows two places, so 1.234,5 and 4.430,42 line up in one column.
  const frac = new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  return (n: number) => (Number.isInteger(n) ? nf : frac).format(n);
}

/** The selection's records counted by one field, largest first; picking one filters the list to it. */
export function RecordGroups({ selection, group, fields, label, onPick }: {
  selection: Selection;
  group: FieldRow;
  fields: FieldRow[];
  /** How a group's value reads (references show their record's title). */
  label: (key: unknown) => string;
  onPick: (key: unknown) => void;
}) {
  const { t, locale } = useI18n();
  const num = useNumberFormat();
  const numeric = numericOf(fields);
  const q = useQuery({
    queryKey: ["records", selection.collection.id, "groups", group.key, selection.search, selection.filter, selection.where],
    queryFn: ({ signal }) => records.aggregate(selection.collection, { search: selection.search, filter: selection.filter, where: selection.where, group: group.key, sum: numeric.map((f) => f.key).join(",") || undefined }, signal),
  });
  const items = q.data?.items ?? [];
  const max = Math.max(1, ...items.map((g) => g.count));
  if (q.isPending) return <p className="text-sm text-muted-foreground">{t("views.loading")}</p>;
  if (!items.length) return null;
  return (
    <section aria-label={t("groups.title").replace("{field}", group.field.name)} className="rounded-lg border">
      <h2 className="border-b px-3 py-2 text-sm font-medium">{t("groups.title").replace("{field}", group.field.name)}</h2>
      <ul className="max-h-72 divide-y overflow-auto">
        {items.map((g, i) => (
          <li key={i}>
            <button type="button" className="grid w-full grid-cols-[minmax(0,12rem)_minmax(0,1fr)_auto] items-center gap-3 px-3 py-1.5 text-left text-sm hover:bg-muted/60 max-sm:grid-cols-[minmax(0,1fr)_auto]" onClick={() => onPick(g.key)}>
              <span className="truncate font-medium">{g.key === null || g.key === undefined || g.key === "" ? t("groups.empty") : label(g.key)}</span>
              <span className="h-2 rounded-full bg-primary/60 max-sm:hidden" style={{ width: `${(g.count / max) * 100}%` }} aria-hidden />
              <span className="whitespace-nowrap text-xs text-muted-foreground">
                {g.count}
                {numeric.filter((f) => g.sums[f.key] !== undefined).map((f) => ` · ${f.field.name} ${fieldFigure(f.field, g.sums[f.key], locale, num)}`)}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
