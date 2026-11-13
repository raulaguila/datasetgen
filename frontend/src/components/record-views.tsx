"use client";

import Link from "next/link";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, FileJson } from "lucide-react";
import { EmptyState } from "@/components/empty-state";
import { fileRefs, isImage, recordTitle, type FieldRow } from "@/components/field-builder";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { records, tools } from "@/lib/api/endpoints";
import type { Collection, DataRecord } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";
import { andWhere, whereText, type Where } from "@/components/record-rules";

export type Layout = "table" | "board" | "calendar" | "gallery";

/** The field each layout arranges records by. */
export const layoutFields: Record<Exclude<Layout, "table">, (r: FieldRow) => boolean> = {
  board: (r) => !r.advanced && (r.field.type === "enum" || r.field.type === "boolean"),
  calendar: (r) => !r.advanced && (r.field.type === "date" || r.field.type === "datetime"),
  gallery: (r) => !r.advanced && (r.field.type === "file" || r.field.type === "files"),
};

interface Common {
  collection: Collection;
  by: FieldRow;
  search?: string;
  where: Where;
  show: (v: unknown, r: FieldRow) => string;
  /** Up to three more fields shown on each card. */
  extra: FieldRow[];
}

// shortcut: the board, calendar month and gallery load at most 100 records; page them when a collection holds more per view.
const CARD_LIMIT = 100;

function useCards(p: { collection: Collection; search?: string; where: Where }, key: string) {
  const where = whereText(p.where) || undefined;
  return useQuery({
    queryKey: ["records", p.collection.id, "cards", key, p.search, where],
    queryFn: ({ signal }) => records.list({ collection_id: p.collection.id, page: 1, limit: CARD_LIMIT, search: p.search, where, sort: "updated_at", order: "desc" }, signal),
  });
}

function Card({ r, collection, extra, show, draggable, onDragStart }: { r: DataRecord; collection: Collection; extra: FieldRow[]; show: Common["show"]; draggable?: boolean; onDragStart?: (e: React.DragEvent) => void }) {
  const data = (r.data ?? {}) as Record<string, unknown>;
  const title = recordTitle(collection.schema ?? null, r.data) ?? r.id.slice(0, 8);
  return (
    <Link href={`/records/${r.id}`} draggable={draggable} onDragStart={onDragStart}
      className="block space-y-1 rounded-md border bg-card p-2.5 text-sm shadow-xs hover:border-primary/50 focus-visible:outline-2">
      <p className="truncate font-medium">{title}</p>
      {extra.filter((f) => data[f.key] !== undefined && data[f.key] !== "").map((f) => (
        <p key={f.key} className="truncate text-xs text-muted-foreground"><span>{f.field.name}:</span> {show(data[f.key], f)}</p>
      ))}
    </Link>
  );
}

/**
 * Kanban: a column per option of a choice (or yes/no) field. Dragging a card to
 * another column saves the new value.
 */
export function BoardView({ save, canEdit, ...p }: Common & { canEdit: boolean; save: (id: string, key: string, v: unknown) => Promise<boolean> }) {
  const { t } = useI18n();
  const q = useCards(p, `board:${p.by.key}`);
  const [over, setOver] = useState<string | null>(null);
  const type = p.by.field.type;
  const lanes: [string, unknown, string][] = type === "boolean"
    ? [["true", true, t("common.yes")], ["false", false, t("common.no")]]
    : p.by.field.options.split(",").map((o) => o.trim()).filter(Boolean).map((o) => [o, o, o]);
  if (!p.by.field.required) lanes.push(["\u0000", null, t("groups.empty")]);
  if (q.isPending) return <ListSkeleton rows={3} />;
  const items = q.data?.items ?? [];
  const laneOf = (r: DataRecord) => {
    const v = ((r.data ?? {}) as Record<string, unknown>)[p.by.key];
    return v === undefined || v === null || v === "" ? "\u0000" : String(v);
  };
  return (
    <div className="flex gap-3 overflow-x-auto pb-2" role="list" aria-label={t("layout.board")}>
      {lanes.map(([key, value, label]) => {
        const cards = items.filter((r) => laneOf(r) === key);
        return (
          <section key={key} role="listitem" aria-label={`${label} (${cards.length})`}
            className={cn("flex w-72 shrink-0 flex-col gap-2 rounded-lg border bg-muted/30 p-2", over === key && "border-primary bg-primary/5")}
            onDragOver={(e) => { if (canEdit) { e.preventDefault(); setOver(key); } }}
            onDragLeave={() => setOver((o) => (o === key ? null : o))}
            onDrop={(e) => { e.preventDefault(); setOver(null); const id = e.dataTransfer.getData("text/record"); if (id) void save(id, p.by.key, value); }}>
            <h3 className="flex items-center justify-between px-1 text-sm font-medium"><span className="truncate">{label}</span><span className="text-xs text-muted-foreground">{cards.length}</span></h3>
            {cards.map((r) => (
              <Card key={r.id} r={r} collection={p.collection} extra={p.extra} show={p.show} draggable={canEdit}
                onDragStart={(e) => { e.dataTransfer.setData("text/record", r.id); e.dataTransfer.effectAllowed = "move"; }} />
            ))}
          </section>
        );
      })}
      {items.length >= CARD_LIMIT && <p className="self-end text-xs text-muted-foreground">{t("layout.first_n").replace("{n}", String(CARD_LIMIT))}</p>}
    </div>
  );
}

const pad = (n: number) => String(n).padStart(2, "0");
const dayKey = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

/** A month grid placing each record on its date field's day. */
export function CalendarView(p: Common) {
  const { t, locale } = useI18n();
  const [month, setMonth] = useState(() => { const d = new Date(); return new Date(d.getFullYear(), d.getMonth(), 1); });
  const next = new Date(month.getFullYear(), month.getMonth() + 1, 1);
  const datetime = p.by.field.type === "datetime";
  const bound = (d: Date) => (datetime ? d.toISOString() : dayKey(d));
  // The month's range joins the list's rules with AND, OR rules and groups included.
  const monthRules = [{ field: p.by.key, op: "gte" as const, value: bound(month) }, { field: p.by.key, op: "lt" as const, value: bound(next) }];
  const where = andWhere(p.where, monthRules);
  const q = useCards({ ...p, where }, `cal:${p.by.key}:${dayKey(month)}`);
  const byDay = new Map<string, DataRecord[]>();
  for (const r of q.data?.items ?? []) {
    const v = ((r.data ?? {}) as Record<string, unknown>)[p.by.key];
    if (typeof v !== "string") continue;
    const key = datetime ? dayKey(new Date(v)) : v.slice(0, 10);
    byDay.set(key, [...(byDay.get(key) ?? []), r]);
  }
  const start = new Date(month);
  start.setDate(1 - month.getDay()); // the Sunday on or before the 1st
  const days = Array.from({ length: 42 }, (_, i) => new Date(start.getFullYear(), start.getMonth(), start.getDate() + i));
  const weekday = new Intl.DateTimeFormat(locale, { weekday: "short" });
  const today = dayKey(new Date());
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <Button variant="outline" size="icon-sm" aria-label={t("layout.prev_month")} onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))}><ChevronLeft className="size-4" /></Button>
        <h2 className="min-w-40 text-center font-medium capitalize">{new Intl.DateTimeFormat(locale, { month: "long", year: "numeric" }).format(month)}</h2>
        <Button variant="outline" size="icon-sm" aria-label={t("layout.next_month")} onClick={() => setMonth(next)}><ChevronRight className="size-4" /></Button>
        <Button variant="ghost" size="sm" onClick={() => { const d = new Date(); setMonth(new Date(d.getFullYear(), d.getMonth(), 1)); }}>{t("layout.today")}</Button>
      </div>
      <div className="grid grid-cols-7 overflow-hidden rounded-lg border text-sm" role="grid" aria-busy={q.isPending}>
        {days.slice(0, 7).map((d) => <div key={d.getDay()} role="columnheader" className="border-b bg-muted/40 px-2 py-1 text-xs font-medium capitalize text-muted-foreground">{weekday.format(d)}</div>)}
        {days.map((d) => {
          const key = dayKey(d);
          const list = byDay.get(key) ?? [];
          return (
            <div key={key} role="gridcell" aria-label={`${d.toLocaleDateString(locale)}: ${list.length}`}
              className={cn("min-h-24 space-y-1 border-b border-r p-1", d.getMonth() !== month.getMonth() && "bg-muted/20 text-muted-foreground")}>
              <p className={cn("text-xs", key === today && "font-semibold text-primary")}>{d.getDate()}</p>
              {list.slice(0, 3).map((r) => (
                <Link key={r.id} href={`/records/${r.id}`} className="block truncate rounded bg-primary/10 px-1 text-xs hover:bg-primary/20">
                  {recordTitle(p.collection.schema ?? null, r.data) ?? r.id.slice(0, 8)}
                </Link>
              ))}
              {list.length > 3 && <p className="px-1 text-xs text-muted-foreground">+{list.length - 3}</p>}
            </div>
          );
        })}
      </div>
    </div>
  );
}


/** Cards with the picture of a file field, for collections of images or documents. */
export function GalleryView(p: Common) {
  const { t } = useI18n();
  const q = useCards(p, `gallery:${p.by.key}`);
  if (q.isPending) return <ListSkeleton rows={3} />;
  const items = q.data?.items ?? [];
  if (!items.length) return <EmptyState icon={FileJson} title={t("common.no_results")} />;
  return (
    <ul className="grid grid-cols-[repeat(auto-fill,minmax(12rem,1fr))] gap-3" aria-label={t("layout.gallery")}>
      {items.map((r) => {
        const files = fileRefs(((r.data ?? {}) as Record<string, unknown>)[p.by.key]);
        const file = files.find(isImage) ?? files[0];
        return (
          <li key={r.id} className="overflow-hidden rounded-lg border bg-card">
            <div className="grid aspect-[4/3] place-items-center bg-muted/40">
              {file && isImage(file)
                // eslint-disable-next-line @next/next/no-img-element -- a private file behind the session proxy, not an optimizable asset
                ? <img src={tools.fileUrl(file.id)} alt={file.name} loading="lazy" className="size-full object-cover" />
                : <span className="px-2 text-center text-xs text-muted-foreground">{file?.name ?? t("layout.no_file")}</span>}
            </div>
            <div className="p-1"><Card r={r} collection={p.collection} extra={p.extra} show={p.show} /></div>
          </li>
        );
      })}
    </ul>
  );
}
