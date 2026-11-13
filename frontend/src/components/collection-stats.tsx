"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { type FieldRow, recordTitle } from "@/components/field-builder";
import { groupableOf, useNumberFormat } from "@/components/record-groups";
import { andWhere, whereText, type Where } from "@/components/record-rules";
import { records } from "@/lib/api/endpoints";
import type { Collection, Widget } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";

const NUMERIC = ["number", "integer"];
const DATES = ["date", "datetime"];
/** How many values a breakdown draws before "others". */
const TOP = 8;

export const isNumeric = (r: FieldRow) => NUMERIC.includes(r.field.type);
/** Fields a bar chart can count by: values that repeat, as the list groups them. */
export const barFields = (rows: FieldRow[]) => groupableOf(rows).filter((r) => !isNumeric(r) && !DATES.includes(r.field.type));

const asWhere = (w?: Widget["where"]): Where => (w ? (w as Where) : { any: false, rules: [] });

/** The aggregate API for a set of records: a count, numbers over one field, or counts by a field. */
function useAggregate(collectionId: string, where: Where, opts: { sum?: string; group?: string } = {}, enabled = true) {
  const text = whereText(where) || undefined;
  return useQuery({
    queryKey: ["records", collectionId, "aggregate", text, opts.sum, opts.group],
    queryFn: ({ signal }) => records.aggregate({ collection_id: collectionId, where: text, ...opts }, signal),
    enabled,
  });
}

/** A value of a breakdown as it reads: empty, yes/no, or the value. */
function useValueText() {
  const { t } = useI18n();
  return (v: unknown) => (v === null || v === undefined || v === "" ? t("groups.empty") : typeof v === "boolean" ? t(v ? "common.yes" : "common.no") : String(v));
}

/** Counts by value, largest first, the tail folded into "others". */
function Bars({ items, total, label }: { items: { key: unknown; count: number }[]; total: number; label: (v: unknown) => string }) {
  const { t } = useI18n();
  const num = useNumberFormat();
  const top = items.slice(0, TOP);
  const rest = total - top.reduce((n, g) => n + g.count, 0);
  const max = Math.max(1, ...top.map((g) => g.count), rest);
  const rows = [...top.map((g) => ({ text: label(g.key), count: g.count })), ...(rest > 0 ? [{ text: t("analysis.others"), count: rest }] : [])];
  return (
    <ul className="space-y-1.5">
      {rows.map((g, i) => (
        <li key={i} className="grid grid-cols-[minmax(0,8rem)_minmax(0,1fr)_auto] items-center gap-2 text-sm">
          <span className="truncate" title={g.text}>{g.text}</span>
          <span className="h-2 rounded-full bg-primary/60" style={{ width: `${Math.max(2, (g.count / max) * 100)}%` }} aria-hidden />
          <span className="text-xs tabular-nums text-muted-foreground">{num(g.count)}{total > 0 && ` · ${Math.round((g.count / total) * 100)}%`}</span>
        </li>
      ))}
    </ul>
  );
}

function Figures({ items }: { items: [string, string][] }) {
  return (
    <dl className={`grid grid-cols-2 gap-x-4 gap-y-2 ${items.length > 2 ? "sm:grid-cols-4" : ""}`}>
      {items.map(([k, v]) => (
        <div key={k} className="min-w-0">
          <dt className="text-xs text-muted-foreground">{k}</dt>
          <dd className="truncate font-heading text-lg font-semibold tabular-nums" title={v}>{v}</dd>
        </div>
      ))}
    </dl>
  );
}

/** The first or last value of a date field, read from the list sorted by it. */
function useEdge(collectionId: string, where: Where, key: string, order: "asc" | "desc", enabled: boolean) {
  const text = whereText(andWhere(where, [{ field: key, op: "not_empty" }])) || undefined;
  return useQuery({
    queryKey: ["records", collectionId, "edge", key, order, text],
    queryFn: ({ signal }) => records.list({ collection_id: collectionId, page: 1, limit: 1, sort: `data.${key}`, order, where: text }, signal),
    enabled,
    select: (d) => (d.items[0]?.data as Record<string, unknown> | undefined)?.[key],
  });
}

/**
 * One field of the collection, the way a schema browser shows it: how many
 * records fill it, then its numbers, its range of dates or its common values.
 */
export function FieldStats({ collection, row, where, total }: { collection: Collection; row: FieldRow; where: Where; total: number }) {
  const { t, locale } = useI18n();
  const num = useNumberFormat();
  const label = useValueText();
  const type = row.field.type;
  const numeric = isNumeric(row);
  const dated = DATES.includes(type);
  const bars = !numeric && !dated && groupableOf([row]).length > 0;
  const filled = useAggregate(collection.id, andWhere(where, [{ field: row.key, op: "not_empty" }]));
  const sums = useAggregate(collection.id, where, { sum: row.key }, numeric);
  const groups = useAggregate(collection.id, where, { group: row.key }, bars);
  const first = useEdge(collection.id, where, row.key, "asc", dated);
  const last = useEdge(collection.id, where, row.key, "desc", dated);
  const n = filled.data?.items[0]?.count ?? 0;
  const pct = total ? Math.round((n / total) * 100) : 0;
  const date = (v: unknown) => (typeof v !== "string" ? "—" : new Date(type === "date" ? `${v}T00:00:00` : v).toLocaleDateString(locale, { dateStyle: "medium" }));
  const s = sums.data?.items[0];
  const g = groups.data?.items ?? [];
  return (
    <section aria-label={row.field.name} className="space-y-3 rounded-xl border bg-card p-4">
      <header className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h3 className="font-heading text-base font-semibold">{row.field.name}</h3>
        <span className="text-xs text-muted-foreground">{t(`collections.type.${type}` as MessageKey)} · <code className="font-mono">{row.key}</code></span>
      </header>
      <div className="space-y-1">
        <div className="flex justify-between text-xs text-muted-foreground">
          <span>{t("analysis.filled")}</span>
          <span className="tabular-nums">{filled.isPending ? "…" : `${num(n)} / ${num(total)} · ${pct}%`}</span>
        </div>
        <div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary/70" style={{ width: `${pct}%` }} /></div>
      </div>
      {numeric && s && s.sums[row.key] !== undefined && (
        <Figures items={[[t("analysis.min"), num(s.mins[row.key])], [t("analysis.max"), num(s.maxs[row.key])], [t("analysis.avg"), num(s.avgs[row.key])], [t("analysis.sum"), num(s.sums[row.key])]]} />
      )}
      {dated && n > 0 && <Figures items={[[t("analysis.first"), date(first.data)], [t("analysis.last"), date(last.data)]]} />}
      {bars && g.length > 0 && (g.length > 1 && g.every((x) => x.count === 1) ? (
        // Bars of one each say nothing: a name or a code, different in every record.
        <p className="text-xs text-muted-foreground">{t("analysis.all_distinct")}</p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">{t(g.length >= 100 ? "analysis.distinct_many" : "analysis.distinct").replace("{n}", String(g.length))}</p>
          <Bars items={g} total={total} label={label} />
        </>
      ))}
    </section>
  );
}

/** One dashboard widget, drawn from the records it selects. */
export function WidgetView({ collection, widget, rows }: { collection: Collection; widget: Widget; rows: FieldRow[] }) {
  const { t } = useI18n();
  const num = useNumberFormat();
  const label = useValueText();
  const where = asWhere(widget.where);
  const field = rows.find((r) => r.key === widget.field);
  const figure = widget.kind === "sum" || widget.kind === "avg" || widget.kind === "min" || widget.kind === "max";
  const agg = useAggregate(collection.id, where, figure ? { sum: widget.field } : widget.kind === "bars" ? { group: widget.field } : {}, widget.kind !== "list" && (!(figure || widget.kind === "bars") || !!field));
  const totals = useAggregate(collection.id, where, {}, widget.kind === "bars");
  const text = whereText(where) || undefined;
  const list = useQuery({
    queryKey: ["records", collection.id, "widget", widget.id, text],
    queryFn: ({ signal }) => records.list({ collection_id: collection.id, page: 1, limit: 5, sort: "updated_at", order: "desc", where: text }, signal),
    enabled: widget.kind === "list",
  });
  const missing = (figure || widget.kind === "bars") && !field;
  const g = agg.data?.items[0];
  const value = !g ? "—" : widget.kind === "count" ? num(g.count)
    : figure && widget.field && g[({ sum: "sums", avg: "avgs", min: "mins", max: "maxs" } as const)[widget.kind as "sum"]][widget.field] !== undefined
      ? num(g[({ sum: "sums", avg: "avgs", min: "mins", max: "maxs" } as const)[widget.kind as "sum"]][widget.field]) : "—";
  return (
    <section aria-label={widget.title} className="flex h-full min-w-0 flex-col gap-2 rounded-xl border bg-card p-4">
      <h3 className="text-xs font-medium text-muted-foreground">{widget.title}</h3>
      {missing ? <p className="text-sm text-muted-foreground">{t("dashboard.field_gone")}</p>
        : widget.kind === "list" ? (
          list.data?.items.length ? (
            <ul className="divide-y text-sm">
              {list.data.items.map((r) => (
                <li key={r.id}><Link href={`/records/${r.id}`} className="block truncate py-1.5 hover:underline">{recordTitle(collection.schema ?? null, r.data) ?? r.id.slice(0, 8)}</Link></li>
              ))}
            </ul>
          ) : <p className="text-sm text-muted-foreground">{list.isPending ? "…" : t("dashboard.no_records")}</p>
        ) : widget.kind === "bars" ? (
          agg.data?.items.length ? <Bars items={agg.data.items} total={totals.data?.items[0]?.count ?? 0} label={label} /> : <p className="text-sm text-muted-foreground">{agg.isPending ? "…" : t("dashboard.no_records")}</p>
        ) : (
          <p className="font-heading text-3xl font-semibold tabular-nums">{agg.isPending ? "…" : value}</p>
        )}
    </section>
  );
}

export const WIDGET_KINDS: Widget["kind"][] = ["count", "sum", "avg", "min", "max", "bars", "list"];
/** Whether a kind needs a field, and which fields fit it. */
export function fieldsFor(kind: Widget["kind"], rows: FieldRow[]): FieldRow[] | null {
  if (kind === "bars") return barFields(rows);
  if (kind === "sum" || kind === "avg" || kind === "min" || kind === "max") return rows.filter((r) => !r.advanced && isNumeric(r));
  return null;
}

