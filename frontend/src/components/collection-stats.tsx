"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { type FieldRow, recordTitle } from "@/components/field-builder";
import { fieldFigure } from "@/components/format-figure";
import { optionCss } from "@/components/option-chip";
import { groupableOf, useNumberFormat } from "@/components/record-groups";
import { andWhere, bothWhere, viewWhere, whereText, type Rule, type Where } from "@/components/record-rules";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { records, tools } from "@/lib/api/endpoints";
import type { Collection, FieldProfile, StatsBucket, Widget } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";
import { formatFigure } from "@/components/format-figure";
import { collectionPath, recordPath } from "@/lib/paths";

export { formatFigure };

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
type Slice = { text: string; count: number; href?: string; empty?: boolean };

/** The top values and the rest folded into "others"; with `href` each value opens its records. */
function useSlices(items: { key: unknown; count: number }[], total: number, label: (v: unknown) => string, href?: (key: unknown) => string): Slice[] {
  const { t } = useI18n();
  const blank = (k: unknown) => k === null || k === undefined || k === "";
  const top = items.slice(0, TOP);
  const rest = total - top.reduce((n, g) => n + g.count, 0);
  const slice = (g: { key: unknown; count: number }): Slice => ({ text: label(g.key), count: g.count, href: href?.(g.key), empty: blank(g.key) });
  // The records without a value close the list: they are a gap to fill, not one of the values.
  return [...top.filter((g) => !blank(g.key)).map(slice), ...(rest > 0 ? [{ text: t("analysis.others"), count: rest }] : []), ...top.filter((g) => blank(g.key)).map(slice)];
}

/** A share in whole percent; a few records out of many read "<1%", never "0%". */
export const percent = (n: number, total: number) => {
  if (total <= 0) return "0%";
  const p = Math.round((n / total) * 100);
  return n > 0 && p === 0 ? "<1%" : `${p}%`;
};
const pctOf = (n: number, total: number) => (total > 0 ? ` · ${percent(n, total)}` : "");
/**
 * Nearly every filled record holds its own value (names, codes, revisions): counts and sums of them say nothing.
 * A handful of records proves nothing, so it takes 20 at least.
 */
export const mostlyDistinct = (distinct: number, filled: number) => filled >= 20 && distinct >= filled * 0.9;

/** A slice's name: a link to its records when it has one. */
function SliceName({ s, className: extra }: { s: Slice; className?: string }) {
  const className = cn(extra, s.empty && "italic text-muted-foreground");
  return s.href ? <Link href={s.href} className={`truncate hover:underline ${className ?? ""}`} title={s.text}>{s.text}</Link> : <span className={`truncate ${className ?? ""}`} title={s.text}>{s.text}</span>;
}

/** How a breakdown draws: each value's own color (a list option's), and how its figure reads (a sum's format). */
type Look = { color?: (key: unknown) => string | undefined; fmt?: (n: number) => string };

/** Counts (or sums) by value, largest first, the tail folded into "others". */
function Bars({ items, total, label, href, color, fmt }: { items: { key: unknown; count: number }[]; total: number; label: (v: unknown) => string; href?: (key: unknown) => string } & Look) {
  const plain = useNumberFormat();
  const num = fmt ?? plain;
  const rows = useSlices(items, total, label, href);
  const keys = items.slice(0, TOP).map((g) => label(g.key));
  const max = Math.max(1, ...rows.map((g) => g.count));
  return (
    <ul className="space-y-1.5">
      {rows.map((g, i) => (
        <li key={i} className="grid grid-cols-[minmax(0,11rem)_minmax(0,1fr)_auto] items-center gap-2 text-sm" title={`${g.text}: ${num(g.count)}${pctOf(g.count, total)}`}>
          <SliceName s={g} />
          <span className="h-2 rounded-full bg-primary/60" style={{ width: `${Math.max(2, (g.count / max) * 100)}%`, background: color?.(items[keys.indexOf(g.text)]?.key) }} aria-hidden />
          <span className="text-xs tabular-nums text-muted-foreground">{num(g.count)}{pctOf(g.count, total)}</span>
        </li>
      ))}
    </ul>
  );
}

/** Shares of the whole as a ring, each value in its fixed series hue, named in the legend beside it. */
function Donut({ items, total, label, href, color: own, fmt }: { items: { key: unknown; count: number }[]; total: number; label: (v: unknown) => string; href?: (key: unknown) => string } & Look) {
  const plain = useNumberFormat();
  const num = fmt ?? plain;
  const rows = useSlices(items, total, label, href);
  const sum = Math.max(1, rows.reduce((n, g) => n + g.count, 0));
  const r = 40;
  const c = 2 * Math.PI * r;
  const gap = rows.length > 1 ? 1.2 : 0; // the surface between slices
  // "Others" is the folded tail: neutral, never a ninth hue.
  const named = Math.min(items.length, TOP);
  // A list option keeps its own color; the others take the series hues in order.
  const keyOf = (g: Slice) => items.find((x) => label(x.key) === g.text)?.key;
  const color = (i: number) => (i < named ? own?.(keyOf(rows[i])) ?? `var(--series-${i + 1})` : "var(--muted-foreground)");
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
function CountTable({ items, total, label, href, head, fmt, measure }: { items: { key: unknown; count: number }[]; total: number; label: (v: unknown) => string; href?: (key: unknown) => string; head: string; measure?: string } & Look) {
  const { t } = useI18n();
  const plain = useNumberFormat();
  const num = fmt ?? plain;
  const rows = useSlices(items, total, label, href);
  return (
    <table className="w-full text-sm">
      <thead><tr className="text-left text-xs text-muted-foreground"><th className="py-1 font-medium">{head}</th><th className="py-1 text-right font-medium">{measure ?? t("dashboard.kind.count")}</th>{total > 0 && <th className="py-1 text-right font-medium">%</th>}</tr></thead>
      <tbody className="divide-y">
        {rows.map((g, i) => (
          <tr key={i}><td className="max-w-0 py-1.5"><SliceName s={g} className="block" /></td><td className="py-1.5 text-right tabular-nums">{num(g.count)}</td>{total > 0 && <td className="py-1.5 text-right tabular-nums text-muted-foreground">{percent(g.count, total)}</td>}</tr>
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

/** Records per bucket as columns, oldest on the left; the top of the scale on the left edge, the pointed bucket's figure in the caption. */
function Series({ points, bucket, line = false }: { points: { at: string; count: number }[]; bucket: StatsBucket; line?: boolean }) {
  const { t, locale } = useI18n();
  const num = useNumberFormat();
  const [hover, setHover] = useState<number | null>(null);
  const all = fillSeries(points, bucket);
  const max = Math.max(1, ...all.map((p) => p.count));
  const opts: Intl.DateTimeFormatOptions = bucket === "year" ? { year: "numeric", timeZone: "UTC" } : bucket === "month" ? { month: "short", year: "numeric", timeZone: "UTC" } : { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" };
  const fmt = (at: string) => new Date(`${at}T00:00:00Z`).toLocaleDateString(locale, opts);
  const title = t("analysis.over_time").replace("{bucket}", t(`analysis.bucket.${bucket}` as MessageKey));
  if (!all.length) return null;
  // Past a few dozen buckets columns get thinner than a pixel and push the card wide: a line fits any count.
  const asLine = line || all.length > 60;
  const pointed = hover !== null ? all[hover] : null;
  const label = `${title}: ${all.map((p) => `${fmt(p.at)} ${num(p.count)}`).join(", ")}`;
  return (
    <figure className="space-y-1">
      <figcaption className="flex justify-between gap-2 text-xs text-muted-foreground">
        <span>{title}</span>
        {/* The pointed bucket reads here, at once, not in a slow native tooltip. */}
        <span className="tabular-nums text-foreground" aria-live="polite">{pointed ? `${fmt(pointed.at)}: ${num(pointed.count)}` : ""}</span>
      </figcaption>
      <div className="grid grid-cols-[auto_minmax(0,1fr)] gap-2">
        {/* The scale: its top and zero, with a faint line at the top. */}
        <div className="flex h-24 flex-col justify-between text-right text-xs leading-none tabular-nums text-muted-foreground" aria-hidden><span>{num(max)}</span><span>0</span></div>
        <div className="relative h-24 border-t border-dashed border-border" role="img" aria-label={label} onMouseLeave={() => setHover(null)}>
          {asLine ? (
            <>
              <svg viewBox={`0 0 ${Math.max(1, all.length - 1)} 100`} preserveAspectRatio="none" className="absolute inset-0 size-full overflow-visible">
                <polyline fill="none" stroke="var(--primary)" strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinejoin="round" points={all.map((p, i) => `${i},${100 - (p.count / max) * 100}`).join(" ")} />
              </svg>
              <div className="absolute inset-0 flex">{all.map((p, i) => <div key={p.at} className={cn("flex-1", hover === i && "bg-primary/10")} onMouseEnter={() => setHover(i)} />)}</div>
            </>
          ) : (
            <div className="flex h-full items-end gap-px">
              {all.map((p, i) => (
                <div key={p.at} onMouseEnter={() => setHover(i)} className="flex h-full min-w-px flex-1 items-end">
                  <div className={cn("w-full rounded-t-sm", hover === i ? "bg-primary" : "bg-primary/60")} style={{ height: p.count ? `${Math.max(3, (p.count / max) * 100)}%` : 0 }} />
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
      <div className="flex justify-between pl-8 text-xs tabular-nums text-muted-foreground">
        <span>{fmt(all[0].at)}</span>
        {all.length > 1 && <span>{fmt(all[all.length - 1].at)}</span>}
      </div>
    </figure>
  );
}

function Figures({ items }: { items: [string, string][] }) {
  return (
    // Four in a row only where the card has room for them; a value wraps rather than cut.
    <dl className={`grid grid-cols-2 gap-x-4 gap-y-2 ${items.length > 2 ? "@sm:grid-cols-4" : ""}`}>
      {items.map(([k, v]) => (
        <div key={k} className="min-w-0">
          <dt className="text-xs text-muted-foreground">{k}</dt>
          <dd className="font-heading text-lg leading-snug font-semibold break-words tabular-nums">{v}</dd>
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
  const distinct = profile?.distinct ?? 0;
  return profile?.sum === undefined && !profile?.first && (top.length === 0 || (distinct > 1 && top.every((x) => x.count === 1)) || mostlyDistinct(distinct, profile?.filled ?? 0));
};

export function FieldStats({ row, profile, total, bucket }: { row: FieldRow; profile?: FieldProfile; total: number; bucket: StatsBucket }) {
  const { t, locale } = useI18n();
  const num = useNumberFormat();
  const label = useValueText();
  const type = row.field.type;
  const n = profile?.filled ?? 0;
  const pct = total ? (n / total) * 100 : 0;
  const date = (v?: string) => (v ? new Date(`${v}T00:00:00`).toLocaleDateString(locale, { dateStyle: "medium" }) : "—");
  const top = profile?.top ?? [];
  const distinct = profile?.distinct ?? 0;
  const fig = (v: number) => fieldFigure(row.field, v, locale, num);
  return (
    <section aria-label={row.field.name} className="@container space-y-3 rounded-xl border bg-card p-4">
      {/* The type always sits under the name, so every card's header reads the same. */}
      <header>
        <h3 className="font-heading text-base font-semibold">{row.field.name}</h3>
        <p className="truncate text-xs text-muted-foreground">{t(`collections.type.${type}` as MessageKey)} · <code className="font-mono">{row.key}</code></p>
      </header>
      <div className="space-y-1">
        <div className="flex justify-between text-xs text-muted-foreground">
          <span>{t("analysis.filled")}</span>
          <span className="tabular-nums">{`${num(n)} / ${num(total)} · ${percent(n, total)}`}</span>
        </div>
        <div className="h-1.5 overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary/70" style={{ width: `${pct}%` }} /></div>
      </div>
      {profile?.sum !== undefined && (
        <Figures items={[[t("analysis.min"), fig(profile.min ?? 0)], [t("analysis.max"), fig(profile.max ?? 0)], [t("analysis.avg"), fig(profile.avg ?? 0)],
          // A sum of codes or revisions means nothing.
          ...(mostlyDistinct(distinct, n) ? [] : [[t("analysis.sum"), fig(profile.sum)] as [string, string]])]} />
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
      ) : mostlyDistinct(distinct, n) ? (
        <p className="text-xs text-muted-foreground">{t("analysis.mostly_distinct").replace("{n}", num(distinct))}</p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">{t("analysis.distinct").replace("{n}", num(distinct))}</p>
          <Bars items={top} total={total} label={label} color={(k) => optionCss(typeof k === "string" ? row.field.colors[k] : undefined)} />
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

/** The records list filtered to `where`: where a widget's figure or bar leads. */
export const recordsHref = (collection: { owner: string; slug: string }, where: Where) => {
  const text = whereText(where);
  return `${collectionPath(collection)}${text ? `?where=${encodeURIComponent(text)}` : ""}`;
};

/** The rule picking one value of a breakdown: empty values have their own. */
const valueRule = (field: string, key: unknown): Rule => (key === null || key === undefined || key === "" ? { field, op: "empty" } : { field, op: "eq", value: key });

/** The day `days` before today, as a date field stores it. */
const daysAgo = (days: number, now = new Date()) => {
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() - days);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
};
/** A widget's period and the one before it, as conditions on its date field. */
export function periods(c: { field: string; days: number }, now = new Date()): [Rule[], Rule[]] {
  const since = daysAgo(c.days - 1, now);
  const before = daysAgo(2 * c.days - 1, now);
  return [[{ field: c.field, op: "gte", value: since }], [{ field: c.field, op: "gte", value: before }, { field: c.field, op: "lt", value: since }]];
}
/** How much a figure moved against the period before, in whole percent; null when there is nothing to compare with. */
export const change = (now: number | undefined, before: number | undefined) => (now === undefined || before === undefined || before === 0 ? null : Math.round(((now - before) / Math.abs(before)) * 100));

/**
 * One dashboard widget, drawn from the records it selects in one request. `scope`
 * is the page's filter, joined to the widget's conditions; `head` sits beside the
 * title (the edit menu).
 */
export function WidgetView({ collection, widget, rows, scope, head }: { collection: Collection; widget: Widget; rows: FieldRow[]; scope?: Where; head?: React.ReactNode }) {
  const { t, locale } = useI18n();
  const num = useNumberFormat();
  const label = useValueText();
  const joined = scope ? bothWhere(asWhere(widget.where), scope) : asWhere(widget.where);
  const base = joined ?? asWhere(widget.where);
  const field = rows.find((r) => r.key === widget.field);
  const figure = widget.kind === "sum" || widget.kind === "avg" || widget.kind === "min" || widget.kind === "max";
  const numeric = figure || widget.kind === "count";
  const compare = numeric && widget.compare && rows.some((r) => r.key === widget.compare!.field) ? widget.compare : undefined;
  const [current, previous] = compare ? periods(compare) : [[], []];
  const where = compare ? andWhere(base, current) : base;
  const needsField = widget.kind !== "count" && widget.kind !== "list";
  const bucket = widget.bucket ?? "month";
  const measure = widget.kind === "bars" && widget.measure ? rows.find((r) => r.key === widget.measure) : undefined;
  const on = !!joined && widget.kind !== "list" && (!needsField || !!field);
  const st = useStats(collection.id, where, {
    numbers: figure && field ? [field] : [],
    values: widget.kind === "bars" && field && !measure ? [field] : [],
    dates: widget.kind === "timeline" && field ? [field] : [],
    bucket,
  }, on && !measure);
  const before = useStats(collection.id, andWhere(base, previous), { numbers: figure && field ? [field] : [] }, on && !!compare);
  const text = whereText(where) || undefined;
  // A sum or average per value: the grouped totals, as the list's groups read them.
  const grouped = useQuery({
    queryKey: ["records", collection.id, "widget-groups", widget.field, widget.measure, text],
    queryFn: ({ signal }) => records.aggregate({ collection_id: collection.id, where: text, group: field!.key, sum: measure!.key }, signal),
    enabled: on && !!measure && !!field,
  });
  const list = useQuery({
    queryKey: ["records", collection.id, "widget", widget.id, text],
    queryFn: ({ signal }) => records.list({ collection_id: collection.id, page: 1, limit: 5, sort: "updated_at", order: "desc", where: text }, signal),
    enabled: !!joined && widget.kind === "list",
  });
  const p = field ? st.data?.fields[field.key] : undefined;
  const pick = (d?: typeof st.data) => (widget.kind === "count" ? d?.total : figure && field ? d?.fields[field.key]?.[widget.kind as "sum" | "avg" | "min" | "max"] ?? undefined : undefined);
  const figureValue = pick(st.data);
  // A widget's own format wins; otherwise a sum reads as its field does (money, percent…).
  const own = widget.format !== undefined || widget.decimals !== undefined;
  const shown = (v: number, f?: FieldRow) => (own || !f ? formatFigure(v, locale, widget.format, widget.decimals) : fieldFigure(f.field, v, locale, num));
  const value = figureValue !== undefined ? shown(figureValue, widget.kind === "count" ? undefined : field) : "—";
  const moved = compare ? change(figureValue, pick(before.data)) : null;
  // Drill-down: the figure opens the records it counts; a bar, the records of its value.
  const all = recordsHref(collection, figure && field ? andWhere(where, [{ field: field.key, op: "not_empty" }]) : where);
  const byValue = field ? (key: unknown) => recordsHref(collection, andWhere(where, [valueRule(field.key, key)])) : undefined;
  const pending = measure ? grouped.isPending : st.isPending;
  const none = <p className="text-sm text-muted-foreground">{pending ? "…" : t("dashboard.no_records")}</p>;
  const total = st.data?.total ?? 0;
  const color = (k: unknown) => optionCss(typeof k === "string" ? field?.field.colors[k] : undefined);
  // Per value: the sum or average of the measure, largest first.
  const fn = widget.fn ?? "sum";
  // Values whose records hold no number for the measure have nothing to sum: they stay out.
  const measured = (grouped.data?.items ?? []).filter((g) => (measure?.key ?? "") in g.sums).map((g) => ({ key: g.key, count: (fn === "avg" ? g.avgs : g.sums)[measure!.key] })).sort((a, b) => b.count - a.count);
  // Shares of a sum add up; averages do not, so they show no share and no "others".
  const measuredTotal = fn === "sum" ? measured.reduce((n, g) => n + g.count, 0) : 0;
  const breakdown = measure ? measured : p?.top ?? [];
  const breakdownTotal = measure ? measuredTotal : total;
  const fmt = measure ? (n: number) => shown(n, measure) : undefined;
  return (
    <section aria-label={widget.title} className="flex h-full min-w-0 flex-col gap-2 rounded-xl border bg-card p-4">
      <div className="flex min-h-6 items-start justify-between gap-2">
        <h3 className="min-w-0 text-xs font-medium text-muted-foreground">
          {widget.title}
          {compare && <span className="font-normal"> · {t("dashboard.last_days").replace("{n}", String(compare.days))}</span>}
        </h3>
        {head}
      </div>
      {!joined ? <p className="text-sm text-muted-foreground">{t("dashboard.filter_conflict")}</p>
        : needsField && !field ? <p className="text-sm text-muted-foreground">{t("dashboard.field_gone")}</p>
        : widget.kind === "list" ? (
          list.data?.items.length ? (
            <ul className="divide-y text-sm">
              {list.data.items.map((r) => (
                <li key={r.id}><Link href={recordPath(collection, r.id)} className="block truncate py-1.5 hover:underline">{recordTitle(collection.schema ?? null, r.data) ?? r.id.slice(0, 8)}</Link></li>
              ))}
            </ul>
          ) : <p className="text-sm text-muted-foreground">{list.isPending ? "…" : t("dashboard.no_records")}</p>
        ) : widget.kind === "bars" ? (
          !breakdown.length ? none
            : widget.visual === "donut" ? <Donut items={breakdown} total={breakdownTotal} label={label} href={byValue} color={color} fmt={fmt} />
            : widget.visual === "table" ? <CountTable items={breakdown} total={breakdownTotal} label={label} href={byValue} head={field?.field.name ?? ""} fmt={fmt} measure={measure ? `${t(`dashboard.fn.${fn}` as MessageKey)}: ${measure.field.name}` : undefined} />
            : <Bars items={breakdown} total={breakdownTotal} label={label} href={byValue} color={color} fmt={fmt} />
        ) : widget.kind === "timeline" ? (
          p?.series?.length ? <Series points={p.series} bucket={bucket} line={widget.visual === "line"} /> : none
        ) : (
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <Link href={all} className="w-fit font-heading text-3xl font-semibold tabular-nums hover:underline" title={t("dashboard.open_records")}>{st.isPending ? "…" : value}</Link>
            {moved !== null && (
              <span className={cn("text-xs font-medium tabular-nums", moved > 0 ? "text-success" : moved < 0 ? "text-destructive" : "text-muted-foreground")}
                title={t("dashboard.vs_before").replace("{n}", String(compare!.days))}>
                {moved > 0 ? "▲" : moved < 0 ? "▼" : "="} {Math.abs(moved)}% <span className="font-normal text-muted-foreground">{t("dashboard.vs_before").replace("{n}", String(compare!.days))}</span>
              </span>
            )}
          </div>
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
