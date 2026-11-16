"use client";

import { useId, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, BarChart3, Copy, GripVertical, Hash, List, Maximize2, Minimize2, MoreHorizontal, MoveDown, MoveUp, Pencil, Plus, Sigma, SquareDivide, Trash2, TrendingUp, type LucideIcon } from "lucide-react";
import { toast } from "sonner";
import { ViewConditions, WIDGET_KINDS, WidgetView, fieldsFor, isDate, isNumeric } from "@/components/collection-stats";
import { BUCKETS } from "@/components/collection-stats";
import { fieldsOf } from "@/components/field-builder";
import { SidePanel } from "@/components/fields-editor";
import { RecordRules, type Where } from "@/components/record-rules";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { tools } from "@/lib/api/endpoints";
import type { Collection, StatsBucket, Widget } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { cn, newId } from "@/lib/utils";

const MAX = 24;
const AUTO = "auto";
const FORMATS: NonNullable<Widget["format"]>[] = ["number", "compact", "currency", "percent"];
/** The drawings a kind offers, its default first. */
const VISUALS: Partial<Record<Widget["kind"], NonNullable<Widget["visual"]>[]>> = { bars: ["bars", "donut", "table"], timeline: ["columns", "line"] };
const KIND_ICONS: Record<Widget["kind"], LucideIcon> = { count: Hash, sum: Sigma, avg: SquareDivide, min: MoveDown, max: MoveUp, bars: BarChart3, timeline: TrendingUp, list: List };
const PERIODS = [7, 30, 90, 365];
const NONE = "none";

/** A field picker of the widget form. */
function Pick({ id, label, value, items, placeholder, onChange }: { id: string; label: string; value: string; items: Record<string, string>; placeholder?: string; onChange: (v: string) => void }) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Select value={value || null} items={items} onValueChange={(v) => onChange(String(v ?? ""))}>
        <SelectTrigger id={id} className="w-full"><SelectValue placeholder={placeholder} /></SelectTrigger>
        <SelectContent>{Object.entries(items).map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}</SelectContent>
      </Select>
    </div>
  );
}

/** The kinds of widget, as cards with an icon and what each shows. */
function KindGallery({ onPick }: { onPick: (k: Widget["kind"]) => void }) {
  const { t } = useI18n();
  return (
    <div className="grid gap-1.5 sm:grid-cols-2">
      {WIDGET_KINDS.map((k) => {
        const Icon = KIND_ICONS[k];
        return (
          <button key={k} type="button" aria-label={t(`dashboard.kind.${k}` as MessageKey)} onClick={() => onPick(k)}
            className="flex items-start gap-2.5 rounded-md border px-3 py-2 text-left outline-none transition-colors hover:border-primary/40 hover:bg-primary/5 focus-visible:ring-2 focus-visible:ring-ring">
            <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
            <span className="min-w-0">
              <span className="block text-sm font-medium">{t(`dashboard.kind.${k}` as MessageKey)}</span>
              <span className="block text-xs text-muted-foreground">{t(`dashboard.kind_hint.${k}` as MessageKey)}</span>
            </span>
          </button>
        );
      })}
    </div>
  );
}

/** The form of one widget, with the widget itself drawn live under it. */
function WidgetForm({ formId, collection, initial, onSave }: { formId: string; collection: Collection; initial: Widget; onSave: (w: Widget) => void }) {
  const { t } = useI18n();
  const id = useId();
  const rows = fieldsOf(collection.schema ?? null);
  const [kind, setKind] = useState<Widget["kind"]>(initial.kind);
  const [title, setTitle] = useState(initial.title);
  const [field, setField] = useState(initial.field ?? "");
  const [where, setWhere] = useState<Where>((initial.where as Where | undefined) ?? { any: false, rules: [] });
  const [bucket, setBucket] = useState<StatsBucket>(initial.bucket ?? "month");
  const [format, setFormat] = useState<string>(initial.format ?? AUTO);
  const [decimals, setDecimals] = useState<string>(initial.decimals === undefined ? AUTO : String(initial.decimals));
  const [visual, setVisual] = useState<Widget["visual"]>(initial.visual);
  const [measure, setMeasure] = useState(initial.measure ? `${initial.fn ?? "sum"}:${initial.measure}` : NONE);
  const [compareField, setCompareField] = useState(initial.compare?.field ?? "");
  const [days, setDays] = useState(initial.compare ? String(initial.compare.days) : NONE);
  const fit = fieldsFor(kind, rows);
  const picked = fit?.find((r) => r.key === field);
  const kinds = Object.fromEntries(WIDGET_KINDS.map((k) => [k, t(`dashboard.kind.${k}` as MessageKey)]));
  const numeric = kind === "count" || kind === "sum" || kind === "avg" || kind === "min" || kind === "max";
  const visuals = VISUALS[kind] ?? [];
  const numbers = rows.filter((r) => !r.advanced && isNumeric(r));
  const dates = rows.filter((r) => !r.advanced && isDate(r));
  const [fn, measureKey] = measure === NONE ? ["", ""] : measure.split(":");
  const measureRow = numbers.find((r) => r.key === measureKey);
  // A blank title names the widget after what it shows.
  const auto = kind === "bars" && measureRow && picked ? `${t(`dashboard.fn.${fn}` as MessageKey)}: ${measureRow.field.name} × ${picked.field.name}` : `${kinds[kind]}${picked ? `: ${picked.field.name}` : ""}`;
  const ready = (!fit || !!picked) && (days === NONE || !!compareField);
  const build = (): Widget => ({
    id: initial.id, kind, title: title.trim() || auto, ...(fit ? { field } : {}), ...(where.rules.length || where.groups?.length ? { where } : {}), ...(kind === "timeline" ? { bucket } : {}),
    ...((numeric || measureRow) && format !== AUTO ? { format: format as NonNullable<Widget["format"]> } : {}), ...((numeric || measureRow) && decimals !== AUTO ? { decimals: Number(decimals) } : {}),
    ...(visual && visuals.includes(visual) && visual !== visuals[0] ? { visual } : {}),
    ...(kind === "bars" && measureRow ? { measure: measureRow.key, fn: fn as "sum" | "avg" } : {}),
    ...(numeric && days !== NONE && compareField ? { compare: { field: compareField, days: Number(days) } } : {}),
    size: initial.size ?? ((kind === "bars" && visual !== "donut") || kind === "list" || kind === "timeline" ? 2 : 1),
  });
  const formats = { [AUTO]: t("dashboard.format_auto"), ...Object.fromEntries(FORMATS.map((f) => [f, t(`dashboard.format.${f}` as MessageKey)])) };
  const decimalItems = { [AUTO]: t("dashboard.decimals_auto"), ...Object.fromEntries(["0", "1", "2", "3"].map((d) => [d, d])) };
  const measures = { [NONE]: t("dashboard.measure.count"), ...Object.fromEntries(numbers.flatMap((r) => (["sum", "avg"] as const).map((f) => [`${f}:${r.key}`, `${t(`dashboard.fn.${f}` as MessageKey)}: ${r.field.name}`]))) };
  return (
    <form id={formId} className="space-y-5" onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); if (ready) onSave(build()); }}>
      <div className="grid gap-4 @sm:grid-cols-2">
        <Pick id={`${id}-kind`} label={t("dashboard.kind")} value={kind} items={kinds} onChange={(v) => { setKind(v as Widget["kind"]); setField(""); setVisual(undefined); setMeasure(NONE); }} />
        {fit && <Pick id={`${id}-field`} label={t(kind === "bars" ? "dashboard.group_by" : "dashboard.field")} value={field} items={Object.fromEntries(fit.map((r) => [r.key, r.field.name]))} placeholder={t(fit.length ? "fix.pick" : "dashboard.no_fields")} onChange={setField} />}
        {kind === "bars" && <Pick id={`${id}-measure`} label={t("dashboard.measure")} value={measure} items={measures} onChange={setMeasure} />}
        {kind === "timeline" && <Pick id={`${id}-bucket`} label={t("analysis.bucket")} value={bucket} items={Object.fromEntries(BUCKETS.map((b) => [b, t(`analysis.bucket.${b}` as MessageKey)]))} onChange={(v) => setBucket(v as StatsBucket)} />}
        {visuals.length > 0 && <Pick id={`${id}-visual`} label={t("dashboard.visual")} value={visual && visuals.includes(visual) ? visual : visuals[0]} items={Object.fromEntries(visuals.map((v) => [v, t(`dashboard.visual.${v}` as MessageKey)]))} onChange={(v) => setVisual(v as Widget["visual"])} />}
        {(numeric || measureRow) && (
          <>
            <Pick id={`${id}-format`} label={t("dashboard.format")} value={format} items={formats} onChange={setFormat} />
            <Pick id={`${id}-decimals`} label={t("dashboard.decimals")} value={decimals} items={decimalItems} onChange={setDecimals} />
          </>
        )}
        {numeric && dates.length > 0 && (
          <>
            <Pick id={`${id}-compare`} label={t("dashboard.compare")} value={days} items={{ [NONE]: t("dashboard.compare_none"), ...Object.fromEntries(PERIODS.map((d) => [String(d), t("dashboard.last_days").replace("{n}", String(d))])) }}
              onChange={(v) => { setDays(v); if (v !== NONE && !compareField) setCompareField(dates[0].key); }} />
            {days !== NONE && <Pick id={`${id}-compare-field`} label={t("dashboard.compare_field")} value={compareField} items={Object.fromEntries(dates.map((r) => [r.key, r.field.name]))} onChange={setCompareField} />}
          </>
        )}
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor={`${id}-title`}>{t("dashboard.title_label")}</Label>
        <Input id={`${id}-title`} maxLength={80} value={title} placeholder={auto} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <div className="space-y-1.5">
        <p className="text-sm font-medium">{t("dashboard.records")}</p>
        <div className="flex flex-wrap items-center gap-2">
          <RecordRules fields={rows} value={where} onChange={setWhere} />
          <ViewConditions collectionId={collection.id} onPick={setWhere} />
          {!where.rules.length && !where.groups?.length && <span className="text-xs text-muted-foreground">{t("dashboard.all_records")}</span>}
        </div>
      </div>
      <section className="space-y-2 border-t pt-4" aria-label={t("dashboard.preview")}>
        <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t("dashboard.preview")}</h3>
        {ready ? <WidgetView collection={collection} widget={build()} rows={rows} /> : <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">{t("dashboard.preview_needs")}</p>}
      </section>
    </form>
  );
}

/**
 * The collection's own dashboard: counts, sums, bars and lists the owners pick,
 * each over the records its conditions select. Every member sees it.
 */
const SM_SPAN = ["", "sm:col-span-1", "sm:col-span-2"];
const XL_SPAN = ["", "xl:col-span-1", "xl:col-span-2", "xl:col-span-3", "xl:col-span-4"];

/** Widths in a grid of `cols`: a widget before a row break, and the last one, take what their row leaves, so no cell stays empty. */
export function packSpans(sizes: number[], cols: number): number[] {
  const out = sizes.map((n) => Math.min(n, cols));
  let row = 0;
  out.forEach((n, i) => {
    if (row + n > cols) { out[i - 1] += cols - row; row = 0; }
    row = (row + n) % cols;
  });
  if (row > 0) out[out.length - 1] += cols - row;
  return out;
}

function spanOf(widgets: Widget[]) {
  const sizes = widgets.map((w) => (w.size === 2 ? 2 : 1));
  const sm = packSpans(sizes, 2);
  const xl = packSpans(sizes, 4);
  return (_: Widget, i: number) => cn(SM_SPAN[sm[i]], XL_SPAN[xl[i]]);
}

export function CollectionDashboard({ collection, scope }: { collection: Collection; /** The page's filter, joined to every widget's conditions. */ scope?: Where }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const formId = useId();
  const owner = roleAllows(collection.role, "owner");
  const key = ["collections", "dashboard", collection.id];
  const q = useQuery({ queryKey: key, queryFn: ({ signal }) => tools.dashboard(collection.id, signal) });
  const [draft, setDraft] = useState<Widget[] | null>(null);
  // A new widget starts in the gallery (kind null), then its form.
  const [editing, setEditing] = useState<{ widget: Widget | null; kind: Widget["kind"] | null } | null>(null);
  const [dragging, setDragging] = useState<string | null>(null);
  const save = useMutation({
    mutationFn: (widgets: Widget[]) => tools.saveDashboard(collection.id, widgets),
    onSuccess: (_, widgets) => { client.setQueryData(key, { widgets }); setDraft(null); toast.success(t("common.saved")); },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const rows = fieldsOf(collection.schema ?? null);
  const widgets = draft ?? q.data?.widgets ?? [];
  const span = spanOf(widgets);
  if (q.isPending || q.isError) return null;
  if (!widgets.length && !draft && !owner) return null;

  const move = (i: number, to: number) => setDraft((w) => { const next = [...(w ?? [])]; const [it] = next.splice(i, 1); next.splice(to, 0, it); return next; });
  const put = (w: Widget) => {
    setDraft((list) => { const cur = list ?? widgets; return cur.some((x) => x.id === w.id) ? cur.map((x) => (x.id === w.id ? w : x)) : [...cur, w]; });
    setEditing(null);
  };
  const duplicate = (w: Widget) => setDraft((list) => { const cur = list ?? []; const at = cur.findIndex((x) => x.id === w.id); return [...cur.slice(0, at + 1), { ...w, id: newId() }, ...cur.slice(at + 1)]; });
  const kind = editing?.widget?.kind ?? editing?.kind;
  const menu = (w: Widget, i: number) => (
    <DropdownMenu>
      <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="icon-sm" className="-mr-2 -mt-1.5 shrink-0" aria-label={`${t("common.actions")}: ${w.title}`} />}><MoreHorizontal className="size-4" /></DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={() => setEditing({ widget: w, kind: w.kind })}><Pencil className="size-4" />{t("dashboard.edit_widget")}</DropdownMenuItem>
        <DropdownMenuItem onClick={() => put({ ...w, size: w.size === 2 ? 1 : 2 })}>{w.size === 2 ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}{t(w.size === 2 ? "dashboard.narrow" : "dashboard.wide")}</DropdownMenuItem>
        <DropdownMenuItem disabled={i === 0} onClick={() => move(i, i - 1)}><ArrowUp className="size-4" />{t("dashboard.move_up")}</DropdownMenuItem>
        <DropdownMenuItem disabled={i === widgets.length - 1} onClick={() => move(i, i + 1)}><ArrowDown className="size-4" />{t("dashboard.move_down")}</DropdownMenuItem>
        <DropdownMenuItem disabled={widgets.length >= MAX} onClick={() => duplicate(w)}><Copy className="size-4" />{t("dashboard.duplicate")}</DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem variant="destructive" onClick={() => setDraft((list) => (list ?? []).filter((x) => x.id !== w.id))}><Trash2 className="size-4" />{t("dashboard.remove")}</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
  return (
    <section aria-label={t("dashboard.title")} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-heading text-base font-semibold">{t("dashboard.title")}</h2>
        {owner && (draft ? (
          <div className="flex flex-wrap gap-2">
            <Button variant="ghost" size="sm" onClick={() => setDraft(null)}>{t("common.cancel")}</Button>
            <Button variant="outline" size="sm" disabled={widgets.length >= MAX} onClick={() => setEditing({ widget: null, kind: null })}><Plus className="size-4" />{t("dashboard.add_widget")}</Button>
            <Button size="sm" disabled={save.isPending} onClick={() => save.mutate(widgets)}>{t("common.save")}</Button>
          </div>
        ) : (
          <Button variant="outline" size="sm" onClick={() => setDraft(widgets)}><Pencil className="size-4" />{t("dashboard.edit")}</Button>
        ))}
      </div>
      {draft && widgets.length > 1 && <p className="text-xs text-muted-foreground">{t("dashboard.drag_hint")}</p>}
      {widgets.length === 0 ? (
        <button type="button" className="w-full rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground hover:bg-muted/40" onClick={() => { setDraft([]); setEditing({ widget: null, kind: null }); }}>
          <Plus className="mx-auto mb-1 size-5" />{t("dashboard.empty")}
        </button>
      ) : (
        // Rows as tall as their tallest widget, no more: a lone figure stays compact.
        <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {widgets.map((w, i) => (
            <li key={w.id} className={cn("relative", span(w, i), dragging === w.id && "opacity-50")}
              draggable={!!draft}
              onDragStart={(e) => { setDragging(w.id); e.dataTransfer.effectAllowed = "move"; }}
              onDragOver={(e) => { if (dragging) e.preventDefault(); }}
              onDrop={(e) => { e.preventDefault(); const from = widgets.findIndex((x) => x.id === dragging); if (from >= 0 && from !== i) move(from, i); setDragging(null); }}
              onDragEnd={() => setDragging(null)}>
              <WidgetView collection={collection} widget={w} rows={rows} scope={draft ? undefined : scope}
                head={draft && <span className="flex items-center gap-0.5"><GripVertical className="size-4 cursor-grab text-muted-foreground" aria-label={t("collections.field_drag")} />{menu(w, i)}</span>} />
            </li>
          ))}
        </ul>
      )}
      <SidePanel
        open={editing !== null}
        onOpenChange={(o) => !o && setEditing(null)}
        title={t(editing?.widget ? "dashboard.edit_widget" : "dashboard.add_widget")}
        description={editing && !kind ? t("dashboard.gallery_hint") : undefined}
        keepOnOutsideClick
        footer={kind && (
          <div className="flex flex-wrap items-center justify-end gap-2">
            {!editing?.widget && <Button type="button" variant="ghost" className="mr-auto" onClick={() => setEditing({ widget: null, kind: null })}>{t("dashboard.change_kind")}</Button>}
            <Button type="button" variant="outline" onClick={() => setEditing(null)}>{t("common.cancel")}</Button>
            <Button type="submit" form={formId}>{t(editing?.widget ? "common.save" : "dashboard.add_widget")}</Button>
          </div>
        )}
      >
        {editing && !kind && <KindGallery onPick={(k) => setEditing({ widget: null, kind: k })} />}
        {editing && kind && (
          <WidgetForm key={editing.widget?.id ?? `new-${kind}`} formId={formId} collection={collection}
            initial={editing.widget ?? { id: newId(), kind, title: "" }} onSave={put} />
        )}
      </SidePanel>
    </section>
  );
}
