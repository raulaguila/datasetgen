"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { type FieldRow, recordTitle } from "@/components/field-builder";
import { groupableOf, useNumberFormat } from "@/components/record-groups";
import { viewWhere, whereText, type Where } from "@/components/record-rules";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { records, tools } from "@/lib/api/endpoints";
import type { Collection, FieldProfile, StatsBucket, Widget } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";

const NUMERIC = ["number", "integer"];
const DATES = ["date", "datetime"];
/** How many values a breakdown draws before "others"; the API sends this many. */
const TOP = 8;
/** The most bars a series draws; past it the oldest fall off. */
const MAX_BARS = 400;

export const BUCKETS: StatsBucket[] = ["day", "week", "month", "year"];
export const isNumeric = (r: FieldRow) => NUMERIC.includes(r.field.type);
export const isDate = (r: FieldRow) => DATES.includes(r.field.type);
/** Fields a bar chart can count by: values that repeat, as the list groups them. */
export const barFields = (rows: FieldRow[]) => groupableOf(rows).filter((r) => !isNumeric(r) && !isDate(r));

const asWhere = (w?: Widget["where"]): Where => (w ? (w as Where) : { any: false, rules: [] });
const csv = (rows: FieldRow[]) => rows.map((r) => r.key).join(",") || undefined;

/** One request profiling the selected records over the given fields. */
export function useStats(collectionId: string, where: Where, q: { filled?: FieldRow[]; numbers?: FieldRow[]; dates?: FieldRow[]; values?: FieldRow[]; bucket?: StatsBucket }, enabled = true) {
  const text = whereText(where) || undefined;
  const p = { collection_id: collectionId, where: text, filled: csv(q.filled ?? []), numbers: csv(q.numbers ?? []), dates: csv(q.dates ?? []), values: csv(q.values ?? []), bucket: q.dates?.length ? q.bucket : undefined };
  return useQuery({
    queryKey: ["records", collectionId, "stats", p],
    queryFn: ({ signal }) => records.stats(p, signal),
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

/** The buckets from the first to the last, empty ones as zero, so gaps show as gaps. */
export function fillSeries(points: { at: string; count: number }[], bucket: StatsBucket): { at: string; count: number }[] {
  if (points.length < 2) return points;
  const counts = new Map(points.map((p) => [p.at, p.count]));
  const out: { at: string; count: number }[] = [];
  const d = new Date(`${points[0].at}T00:00:00Z`);
  const end = points[points.length - 1].at;
  for (let at = points[0].at; at <= end && out.length < 5000; at = d.toISOString().slice(0, 10)) {
    out.push({ at, count: counts.get(at) ?? 0 });
    if (bucket === "day") d.setUTCDate(d.getUTCDate() + 1);
    else if (bucket === "week") d.setUTCDate(d.getUTCDate() + 7);
    else if (bucket === "month") d.setUTCMonth(d.getUTCMonth() + 1);
    else d.setUTCFullYear(d.getUTCFullYear() + 1);
  }
  return out.slice(-MAX_BARS);
}

/** Records per bucket as columns, oldest on the left. */
function Series({ points, bucket }: { points: { at: string; count: number }[]; bucket: StatsBucket }) {
  const { t, locale } = useI18n();
  const num = useNumberFormat();
  const all = fillSeries(points, bucket);
  const max = Math.max(1, ...all.map((p) => p.count));
  const opts: Intl.DateTimeFormatOptions = bucket === "year" ? { year: "numeric", timeZone: "UTC" } : bucket === "month" ? { month: "short", year: "numeric", timeZone: "UTC" } : { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" };
  const fmt = (at: string) => new Date(`${at}T00:00:00Z`).toLocaleDateString(locale, opts);
  const title = t("analysis.over_time").replace("{bucket}", t(`analysis.bucket.${bucket}` as MessageKey));
  if (!all.length) return null;
  return (
    <figure className="space-y-1">
      <figcaption className="text-xs text-muted-foreground">{title}</figcaption>
      <div className="flex h-24 items-end gap-px" role="img" aria-label={`${title}: ${all.map((p) => `${fmt(p.at)} ${num(p.count)}`).join(", ")}`}>
        {all.map((p) => (
          <div key={p.at} title={`${fmt(p.at)}: ${num(p.count)}`} className="min-w-px flex-1 rounded-t-sm bg-primary/60 hover:bg-primary" style={{ height: p.count ? `${Math.max(3, (p.count / max) * 100)}%` : 0 }} />
        ))}
      </div>
      <div className="flex justify-between text-xs tabular-nums text-muted-foreground">
        <span>{fmt(all[0].at)}</span>
        {all.length > 1 && <span>{fmt(all[all.length - 1].at)}</span>}
      </div>
    </figure>
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

/**
 * One field of the collection, the way a schema browser shows it: how many
 * records fill it, then its numbers, its dates over time or its common values.
 */
export function FieldStats({ row, profile, total, bucket }: { row: FieldRow; profile?: FieldProfile; total: number; bucket: StatsBucket }) {
  const { t, locale } = useI18n();
  const num = useNumberFormat();
  const label = useValueText();
  const type = row.field.type;
  const n = profile?.filled ?? 0;
  const pct = total ? Math.round((n / total) * 100) : 0;
  const date = (v?: string) => (v ? new Date(`${v}T00:00:00`).toLocaleDateString(locale, { dateStyle: "medium" }) : "—");
  const top = profile?.top ?? [];
  const distinct = profile?.distinct ?? 0;
  return (
    <section aria-label={row.field.name} className="space-y-3 rounded-xl border bg-card p-4">
      <header className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1">
        <h3 className="font-heading text-base font-semibold">{row.field.name}</h3>
        <span className="text-xs text-muted-foreground">{t(`collections.type.${type}` as MessageKey)} · <code className="font-mono">{row.key}</code></span>
      </header>
      <div className="space-y-1">
        <div className="flex justify-between text-xs text-muted-foreground">
          <span>{t("analysis.filled")}</span>
          <span className="tabular-nums">{`${num(n)} / ${num(total)} · ${pct}%`}</span>
        </div>
        <div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary/70" style={{ width: `${pct}%` }} /></div>
      </div>
      {profile?.sum !== undefined && (
        <Figures items={[[t("analysis.min"), num(profile.min ?? 0)], [t("analysis.max"), num(profile.max ?? 0)], [t("analysis.avg"), num(profile.avg ?? 0)], [t("analysis.sum"), num(profile.sum)]]} />
      )}
      {profile?.first && (
        <>
          <Figures items={[[t("analysis.first"), date(profile.first)], [t("analysis.last"), date(profile.last)]]} />
          <Series points={profile.series ?? []} bucket={bucket} />
        </>
      )}
      {top.length > 0 && (distinct > 1 && top.every((x) => x.count === 1) ? (
        // Bars of one each say nothing: a name or a code, different in every record.
        <p className="text-xs text-muted-foreground">{t("analysis.all_distinct")}</p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">{t("analysis.distinct").replace("{n}", num(distinct))}</p>
          <Bars items={top} total={total} label={label} />
        </>
      ))}
    </section>
  );
}

/** A picker that copies a saved view's conditions; hidden while there are none. */
export function ViewConditions({ collectionId, onPick }: { collectionId: string; onPick: (w: Where) => void }) {
  const { t } = useI18n();
  const views = useQuery({ queryKey: ["views", collectionId], queryFn: () => tools.views(collectionId) });
  const usable = (views.data?.items ?? []).filter((v) => { const w = viewWhere(v.query); return w.rules.length || w.groups?.length; });
  if (!usable.length) return null;
  return (
    <Select value={null} items={Object.fromEntries(usable.map((v) => [v.id, v.name]))} onValueChange={(id) => { const v = usable.find((x) => x.id === id); if (v) onPick(viewWhere(v.query)); }}>
      <SelectTrigger size="sm" aria-label={t("analysis.from_view")}><SelectValue placeholder={t("analysis.from_view")} /></SelectTrigger>
      <SelectContent>{usable.map((v) => <SelectItem key={v.id} value={v.id}>{v.name}</SelectItem>)}</SelectContent>
    </Select>
  );
}

/** One dashboard widget, drawn from the records it selects in one request. */
export function WidgetView({ collection, widget, rows }: { collection: Collection; widget: Widget; rows: FieldRow[] }) {
  const { t } = useI18n();
  const num = useNumberFormat();
  const label = useValueText();
  const where = asWhere(widget.where);
  const field = rows.find((r) => r.key === widget.field);
  const figure = widget.kind === "sum" || widget.kind === "avg" || widget.kind === "min" || widget.kind === "max";
  const needsField = widget.kind !== "count" && widget.kind !== "list";
  const bucket = widget.bucket ?? "month";
  const st = useStats(collection.id, where, {
    numbers: figure && field ? [field] : [],
    values: widget.kind === "bars" && field ? [field] : [],
    dates: widget.kind === "timeline" && field ? [field] : [],
    bucket,
  }, widget.kind !== "list" && (!needsField || !!field));
  const text = whereText(where) || undefined;
  const list = useQuery({
    queryKey: ["records", collection.id, "widget", widget.id, text],
    queryFn: ({ signal }) => records.list({ collection_id: collection.id, page: 1, limit: 5, sort: "updated_at", order: "desc", where: text }, signal),
    enabled: widget.kind === "list",
  });
  const p = field ? st.data?.fields[field.key] : undefined;
  const figureValue = figure ? p?.[widget.kind as "sum" | "avg" | "min" | "max"] : undefined;
  const value = widget.kind === "count" ? (st.data ? num(st.data.total) : "—") : figureValue !== undefined ? num(figureValue) : "—";
  const none = <p className="text-sm text-muted-foreground">{st.isPending ? "…" : t("dashboard.no_records")}</p>;
  return (
    <section aria-label={widget.title} className="flex h-full min-w-0 flex-col gap-2 rounded-xl border bg-card p-4">
      <h3 className="text-xs font-medium text-muted-foreground">{widget.title}</h3>
      {needsField && !field ? <p className="text-sm text-muted-foreground">{t("dashboard.field_gone")}</p>
        : widget.kind === "list" ? (
          list.data?.items.length ? (
            <ul className="divide-y text-sm">
              {list.data.items.map((r) => (
                <li key={r.id}><Link href={`/records/${r.id}`} className="block truncate py-1.5 hover:underline">{recordTitle(collection.schema ?? null, r.data) ?? r.id.slice(0, 8)}</Link></li>
              ))}
            </ul>
          ) : <p className="text-sm text-muted-foreground">{list.isPending ? "…" : t("dashboard.no_records")}</p>
        ) : widget.kind === "bars" ? (
          p?.top?.length ? <Bars items={p.top} total={st.data?.total ?? 0} label={label} /> : none
        ) : widget.kind === "timeline" ? (
          p?.series?.length ? <Series points={p.series} bucket={bucket} /> : none
        ) : (
          <p className="font-heading text-3xl font-semibold tabular-nums">{st.isPending ? "…" : value}</p>
        )}
    </section>
  );
}

export const WIDGET_KINDS: Widget["kind"][] = ["count", "sum", "avg", "min", "max", "bars", "timeline", "list"];
/** Whether a kind needs a field, and which fields fit it. */
export function fieldsFor(kind: Widget["kind"], rows: FieldRow[]): FieldRow[] | null {
  if (kind === "bars") return barFields(rows);
  if (kind === "timeline") return rows.filter((r) => !r.advanced && isDate(r));
  if (kind === "sum" || kind === "avg" || kind === "min" || kind === "max") return rows.filter((r) => !r.advanced && isNumeric(r));
  return null;
}
