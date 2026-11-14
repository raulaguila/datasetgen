"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { type FieldRow, recordTitle } from "@/components/field-builder";
import { groupableOf, useNumberFormat } from "@/components/record-groups";
import { andWhere, bothWhere, viewWhere, whereText, type Rule, type Where } from "@/components/record-rules";
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
type Slice = { text: string; count: number; href?: string };

/** The top values and the rest folded into "others"; with `href` each value opens its records. */
function useSlices(items: { key: unknown; count: number }[], total: number, label: (v: unknown) => string, href?: (key: unknown) => string): Slice[] {
  const { t } = useI18n();
  const top = items.slice(0, TOP);
  const rest = total - top.reduce((n, g) => n + g.count, 0);
  return [...top.map((g) => ({ text: label(g.key), count: g.count, href: href?.(g.key) })), ...(rest > 0 ? [{ text: t("analysis.others"), count: rest }] : [])];
}

const pctOf = (n: number, total: number) => (total > 0 ? ` · ${Math.round((n / total) * 100)}%` : "");

/** A slice's name: a link to its records when it has one. */
function SliceName({ s, className }: { s: Slice; className?: string }) {
  return s.href ? <Link href={s.href} className={`truncate hover:underline ${className ?? ""}`} title={s.text}>{s.text}</Link> : <span className={`truncate ${className ?? ""}`} title={s.text}>{s.text}</span>;
}

/** Counts by value, largest first, the tail folded into "others". */
function Bars({ items, total, label, href }: { items: { key: unknown; count: number }[]; total: number; label: (v: unknown) => string; href?: (key: unknown) => string }) {
  const num = useNumberFormat();
  const rows = useSlices(items, total, label, href);
  const max = Math.max(1, ...rows.map((g) => g.count));
  return (
    <ul className="space-y-1.5">
      {rows.map((g, i) => (
        <li key={i} className="grid grid-cols-[minmax(0,8rem)_minmax(0,1fr)_auto] items-center gap-2 text-sm" title={`${g.text}: ${num(g.count)}${pctOf(g.count, total)}`}>
          <SliceName s={g} />
          <span className="h-2 rounded-full bg-primary/60" style={{ width: `${Math.max(2, (g.count / max) * 100)}%` }} aria-hidden />
          <span className="text-xs tabular-nums text-muted-foreground">{num(g.count)}{pctOf(g.count, total)}</span>
        </li>
      ))}
    </ul>
  );
}

/** Shares of the whole as a ring, each value in its fixed series hue, named in the legend beside it. */
function Donut({ items, total, label, href }: { items: { key: unknown; count: number }[]; total: number; label: (v: unknown) => string; href?: (key: unknown) => string }) {
  const num = useNumberFormat();
  const rows = useSlices(items, total, label, href);
  const sum = Math.max(1, rows.reduce((n, g) => n + g.count, 0));
  const r = 40;
  const c = 2 * Math.PI * r;
  const gap = rows.length > 1 ? 1.2 : 0; // the surface between slices
  // "Others" is the folded tail: neutral, never a ninth hue.
  const named = Math.min(items.length, TOP);
  const color = (i: number) => (i < named ? `var(--series-${i + 1})` : "var(--muted-foreground)");
  // Where each slice starts along the ring: the counts before it.
  const starts = rows.map((_, i) => rows.slice(0, i).reduce((n, g) => n + (g.count / sum) * c, 0));
  return (
    <div className="flex flex-wrap items-center gap-4">
      <svg viewBox="0 0 100 100" className="size-24 shrink-0 -rotate-90" role="img" aria-label={rows.map((g) => `${g.text} ${num(g.count)}`).join(", ")}>
        {rows.map((g, i) => {
          const len = (g.count / sum) * c;
          return (
            <circle key={i} cx="50" cy="50" r={r} fill="none" stroke={color(i)} strokeWidth="16" strokeDasharray={`${Math.max(0, len - gap)} ${c}`} strokeDashoffset={-starts[i]}>
              <title>{`${g.text}: ${num(g.count)}${pctOf(g.count, total)}`}</title>
            </circle>
          );
        })}
      </svg>
      <ul className="min-w-36 flex-1 space-y-1 text-sm">
        {rows.map((g, i) => (
          <li key={i} className="flex items-center gap-2">
            <span className="size-2.5 shrink-0 rounded-full" style={{ background: color(i) }} aria-hidden />
            <SliceName s={g} className="min-w-0 flex-1" />
            <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{num(g.count)}{pctOf(g.count, total)}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

/** The same counts as rows of a table. */
function CountTable({ items, total, label, href, head }: { items: { key: unknown; count: number }[]; total: number; label: (v: unknown) => string; href?: (key: unknown) => string; head: string }) {
  const { t } = useI18n();
  const num = useNumberFormat();
  const rows = useSlices(items, total, label, href);
  return (
    <table className="w-full text-sm">
      <thead><tr className="text-left text-xs text-muted-foreground"><th className="py-1 font-medium">{head}</th><th className="py-1 text-right font-medium">{t("dashboard.kind.count")}</th><th className="py-1 text-right font-medium">%</th></tr></thead>
      <tbody className="divide-y">
        {rows.map((g, i) => (
          <tr key={i}><td className="max-w-0 py-1.5"><SliceName s={g} className="block" /></td><td className="py-1.5 text-right tabular-nums">{num(g.count)}</td><td className="py-1.5 text-right tabular-nums text-muted-foreground">{total ? Math.round((g.count / total) * 100) : 0}%</td></tr>
        ))}
      </tbody>
    </table>
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
function Series({ points, bucket, line = false }: { points: { at: string; count: number }[]; bucket: StatsBucket; line?: boolean }) {
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
      {line ? (
        // A 2px line over the buckets; each bucket's column is its hover target.
        <div className="relative h-24" role="img" aria-label={`${title}: ${all.map((p) => `${fmt(p.at)} ${num(p.count)}`).join(", ")}`}>
          <svg viewBox={`0 0 ${Math.max(1, all.length - 1)} 100`} preserveAspectRatio="none" className="absolute inset-0 size-full overflow-visible">
            <polyline fill="none" stroke="var(--primary)" strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinejoin="round" points={all.map((p, i) => `${i},${100 - (p.count / max) * 96}`).join(" ")} />
          </svg>
          <div className="absolute inset-0 flex">{all.map((p) => <div key={p.at} className="flex-1 hover:bg-primary/10" title={`${fmt(p.at)}: ${num(p.count)}`} />)}</div>
        </div>
      ) : (
        <div className="flex h-24 items-end gap-px" role="img" aria-label={`${title}: ${all.map((p) => `${fmt(p.at)} ${num(p.count)}`).join(", ")}`}>
          {all.map((p) => (
            <div key={p.at} title={`${fmt(p.at)}: ${num(p.count)}`} className="min-w-px flex-1 rounded-t-sm bg-primary/60 hover:bg-primary" style={{ height: p.count ? `${Math.max(3, (p.count / max) * 100)}%` : 0 }} />
          ))}
        </div>
      )}
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
/** A field whose card holds only how filled it is: no figures, dates or bars worth drawing. */
export const isPlain = (profile?: FieldProfile) => {
  const top = profile?.top ?? [];
  return profile?.sum === undefined && !profile?.first && (top.length === 0 || ((profile?.distinct ?? 0) > 1 && top.every((x) => x.count === 1)));
};

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

/** A widget's number the way its format reads it. */
export function formatFigure(v: number, locale: string, format: Widget["format"] = "number", decimals?: number): string {
  const fixed = decimals === undefined ? {} : { minimumFractionDigits: decimals, maximumFractionDigits: decimals };
  if (format === "compact") return new Intl.NumberFormat(locale, { notation: "compact", maximumFractionDigits: decimals ?? 1 }).format(v);
  // shortcut: the money follows the language (BRL / USD), fine while each install bills in one currency.
  if (format === "currency") return new Intl.NumberFormat(locale, { style: "currency", currency: locale === "pt-BR" ? "BRL" : "USD", ...fixed }).format(v);
  // The stored number is already the percentage (12.5 → 12,5%).
  if (format === "percent") return `${new Intl.NumberFormat(locale, { maximumFractionDigits: decimals ?? 1, ...fixed }).format(v)}%`;
  return new Intl.NumberFormat(locale, { maximumFractionDigits: decimals ?? 2, ...fixed }).format(v);
}

/** The records list filtered to `where`: where a widget's figure or bar leads. */
export const recordsHref = (collectionId: string, where: Where) => {
  const text = whereText(where);
  return `/collections/${collectionId}${text ? `?where=${encodeURIComponent(text)}` : ""}`;
};

/** The rule picking one value of a breakdown: empty values have their own. */
const valueRule = (field: string, key: unknown): Rule => (key === null || key === undefined || key === "" ? { field, op: "empty" } : { field, op: "eq", value: key });

/**
 * One dashboard widget, drawn from the records it selects in one request. `scope`
 * is the dashboard's own filter, joined to the widget's conditions.
 */
export function WidgetView({ collection, widget, rows, scope }: { collection: Collection; widget: Widget; rows: FieldRow[]; scope?: Where }) {
  const { t, locale } = useI18n();
  const label = useValueText();
  const joined = scope ? bothWhere(asWhere(widget.where), scope) : asWhere(widget.where);
  const where = joined ?? asWhere(widget.where);
  const field = rows.find((r) => r.key === widget.field);
  const figure = widget.kind === "sum" || widget.kind === "avg" || widget.kind === "min" || widget.kind === "max";
  const needsField = widget.kind !== "count" && widget.kind !== "list";
  const bucket = widget.bucket ?? "month";
  const st = useStats(collection.id, where, {
    numbers: figure && field ? [field] : [],
    values: widget.kind === "bars" && field ? [field] : [],
    dates: widget.kind === "timeline" && field ? [field] : [],
    bucket,
  }, !!joined && widget.kind !== "list" && (!needsField || !!field));
  const text = whereText(where) || undefined;
  const list = useQuery({
    queryKey: ["records", collection.id, "widget", widget.id, text],
    queryFn: ({ signal }) => records.list({ collection_id: collection.id, page: 1, limit: 5, sort: "updated_at", order: "desc", where: text }, signal),
    enabled: !!joined && widget.kind === "list",
  });
  const p = field ? st.data?.fields[field.key] : undefined;
  const figureValue = figure ? p?.[widget.kind as "sum" | "avg" | "min" | "max"] : undefined;
  const shown = (v: number) => formatFigure(v, locale, widget.format, widget.decimals);
  const value = widget.kind === "count" ? (st.data ? shown(st.data.total) : "—") : figureValue !== undefined ? shown(figureValue) : "—";
  // Drill-down: the figure opens the records it counts; a bar, the records of its value.
  const all = recordsHref(collection.id, figure && field ? andWhere(where, [{ field: field.key, op: "not_empty" }]) : where);
  const byValue = field ? (key: unknown) => recordsHref(collection.id, andWhere(where, [valueRule(field.key, key)])) : undefined;
  const none = <p className="text-sm text-muted-foreground">{st.isPending ? "…" : t("dashboard.no_records")}</p>;
  const total = st.data?.total ?? 0;
  return (
    <section aria-label={widget.title} className="flex h-full min-w-0 flex-col gap-2 rounded-xl border bg-card p-4">
      <h3 className="text-xs font-medium text-muted-foreground">{widget.title}</h3>
      {!joined ? <p className="text-sm text-muted-foreground">{t("dashboard.filter_conflict")}</p>
        : needsField && !field ? <p className="text-sm text-muted-foreground">{t("dashboard.field_gone")}</p>
        : widget.kind === "list" ? (
          list.data?.items.length ? (
            <ul className="divide-y text-sm">
              {list.data.items.map((r) => (
                <li key={r.id}><Link href={`/records/${r.id}`} className="block truncate py-1.5 hover:underline">{recordTitle(collection.schema ?? null, r.data) ?? r.id.slice(0, 8)}</Link></li>
              ))}
            </ul>
          ) : <p className="text-sm text-muted-foreground">{list.isPending ? "…" : t("dashboard.no_records")}</p>
        ) : widget.kind === "bars" ? (
          !p?.top?.length ? none
            : widget.visual === "donut" ? <Donut items={p.top} total={total} label={label} href={byValue} />
            : widget.visual === "table" ? <CountTable items={p.top} total={total} label={label} href={byValue} head={field?.field.name ?? ""} />
            : <Bars items={p.top} total={total} label={label} href={byValue} />
        ) : widget.kind === "timeline" ? (
          p?.series?.length ? <Series points={p.series} bucket={bucket} line={widget.visual === "line"} /> : none
        ) : (
          <Link href={all} className="w-fit font-heading text-3xl font-semibold tabular-nums hover:underline" title={t("dashboard.open_records")}>{st.isPending ? "…" : value}</Link>
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
