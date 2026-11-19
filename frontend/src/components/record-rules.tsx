"use client";

import type { CollectionRef } from "@/lib/paths";
import { useEffect, useId, useRef, useState, useSyncExternalStore } from "react";
import { createPortal } from "react-dom";
import { ContextMenu } from "@base-ui/react/context-menu";
import { Popover } from "@base-ui/react/popover";
import { keepPreviousData, useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { Bookmark, Check, ChevronDown, Code, Copy, Filter, FilterX, Group, Loader2, MoreHorizontal, Plus, Search, SlidersHorizontal, Trash2, X } from "lucide-react";
import { toast } from "sonner";
import { IconAction } from "@/components/bits";
import { recordTitle, type FieldRow } from "@/components/field-builder";
import { optionColor } from "@/components/option-chip";
import { LIST_OPS, entriesOf, fromDraft, kindOf, needsValue, opsFor, toDraft, whereText, type DraftRule, type EditOp, type Entry, type Kind, type Rule, type Where } from "@/components/rule-model";
import { parseRuleText, refNameOf, ruleText, suggestAt, type Lang, type RefName, type TextError } from "@/components/rule-text";
import { RecordPicker, RefChip, useRecordLabel } from "@/components/schema-form";
import { TYPE_ICONS } from "@/components/type-icons";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectSeparator, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import type { MessageKey } from "@/lib/i18n/messages";
import { collections, records, tools } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { cn } from "@/lib/utils";

export * from "@/components/rule-model";

type Translate = ReturnType<typeof useI18n>["t"];

const MAX_RULES = 20;
const MAX_GROUPS = 10;

/** "Today", "3 days ago", or the date in the reader's format. */
function dateText(raw: string, t: Translate): string {
  if (/^@-?\d+$/.test(raw)) {
    const n = Number(raw.slice(1));
    if (n === 0) return t("rules.today");
    if (n === -1) return t("rules.yesterday");
    if (n === 1) return t("rules.tomorrow");
    return (n < 0 ? t("rules.days_ago") : t("rules.days_ahead")).replace("{n}", String(Math.abs(n)));
  }
  const d = new Date(`${raw}T00:00:00`);
  return Number.isNaN(d.getTime()) ? raw : d.toLocaleDateString();
}

/** A record's name, for chips: the id until it loads. */
function RefName({ target, id }: { target: string; id: string }) {
  return <>{useRecordLabel(target, id) ?? id.slice(0, 8)}</>;
}

function opLabel(op: EditOp, kind: Kind, t: Translate): string {
  return kind === "date" && ["eq", "neq", "lt", "gt", "lte", "gte"].includes(op) ? t(`rules.op_date.${op}` as MessageKey) : t(`rules.op.${op}` as MessageKey);
}

const SYMBOL: Partial<Record<EditOp, string>> = { eq: "=", neq: "≠", gt: ">", gte: "≥", lt: "<", lte: "≤" };
/** The operator as a chip shows it: a symbol where one reads at a glance (dates and lists keep words). */
const opShort = (op: EditOp, kind: Kind, t: Translate) => (!["date", "strings", "refs"].includes(kind) && SYMBOL[op]) || opLabel(op, kind, t);

/** A rule's value as a chip shows it, with record names; null when the operator takes none. */
function RuleValue({ entry, rule, t }: { entry: Entry | undefined; rule: Rule; t: Translate }) {
  const kind = entry?.kind ?? "text";
  const d = toDraft(rule, kind);
  if (!needsValue(d.op)) return null;
  if (d.op === "last_days" || d.op === "next_days") return <>{d.raw} {t("rules.days")}</>;
  const one = (raw: string) => (kind === "date" ? dateText(raw, t) : kind === "boolean" ? t(raw === "true" ? "common.yes" : "common.no") : kind === "ref" || kind === "refs" ? <RefName target={entry?.target ?? ""} id={raw} /> : raw);
  if (d.op === "between") return <>{one(d.raw)} {t("rules.and")} {one(d.raw2)}</>;
  if (d.list.length) return <>{d.list.map((v, i) => <span key={v}>{i > 0 && ", "}{one(v)}</span>)}</>;
  return <>{one(d.raw)}</>;
}

/** Plain text of a rule, for the chip's buttons. */
function ruleWords(entry: Entry | undefined, rule: Rule, t: Translate): string {
  const kind = entry?.kind ?? "text";
  const d = toDraft(rule, kind);
  const one = (raw: string) => (kind === "date" ? dateText(raw, t) : kind === "boolean" ? t(raw === "true" ? "common.yes" : "common.no") : raw);
  const value = !needsValue(d.op) ? "" : d.op === "last_days" || d.op === "next_days" ? `${d.raw} ${t("rules.days")}` : d.list.length ? d.list.map(one).join(", ") : d.op === "between" ? `${one(d.raw)} – ${one(d.raw2)}` : one(d.raw);
  return `${entry?.name ?? rule.field} ${opShort(d.op, kind, t)} ${value}`.trim();
}

/** A date: a fixed day, or today give or take days, so a saved view keeps up. */
function DateValue({ raw, label, onChange }: { raw: string; label: string; onChange: (raw: string) => void }) {
  const { t } = useI18n();
  const relative = /^@-?\d+$/.test(raw);
  const modes: Record<string, string> = { date: t("rules.date_fixed"), "@0": t("rules.today"), "@-1": t("rules.yesterday"), "@1": t("rules.tomorrow") };
  if (relative && !modes[raw]) modes[raw] = dateText(raw, t);
  return (
    <div className="flex min-w-0 gap-1.5">
      <Select value={relative ? raw : "date"} items={modes} onValueChange={(v) => onChange(v === "date" ? "" : String(v))}>
        <SelectTrigger className={relative ? "w-full" : "w-28 shrink-0"} aria-label={`${t("rules.date_mode")} ${label}`}><SelectValue /></SelectTrigger>
        <SelectContent>{Object.entries(modes).map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}</SelectContent>
      </Select>
      {!relative && <Input type="date" className="min-w-0" aria-label={`${t("rules.value")} ${label}`} value={raw} onChange={(e) => onChange(e.target.value)} />}
    </div>
  );
}

/** Text with the field's commonest values offered as you type. */
function TextValue({ collection, entry, raw, label, onChange }: { collection?: CollectionRef; entry: Entry; raw: string; label: string; onChange: (raw: string) => void }) {
  const { t } = useI18n();
  const listId = useId();
  const ask = !!collection && entry.stored && entry.key[0] !== "$";
  const top = useQuery({
    queryKey: ["records", collection?.id, "values", entry.key],
    queryFn: ({ signal }) => records.stats(collection!, { values: entry.key }, signal),
    enabled: ask, staleTime: 60_000,
  });
  const values = (top.data?.fields?.[entry.key]?.top ?? []).map((x) => x.key).filter((k): k is string => typeof k === "string" && k !== "");
  return (
    <>
      <Input aria-label={`${t("rules.value")} ${label}`} value={raw} placeholder={entry.kind === "strings" ? t("rules.in_hint") : t("rules.value")} list={values.length ? listId : undefined} onChange={(e) => onChange(e.target.value)} />
      {values.length > 0 && <datalist id={listId}>{values.map((v) => <option key={v} value={v} />)}</datalist>}
    </>
  );
}

/** Records picked by searching the reference's collection. */
function RecordsValue({ entry, ids, many, label, onChange }: { entry: Entry; ids: string[]; many: boolean; label: string; onChange: (ids: string[]) => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const id = useId();
  return (
    <div className="min-w-0 space-y-1.5">
      {ids.length > 0 && <ul className="flex flex-wrap gap-1.5" aria-label={label}>{ids.map((v) => <RefChip key={v} target={entry.target} value={v} onRemove={() => onChange(ids.filter((x) => x !== v))} />)}</ul>}
      {open ? (
        <RecordPicker id={id} target={entry.target} onPick={(r) => { onChange(many ? (ids.includes(r) ? ids : [...ids, r]) : [r]); if (!many) setOpen(false); }} onClose={() => setOpen(false)} />
      ) : (many || ids.length === 0) && (
        <Button type="button" variant="outline" size="sm" className="w-full justify-start" aria-label={`${t("rules.value")} ${label}`} onClick={() => setOpen(true)}><Search className="size-4" />{t(many && ids.length ? "rules.pick_more" : "rules.pick_record")}</Button>
      )}
    </div>
  );
}

/** Options picked by toggling them: "is one of" and "is none of". */
function OptionsValue({ options, colors, picked, label, onChange }: { options: string[]; colors: Record<string, string>; picked: string[]; label: string; onChange: (list: string[]) => void }) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-1">
      {options.map((o) => {
        const on = picked.includes(o);
        return (
          <Button key={o} type="button" size="sm" variant="outline" aria-pressed={on}
            className={cn("h-7 gap-1.5 px-2 text-xs", on && "border-primary bg-primary/10 text-primary hover:bg-primary/15")}
            onClick={() => onChange(on ? picked.filter((x) => x !== o) : [...picked, o])}>
            {on ? <Check className="size-3.5" aria-hidden /> : <span className={cn("size-2 rounded-full", optionColor(colors[o]).dot)} aria-hidden />}{o}
          </Button>
        );
      })}
    </div>
  );
}

const low = (s: string) => s.toLocaleLowerCase();

/** A field picked from a searchable list, each with its type's icon. */
function FieldPicker({ entries, value, label, onPick }: { entries: Entry[]; value: string; label: string; onPick: (key: string) => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [at, setAt] = useState(0);
  const listId = useId();
  const shown = entries.filter((e) => low(e.name).includes(low(q.trim())));
  const current = entries.find((e) => e.key === value);
  const Icon = current ? TYPE_ICONS[current.type] : null;
  const pick = (key: string) => { onPick(key); setOpen(false); };
  return (
    <Popover.Root open={open} onOpenChange={(o) => { setOpen(o); setQ(""); setAt(0); }}>
      <Popover.Trigger render={
        <button type="button" role="combobox" aria-expanded={open} aria-controls={listId} aria-haspopup="listbox" aria-label={label}
          className="flex h-[var(--control-h)] w-full min-w-0 items-center gap-1.5 rounded-lg border border-input bg-card py-1 pr-2 pl-2.5 text-left text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-[color-mix(in_oklab,var(--input-fill)_30%,transparent)]" />
      }>
        {Icon && <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
        <span className="min-w-0 flex-1 truncate">{current?.name ?? value}</span>
        <ChevronDown className="size-4 shrink-0 text-muted-foreground" aria-hidden />
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner sideOffset={4} align="start" collisionPadding={16} className="z-50">
          <Popover.Popup className="w-64 max-w-[calc(100vw-32px)] rounded-lg bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10">
            <Input autoFocus type="search" aria-label={t("rules.field_search")} placeholder={t("rules.field_search")} value={q} aria-controls={listId}
              aria-activedescendant={shown[at] ? `${listId}-${at}` : undefined}
              onChange={(e) => { setQ(e.target.value); setAt(0); }}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") { e.preventDefault(); setAt((i) => Math.min(i + 1, shown.length - 1)); }
                if (e.key === "ArrowUp") { e.preventDefault(); setAt((i) => Math.max(i - 1, 0)); }
                if (e.key === "Enter" && !e.nativeEvent.isComposing) { e.preventDefault(); if (shown[at]) pick(shown[at].key); }
              }} />
            <ul id={listId} role="listbox" aria-label={label} className="mt-1 max-h-64 overflow-y-auto">
              {shown.map((e, i) => {
                const I = TYPE_ICONS[e.type];
                return (
                  <li key={e.key} id={`${listId}-${i}`} role="option" aria-selected={e.key === value} data-active={i === at || undefined}
                    className="flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-sm data-active:bg-accent data-active:text-accent-foreground"
                    onMouseMove={() => setAt(i)} onClick={() => pick(e.key)}>
                    <I className="size-4 shrink-0 text-muted-foreground" aria-hidden /><span className="min-w-0 flex-1 truncate">{e.name}</span>
                    {e.key === value && <Check className="size-4 shrink-0" aria-hidden />}
                  </li>
                );
              })}
              {shown.length === 0 && <li className="px-2 py-1.5 text-sm text-muted-foreground">{t("rules.no_fields")}</li>}
            </ul>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}

/** Date operators in sections: the day itself, a range, relative to today, filled in. */
const DATE_SECTIONS: ["day" | "range" | "relative" | "fill", EditOp[]][] = [
  ["day", ["eq", "neq"]], ["range", ["lt", "gt", "lte", "gte", "between"]], ["relative", ["last_days", "next_days"]], ["fill", ["empty", "not_empty"]],
];

const PHONE = "(max-width: 639px)";
const subscribePhone = (cb: () => void) => { const m = window.matchMedia(PHONE); m.addEventListener("change", cb); return () => m.removeEventListener("change", cb); };
const usePhone = () => useSyncExternalStore(subscribePhone, () => window.matchMedia(PHONE).matches, () => false);

type DraftGroup = { any: boolean; rules: DraftRule[] };
type Draft = { any: boolean; groups: DraftGroup[] };

/** The names of the records a filter points at, for writing it as text. */
async function idNames(client: QueryClient, w: Where, entries: Entry[]): Promise<Record<string, string>> {
  const byTarget = new Map<string, Set<string>>();
  for (const r of [...w.rules, ...(w.groups ?? []).flatMap((g) => g.rules)]) {
    const e = entries.find((x) => x.key === r.field);
    if (!e || (e.kind !== "ref" && e.kind !== "refs")) continue;
    const ids = (Array.isArray(r.value) ? r.value : [r.value]).filter((v): v is string => typeof v === "string");
    if (ids.length) byTarget.set(e.target, new Set([...(byTarget.get(e.target) ?? []), ...ids]));
  }
  const out: Record<string, string> = {};
  await Promise.all([...byTarget].map(async ([target, ids]) => {
    const [meta, found] = await Promise.all([
      client.fetchQuery({ queryKey: ["collections", "detail", target], queryFn: () => collections.byID(target) }),
      collections.at(target).then((c) => records.list(c, { page: 1, limit: 100, ids: [...ids].join(",") })),
    ]);
    for (const r of found.items) { const name = recordTitle(meta.schema ?? null, r.data); if (name) out[r.id] = name; }
  }));
  return out;
}

/** Records named in the text, looked up by exact title: one match each, or an error. */
async function resolveNames(client: QueryClient, w: Where, refs: RefName[], entries: Entry[], src: string): Promise<{ where: Where } | { error: TextError }> {
  const ids = new Map<string, string>();
  for (const r of refs) {
    const k = `${r.target}\n${low(r.name)}`;
    if (ids.has(k)) continue;
    const [meta, found] = await Promise.all([
      client.fetchQuery({ queryKey: ["collections", "detail", r.target], queryFn: () => collections.byID(r.target) }),
      collections.at(r.target).then((c) => records.list(c, { page: 1, limit: 20, search: r.name })),
    ]);
    const hits = found.items.filter((x) => low(recordTitle(meta.schema ?? null, x.data) ?? "") === low(r.name));
    if (hits.length !== 1) {
      const field = entries.find((e) => e.key === r.field)?.name ?? r.field;
      return { error: { key: hits.length ? "ref_many" : "ref_none", params: { field, x: r.name }, at: Math.max(0, src.indexOf(r.name) - 1) } };
    }
    ids.set(k, hits[0].id);
  }
  const target = (field: string) => entries.find((e) => e.key === field)?.target ?? "";
  const swap = (field: string, v: unknown) => { const n = refNameOf(v); return n === undefined ? v : ids.get(`${target(field)}\n${low(n)}`); };
  const fix = (r: Rule): Rule => (r.value === undefined ? r : { ...r, value: Array.isArray(r.value) ? r.value.map((v) => swap(r.field, v)) : swap(r.field, r.value) });
  return { where: { ...w, rules: w.rules.map(fix), ...(w.groups ? { groups: w.groups.map((g) => ({ ...g, rules: g.rules.map(fix) })) } : {}) } };
}

/**
 * A "Conditions" button: rules that compare fields (greater than, contains,
 * empty…), joined by AND or OR, and groups of rules for mixes such as
 * (A and B) or (C and D), edited row by row or written as text.
 *
 * Applied rules show in a bar of their own (in `bar`, an element below the
 * toolbar, or right after the button): plain AND rules as one chip each, edited
 * in place from the chip; a mix of OR and groups as one summary chip that opens
 * the editor. With the collection's id, text values offer the field's
 * commonest values; with `count` the apply buttons tell how many records the
 * edited rules keep, and `saveView` lets them be kept as a saved view.
 */
export function RecordRules({ fields: all, value, onChange, collection, count, saveView, bar, summary }: {
  fields: FieldRow[]; value: Where; onChange: (w: Where) => void; collection?: CollectionRef;
  /** The list's search, for counting what the edited rules keep. */
  count?: { search?: string };
  saveView?: boolean;
  /** Where the applied rules show; without it, right after the button. */
  bar?: HTMLElement | null;
  /** Shown at the end of the bar, such as "2 of 40 records". */
  summary?: React.ReactNode;
}) {
  const { t, locale } = useI18n();
  const lang: Lang = locale === "en-US" ? "en" : "pt";
  const client = useQueryClient();
  const phone = usePhone();
  const entries = entriesOf(all, t);
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Draft>({ any: false, groups: [] });
  const [mode, setMode] = useState<"visual" | "text">("visual");
  const [text, setText] = useState("");
  const [textBase, setTextBase] = useState("");
  const [cursor, setCursor] = useState(0);
  const [refError, setRefError] = useState<TextError | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [viewName, setViewName] = useState<string | null>(null);
  // One rule edited from its chip: its place in value.rules (-1: a new one) and the chip it hangs from.
  const [quick, setQuick] = useState<{ at: number; rule: DraftRule; anchor: Element } | null>(null);
  const textBox = useRef<HTMLTextAreaElement>(null);
  const entryOf = (key: string) => entries.find((f) => f.key === key);
  const kindOfKey = (key: string): Kind => entryOf(key)?.kind ?? "text";
  const joinWord = (any: boolean) => t(any ? "rules.or" : "rules.and");
  const groups = (value.groups ?? []).filter((g) => g.rules.length);
  const applied = value.rules.length + groups.reduce((n, g) => n + g.rules.length, 0);
  // Plain AND rules are edited chip by chip; OR and groups need the editor.
  const simple = !groups.length && (!value.any || value.rules.length <= 1);
  const blank = (): DraftRule => ({ field: entries[0].key, op: opsFor(entries[0].kind)[0], raw: "", raw2: "", list: [] });
  const draftOf = (r: Rule) => toDraft(r, kindOfKey(r.field));

  // Without groups the rules are one group; with them, a loose rule is a group of one.
  const draftFrom = (w: Where): Draft => {
    const gs = (w.groups ?? []).filter((g) => g.rules.length);
    const next = gs.length
      ? { any: w.any, groups: [...w.rules.map((r) => ({ any: false, rules: [draftOf(r)] })), ...gs.map((g) => ({ any: g.any, rules: g.rules.map(draftOf) }))] }
      : { any: false, groups: [{ any: w.any, rules: w.rules.map(draftOf) }] };
    if (next.groups.length === 1 && next.groups[0].rules.length === 0) next.groups[0].rules.push(blank());
    return next;
  };
  const whereOf = (d: Draft): Where => {
    const done = d.groups.map((g) => ({ any: g.any, rules: g.rules.map((r) => fromDraft(r, kindOfKey(r.field))).filter((r): r is Rule => !!r) })).filter((g) => g.rules.length);
    if (done.length <= 1) return { any: done[0]?.any ?? false, rules: done[0]?.rules ?? [] };
    const loose = done.filter((g) => g.rules.length === 1).map((g) => g.rules[0]);
    const many = done.filter((g) => g.rules.length > 1);
    return many.length ? { any: d.any, rules: loose, groups: many } : { any: d.any, rules: loose };
  };
  /** The filter with the chip's rule put in (or left out while incomplete). */
  const quickWhere = (q: NonNullable<typeof quick>): Where => {
    const r = fromDraft(q.rule, kindOfKey(q.rule.field));
    const rules = q.at < 0 ? [...value.rules, ...(r ? [r] : [])] : value.rules.flatMap((x, i) => (i !== q.at ? [x] : r ? [r] : []));
    return { any: false, rules };
  };

  const parsed = open && mode === "text" ? parseRuleText(text, entries) : null;
  const textError = parsed && "error" in parsed ? parsed.error : refError;
  // Rules still holding names to look up are not counted until applied.
  const pending: Where | null = quick ? quickWhere(quick) : !open ? null : mode === "visual" ? whereOf(draft) : parsed && "where" in parsed && !parsed.refNames.length ? parsed.where : null;
  const dirty = (mode === "text" && text !== textBase) || whereText(whereOf(draft)) !== whereText(value);
  const counted = useDebounced(pending ? whereText(pending) : null, 250);
  const live = useQuery({
    queryKey: ["records", collection?.id, "count", counted, count?.search],
    queryFn: ({ signal }) => records.list(collection!, { page: 1, limit: 1, search: count?.search || undefined, where: counted || undefined }, signal),
    enabled: (open || !!quick) && !!count && !!collection && counted !== null,
    placeholderData: keepPreviousData, staleTime: 30_000,
  });
  const create = useMutation({
    mutationFn: (p: { name: string; where: Where }) => {
      const q = new URLSearchParams(window.location.search);
      q.delete("page"); q.delete("filter");
      const w = whereText(p.where);
      if (w) q.set("where", w); else q.delete("where");
      return tools.createView(collection!, { name: p.name, query: q.toString(), shared: false });
    },
    onSuccess: () => { setViewName(null); toast.success(t("views.saved")); void client.invalidateQueries({ queryKey: ["views", collection?.id] }); },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });

  const show = () => {
    setQuick(null);
    setDraft(draftFrom(value)); setMode("visual"); setText(""); setTextBase(""); setRefError(null);
    setConfirm(false); setViewName(null); setOpen(true);
  };
  const finish = () => { setOpen(false); setConfirm(false); };
  // Clicking outside or Escape with edits not applied asks first.
  const onOpenChange = (o: boolean) => {
    if (o) return show();
    if (dirty && !confirm) return setConfirm(true);
    finish();
  };
  /** The text read into rules, names looked up; null (and the error shown) when it fails. */
  const textWhere = async (): Promise<Where | null> => {
    const r = parseRuleText(text, entries);
    if ("error" in r) return null;
    if (!r.refNames.length) return r.where;
    setBusy(true);
    try {
      const res = await resolveNames(client, r.where, r.refNames, entries, text);
      if ("error" in res) { setRefError(res.error); return null; }
      return res.where;
    } finally { setBusy(false); }
  };
  const toMode = async (m: "visual" | "text") => {
    if (m === mode) return;
    if (m === "text") {
      const w = whereOf(draft);
      setBusy(true);
      const names = await idNames(client, w, entries).catch(() => ({}));
      setBusy(false);
      const s = ruleText(w, entries, lang, names);
      setText(s); setTextBase(s); setCursor(s.length); setRefError(null); setMode("text");
      requestAnimationFrame(() => textBox.current?.focus());
      return;
    }
    const w = await textWhere();
    if (!w) return;
    setDraft(draftFrom(w)); setMode("visual");
  };
  const apply = async () => {
    const w = mode === "visual" ? whereOf(draft) : await textWhere();
    if (!w) return;
    onChange(w);
    finish();
  };

  /** A rule with some parts changed: a new field starts empty, keeping the operator only when it still fits. */
  const patched = (r: DraftRule, patch: Partial<DraftRule>): DraftRule => {
    const next = { ...r, ...patch };
    if (patch.field && patch.field !== r.field) {
      const ops = opsFor(kindOfKey(patch.field));
      Object.assign(next, { raw: "", raw2: "", list: [], op: ops.includes(next.op) ? next.op : ops[0] });
    }
    // A day count and a date do not share a value.
    if (patch.op && (["last_days", "next_days"].includes(patch.op) !== ["last_days", "next_days"].includes(r.op))) Object.assign(next, { raw: "", raw2: "" });
    return next;
  };
  // A group left empty goes, unless it is the only one.
  const setGroup = (gi: number, patch: (g: DraftGroup) => DraftGroup) =>
    setDraft((d) => {
      const gs = d.groups.map((g, j) => (j === gi ? patch(g) : g));
      const kept = gs.filter((g) => g.rules.length);
      return { ...d, groups: kept.length ? kept : [gs[0]] };
    });
  const setRule = (gi: number, i: number, patch: Partial<DraftRule>) => setGroup(gi, (g) => ({ ...g, rules: g.rules.map((r, j) => (j === i ? patched(r, patch) : r)) }));
  const total = draft.groups.reduce((n, g) => n + g.rules.length, 0);
  const grouped = draft.groups.length > 1;
  const duplicate = (gi: number, i: number) => setGroup(gi, (g) => ({ ...g, rules: [...g.rules.slice(0, i + 1), { ...g.rules[i], list: [...g.rules[i].list] }, ...g.rules.slice(i + 1)] }));
  // The rule becomes a group of its own with a new blank rule, joined the other way round.
  const toGroup = (gi: number, i: number) => setDraft((d) => {
    const top: Draft = d.groups.length > 1 ? d : { any: d.groups[0].any, groups: d.groups[0].rules.map((r) => ({ any: false, rules: [r] })) };
    const at = d.groups.length > 1 ? gi : i;
    const g = top.groups[at];
    const rule = d.groups.length > 1 ? g.rules[i] : g.rules[0];
    const fresh = { any: !top.any, rules: [rule, blank()] };
    const gs = [...top.groups];
    if (g.rules.length === 1) gs.splice(at, 1, fresh);
    else gs.splice(at, 1, { ...g, rules: g.rules.filter((_, j) => j !== i) }, fresh);
    return { ...top, groups: gs };
  });
  // A loose rule: in the one group while there are no groups, else a group of one.
  const addRule = () => setDraft((d) => (d.groups.length > 1 ? { ...d, groups: [...d.groups, { any: false, rules: [blank()] }] } : { ...d, groups: [{ ...d.groups[0], rules: [...d.groups[0].rules, blank()] }] }));
  const addGroup = () => setDraft((d) => {
    // The rules so far stay one bracket (or one rule); the new group joins them by OR.
    const first = d.groups.length === 1 ? [d.groups[0]] : d.groups;
    return { any: d.groups.length === 1 ? true : d.any, groups: [...first, { any: false, rules: [blank(), blank()] }] };
  });

  if (entries.length === 0) return null;

  /** The join between two items as a word that switches AND and OR on a click. */
  const joinToggle = (any: boolean, label: string, onPick: (any: boolean) => void) => (
    <button type="button" aria-label={`${label}: ${joinWord(any)}`} title={t("rules.join_switch")}
      className="h-[var(--control-h)] rounded-md px-1.5 text-left text-sm font-medium text-primary underline decoration-dotted underline-offset-4 hover:bg-primary/10"
      onClick={() => onPick(!any)}>{joinWord(any)}</button>
  );
  /** "Where" before the first item; the join word after it. */
  const connector = (i: number, any: boolean, label: string, onPick: (any: boolean) => void) =>
    i === 0 ? <span className="block py-1.5 text-sm text-muted-foreground">{t("rules.where")}</span> : joinToggle(any, label, onPick);
  const valueOf = (r: DraftRule, n: number, set: (patch: Partial<DraftRule>) => void) => {
    const entry = entryOf(r.field);
    if (!entry || !needsValue(r.op)) return <span />;
    const label = String(n);
    const kind = entry.kind;
    if (r.op === "last_days" || r.op === "next_days")
      return <div className="flex items-center gap-1.5"><Input type="number" min={0} inputMode="numeric" aria-label={`${t("rules.value")} ${label}`} value={r.raw} onChange={(e) => set({ raw: e.target.value })} /><span className="text-sm text-muted-foreground">{t("rules.days")}</span></div>;
    if (kind === "ref" || kind === "refs") {
      const many = LIST_OPS.includes(r.op);
      return <RecordsValue entry={entry} many={many} ids={many ? r.list : r.raw ? [r.raw] : []} label={label} onChange={(ids) => set(many ? { list: ids } : { raw: ids[0] ?? "" })} />;
    }
    if (kind === "enum" && LIST_OPS.includes(r.op)) return <OptionsValue options={entry.options} colors={entry.colors} picked={r.list} label={`${t("rules.value")} ${label}`} onChange={(list) => set({ list })} />;
    if (kind === "boolean" || kind === "enum") {
      const items = kind === "boolean" ? { true: t("common.yes"), false: t("common.no") } : Object.fromEntries(entry.options.map((o) => [o, o]));
      return (
        <Select value={r.raw || null} items={items} onValueChange={(v) => set({ raw: String(v) })}>
          <SelectTrigger className="w-full" aria-label={`${t("rules.value")} ${label}`}><SelectValue placeholder={t("rules.value")} /></SelectTrigger>
          <SelectContent>{Object.entries(items).map(([v, l]) => (
            <SelectItem key={v} value={v}>{kind === "enum" && <span className={cn("size-2 rounded-full", optionColor(entry.colors[v]).dot)} aria-hidden />}{l}</SelectItem>
          ))}</SelectContent>
        </Select>
      );
    }
    if (kind === "date") {
      if (r.op !== "between") return <DateValue raw={r.raw} label={label} onChange={(raw) => set({ raw })} />;
      return <div className="space-y-1.5"><DateValue raw={r.raw} label={label} onChange={(raw) => set({ raw })} /><DateValue raw={r.raw2} label={`${label} ${t("rules.and")}`} onChange={(raw2) => set({ raw2 })} /></div>;
    }
    if (kind === "number") {
      const box = (v: string, l: string, key: "raw" | "raw2") => <Input type="number" inputMode="decimal" aria-label={l} value={v} placeholder={t("rules.value")} onChange={(e) => set({ [key]: e.target.value })} />;
      if (r.op !== "between") return box(r.raw, `${t("rules.value")} ${label}`, "raw");
      return <div className="flex items-center gap-1.5">{box(r.raw, `${t("rules.value")} ${label}`, "raw")}<span className="text-sm text-muted-foreground">{t("rules.and")}</span>{box(r.raw2, `${t("rules.value_to")} ${label}`, "raw2")}</div>;
    }
    return <TextValue collection={collection} entry={entry} raw={r.raw} label={label} onChange={(raw) => set({ raw })} />;
  };
  const opSelect = (r: DraftRule, n: number, set: (patch: Partial<DraftRule>) => void) => {
    const kind = kindOfKey(r.field);
    const ops = opsFor(kind);
    const items = Object.fromEntries(ops.map((o) => [o, opLabel(o, kind, t)]));
    const item = (o: EditOp) => <SelectItem key={o} value={o}>{items[o]}</SelectItem>;
    return (
      <Select value={r.op} items={items} onValueChange={(v) => set({ op: v as EditOp })}>
        <SelectTrigger className="w-full" aria-label={`${t("rules.op_label")} ${n}`}><SelectValue /></SelectTrigger>
        <SelectContent>
          {kind === "date"
            ? DATE_SECTIONS.map(([k, list], si) => (
              <SelectGroup key={k}>
                {si > 0 && <SelectSeparator />}
                <SelectLabel>{t(`rules.op_group.${k}`)}</SelectLabel>
                {list.filter((o) => ops.includes(o)).map(item)}
              </SelectGroup>
            ))
            : ops.map(item)}
        </SelectContent>
      </Select>
    );
  };
  const fieldPicker = (r: DraftRule, n: number, set: (patch: Partial<DraftRule>) => void) =>
    <FieldPicker entries={entries} value={r.field} label={`${t("rules.field")} ${n}`} onPick={(key) => set({ field: key })} />;
  const COLS = "grid-cols-[3rem_minmax(0,1fr)_9.5rem_minmax(0,1.3fr)_4rem]";
  /** One rule: where/join, field, operator, value, and its actions on hover. */
  const ruleRow = (gi: number, r: DraftRule, i: number, n: number, join: React.ReactNode, cols = COLS) => {
    const name = entryOf(r.field)?.name ?? r.field;
    const set = (patch: Partial<DraftRule>) => setRule(gi, i, patch);
    const actions = (
      <div className="flex justify-end opacity-0 transition-opacity group-hover/row:opacity-100 focus-within:opacity-100 has-[[aria-expanded=true]]:opacity-100 max-sm:opacity-100">
        <DropdownMenu>
          <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="icon-sm" aria-label={`${t("rules.more")}: ${name}`} />}><MoreHorizontal className="size-4" /></DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem disabled={total >= MAX_RULES} onClick={() => duplicate(gi, i)}><Copy />{t("rules.duplicate")}</DropdownMenuItem>
            <DropdownMenuItem disabled={total >= MAX_RULES || draft.groups.length >= MAX_GROUPS} onClick={() => toGroup(gi, i)}><Group />{t("rules.to_group")}</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <IconAction label={t("rules.remove")} target={name} onClick={() => setGroup(gi, (x) => ({ ...x, rules: x.rules.filter((_, j) => j !== i) }))}><X className="size-4" /></IconAction>
      </div>
    );
    if (phone) return (
      <li key={`${gi}-${i}`} className="group/row space-y-1.5 border-b pb-3 last:border-b-0">
        <div className="flex items-center gap-2">{join}<span className="flex-1" />{actions}</div>
        {fieldPicker(r, n, set)}{opSelect(r, n, set)}{valueOf(r, n, set)}
      </li>
    );
    return (
      <li key={`${gi}-${i}`} className={cn("group/row grid items-start gap-1.5", cols)}>
        {join}{fieldPicker(r, n, set)}{opSelect(r, n, set)}{valueOf(r, n, set)}{actions}
      </li>
    );
  };

  const suggestions = mode === "text" ? suggestAt(text, cursor, entries, lang) : { from: 0, items: [] };
  const take = (item: string) => {
    const end = cursor;
    const next = `${text.slice(0, suggestions.from)}${item} ${text.slice(end).replace(/^\s+/, "")}`;
    const at = suggestions.from + item.length + 1;
    setText(next); setCursor(at); setRefError(null);
    requestAnimationFrame(() => { textBox.current?.focus(); textBox.current?.setSelectionRange(at, at); });
  };
  const counter = (on: boolean) => count && on && live.data !== undefined && <span className="font-normal opacity-80">· {plural(live.data.pagination.total_items, t)}</span>;
  // Rules are numbered across the groups, so each control keeps a distinct name.
  let n = 0;
  const rows = grouped ? (
    draft.groups.map((g, gi) => {
      const top = connector(gi, draft.any, t("rules.join_groups").replace(":", ""), (any) => setDraft((d) => ({ ...d, any })));
      if (g.rules.length === 1) return ruleRow(gi, g.rules[0], 0, ++n, top);
      return (
        <li key={`g${gi}`} className={cn("gap-1.5", !phone && "grid grid-cols-[3rem_minmax(0,1fr)] items-start")}>
          {top}
          <section aria-label={t("rules.group").replace("{n}", String(gi + 1))} className="space-y-2 border-l-2 border-primary/30 pl-3 max-sm:mt-1.5">
            <ul className="space-y-2">
              {g.rules.map((r, i) => ruleRow(gi, r, i, ++n, connector(i, g.any, `${t("rules.join").replace(":", "")} ${n}`, (any) => setGroup(gi, (x) => ({ ...x, any }))),
                "grid-cols-[2.75rem_minmax(0,1fr)_8.5rem_minmax(0,1.2fr)_4rem]"))}
            </ul>
            <Button type="button" variant="ghost" size="sm" className="h-7 text-xs text-muted-foreground" disabled={total >= MAX_RULES} onClick={() => setGroup(gi, (x) => ({ ...x, rules: [...x.rules, blank()] }))}>
              <Plus className="size-3.5" />{t("rules.add_in_group")}
            </Button>
          </section>
        </li>
      );
    })
  ) : (
    draft.groups[0]?.rules.map((r, i) => ruleRow(0, r, i, ++n, connector(i, draft.groups[0].any, `${t("rules.join").replace(":", "")} ${n}`, (any) => setGroup(0, (x) => ({ ...x, any })))))
  );
  const editor = (
    // React carries submit up through the portal: stop it before an outer form (a widget, an automation) takes it.
    <form className="flex min-h-0 flex-1 flex-col gap-3" onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); void apply(); }}>
      {mode === "text" ? (
        <div className="space-y-2">
          <Textarea ref={textBox} aria-label={t("rules.text_label")} rows={phone ? 6 : 3} spellCheck={false} className="font-mono text-sm" value={text}
            aria-invalid={!!textError || undefined} aria-describedby="rules-text-help"
            onChange={(e) => { setText(e.target.value); setCursor(e.target.selectionStart); setRefError(null); }}
            onSelect={(e) => setCursor(e.currentTarget.selectionStart)}
            onKeyDown={(e) => {
              if (e.key === "Tab" && !e.shiftKey && suggestions.items.length) { e.preventDefault(); take(suggestions.items[0]); }
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); e.currentTarget.form?.requestSubmit(); }
            }} />
          {suggestions.items.length > 0 && (
            <ul aria-label={t("rules.suggest")} className="flex flex-wrap gap-1">
              {suggestions.items.map((s, i) => (
                <li key={s}><button type="button" className={cn("rounded-md border px-1.5 py-0.5 font-mono text-xs hover:bg-accent", i === 0 && "border-primary/40 bg-primary/5")} onClick={() => take(s)}>{s}</button></li>
              ))}
            </ul>
          )}
          {textError
            ? <p role="alert" className="text-sm text-destructive">{errorText(textError, t)} <span className="text-xs opacity-80">({t("rules.err_at").replace("{n}", String(textError.at + 1))})</span></p>
            : <p id="rules-text-help" className="text-xs text-muted-foreground">{t("rules.text_hint")}</p>}
        </div>
      ) : (
        <ul className={cn("min-h-0 space-y-2 overflow-y-auto", !phone && "max-h-[26rem] pr-1")}>{rows}</ul>
      )}
      {confirm ? (
        <div role="alertdialog" aria-label={t("rules.discard_q")} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 p-2 text-sm">
          <span>{t("rules.discard_q")}</span>
          <div className="flex gap-2">
            <Button type="button" size="sm" variant="ghost" onClick={finish}>{t("rules.discard")}</Button>
            <Button type="button" size="sm" autoFocus onClick={() => setConfirm(false)}>{t("rules.keep_editing")}</Button>
          </div>
        </div>
      ) : viewName !== null ? (
        <div className="flex gap-2 border-t pt-3">
          <Input autoFocus maxLength={80} aria-label={t("views.name")} placeholder={t("views.name")} value={viewName} onChange={(e) => setViewName(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && !e.nativeEvent.isComposing) { e.preventDefault(); e.stopPropagation(); if (viewName.trim() && pending) create.mutate({ name: viewName.trim(), where: pending }); } if (e.key === "Escape") { e.stopPropagation(); setViewName(null); } }} />
          <Button type="button" variant="ghost" onClick={() => setViewName(null)}>{t("common.cancel")}</Button>
          <Button type="button" disabled={!viewName.trim() || !pending || create.isPending} onClick={() => pending && create.mutate({ name: viewName.trim(), where: pending })}>{t("common.save")}</Button>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-1 border-t pt-3">
          {mode === "visual" && (
            <DropdownMenu>
              <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="sm" disabled={total >= MAX_RULES} />}><Plus className="size-4" />{t("rules.add_menu")}<ChevronDown className="size-3.5 opacity-60" /></DropdownMenuTrigger>
              <DropdownMenuContent align="start">
                <DropdownMenuItem onClick={addRule}><SlidersHorizontal />{t("rules.add_rule")}</DropdownMenuItem>
                <DropdownMenuItem disabled={total + 2 > MAX_RULES || draft.groups.length >= MAX_GROUPS} onClick={addGroup}><Group />{t("rules.add_group_item")}</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )}
          <Button type="button" variant="ghost" size="sm" className="text-muted-foreground" disabled={busy} onClick={() => void toMode(mode === "visual" ? "text" : "visual")}>
            {mode === "visual" ? <><Code className="size-4" />{t("rules.as_text")}</> : <><SlidersHorizontal className="size-4" />{t("rules.as_visual")}</>}
          </Button>
          <span className="flex-1" />
          {saveView && collection && <Button type="button" variant="ghost" size="sm" className="text-muted-foreground" disabled={!pending} onClick={() => setViewName("")}><Bookmark className="size-4" />{t("rules.save_view")}</Button>}
          <Button type="submit" size="sm" className="ml-auto" disabled={busy || (mode === "text" && !!parsed && "error" in parsed)}>
            {busy && <Loader2 className="size-4 animate-spin" />}{t("filters.apply")}{counter(open)}
          </Button>
        </div>
      )}
    </form>
  );
  const trigger = <><SlidersHorizontal className="size-4" />{t("rules.title")}{applied > 0 && <span className="rounded-full bg-primary/15 px-1.5 text-xs text-primary">{applied}</span>}</>;

  const editQuick = (at: number, anchor: Element) => setQuick({ at, anchor, rule: at < 0 ? blank() : draftOf(value.rules[at]) });
  const applyQuick = () => { if (quick) onChange(quickWhere(quick)); setQuick(null); };
  const qn = quick ? (quick.at < 0 ? value.rules.length + 1 : quick.at + 1) : 0;
  const setQuickRule = (patch: Partial<DraftRule>) => setQuick((q) => (q ? { ...q, rule: patched(q.rule, patch) } : q));
  const quickEditor = quick && (
    <Popover.Root open onOpenChange={(o) => { if (!o) setQuick(null); }}>
      <Popover.Portal>
        <Popover.Positioner anchor={quick.anchor} sideOffset={6} align="start" collisionPadding={16} className="z-50">
          <Popover.Popup aria-label={t("rules.edit")} className="w-80 max-w-[calc(100vw-32px)] rounded-lg bg-popover p-3 text-popover-foreground shadow-md ring-1 ring-foreground/10">
            <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); applyQuick(); }}>
              {fieldPicker(quick.rule, qn, setQuickRule)}
              {opSelect(quick.rule, qn, setQuickRule)}
              {valueOf(quick.rule, qn, setQuickRule)}
              <div className="flex items-center gap-1 border-t pt-2">
                {quick.at >= 0 && <Button type="button" variant="ghost" size="sm" className="text-muted-foreground" onClick={() => { onChange({ any: false, rules: value.rules.filter((_, i) => i !== quick.at) }); setQuick(null); }}><Trash2 className="size-4" />{t("rules.remove_short")}</Button>}
                <span className="flex-1" />
                <Button type="submit" size="sm" disabled={!fromDraft(quick.rule, kindOfKey(quick.rule.field))}>{t("filters.apply")}{counter(true)}</Button>
              </div>
            </form>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );

  /** A rule as a chip: field, operator, value; a click edits it. */
  const chip = (r: Rule, at: number, onRemove: () => void) => {
    const entry = entryOf(r.field);
    const words = ruleWords(entry, r, t);
    const d = toDraft(r, entry?.kind ?? "text");
    const value = <RuleValue entry={entry} rule={r} t={t} />;
    return (
      <li key={`c${at}`} className="flex h-7 items-center overflow-hidden rounded-full border bg-card text-xs shadow-xs">
        <button type="button" className="flex h-full items-center hover:bg-muted/60" aria-label={`${t("rules.edit")}: ${words}`} onClick={(e) => editQuick(at, e.currentTarget.parentElement!)}>
          <span className="pl-2.5 pr-1.5 text-muted-foreground">{entry?.name ?? r.field}</span>{" "}
          <span className="border-x px-1.5 text-muted-foreground">{opShort(d.op, entry?.kind ?? "text", t)}</span>{" "}
          {needsValue(d.op) && <span className="max-w-48 truncate px-1.5 font-medium">{value}</span>}
        </button>
        <button type="button" className="grid h-full w-6 place-items-center text-muted-foreground hover:bg-muted/60 hover:text-foreground" aria-label={`${t("rules.remove")}: ${words}`} onClick={onRemove}><X className="size-3" /></button>
      </li>
    );
  };
  const chips = applied > 0 && (
    <div className="flex flex-wrap items-center gap-1.5">
      <ul className="flex flex-wrap items-center gap-1.5" aria-label={t("rules.active")}>
        {simple
          ? value.rules.map((r, i) => chip(r, i, () => onChange({ ...value, rules: value.rules.filter((_, j) => j !== i) })))
          : (
            <li className="flex h-7 items-center overflow-hidden rounded-full border border-primary/30 bg-primary/5 text-xs">
              <button type="button" className="flex h-full items-center gap-1.5 pl-2.5 pr-1.5 hover:bg-primary/10" aria-label={`${t("rules.advanced")}: ${applied}`} onClick={show}>
                <Group className="size-3.5 text-primary" aria-hidden /><span className="font-medium">{t("rules.advanced")}</span>{" "}
                <span className="text-muted-foreground">· {t(applied === 1 ? "rules.count_one" : "rules.count_many").replace("{n}", String(applied))}</span>
              </button>
              <button type="button" className="grid h-full w-6 place-items-center text-muted-foreground hover:bg-primary/10" aria-label={`${t("rules.remove_all")}`} onClick={() => onChange({ any: false, rules: [] })}><X className="size-3" /></button>
            </li>
          )}
      </ul>
      {value.rules.length < MAX_RULES && (
        <Button type="button" variant="ghost" size="sm" className="h-7 px-2 text-xs text-muted-foreground" onClick={(e) => (simple ? editQuick(-1, e.currentTarget) : show())}>
          <Plus className="size-3.5" />{t("rules.add_filter")}
        </Button>
      )}
      <span className="flex-1" />
      {summary && <span className="text-xs text-muted-foreground" role="status">{summary}</span>}
      {simple && value.rules.length > 1 && <Button type="button" variant="ghost" size="sm" className="h-7 px-2 text-xs text-muted-foreground" onClick={() => onChange({ any: false, rules: [] })}>{t("filters.clear")}</Button>}
    </div>
  );
  return (
    <>
      {phone ? (
        <Sheet open={open} onOpenChange={onOpenChange}>
          <Button variant="outline" onClick={() => show()}>{trigger}</Button>
          <SheetContent side="bottom" className="h-[92dvh] gap-3 rounded-t-xl p-4" showCloseButton={false}>
            <SheetTitle>{t("rules.title")}</SheetTitle>
            {editor}
          </SheetContent>
        </Sheet>
      ) : (
        <Popover.Root open={open} onOpenChange={onOpenChange}>
          <Popover.Trigger render={<Button variant="outline" />}>{trigger}</Popover.Trigger>
          <Popover.Portal>
            <Popover.Positioner sideOffset={4} align="start" collisionPadding={16} className="z-50">
              <Popover.Popup aria-label={t("rules.title")} className="flex w-[44rem] max-w-[calc(100vw-32px)] flex-col rounded-xl bg-popover p-3 text-popover-foreground shadow-lg ring-1 ring-foreground/10">
                {editor}
              </Popover.Popup>
            </Popover.Positioner>
          </Popover.Portal>
        </Popover.Root>
      )}
      {quickEditor}
      {chips && (bar ? createPortal(chips, bar) : <div className="basis-full">{chips}</div>)}
    </>
  );
}

const plural = (n: number, t: Translate) => (n === 1 ? t("nav.record_one") : t("nav.record_many").replace("{n}", n.toLocaleString()));

function errorText(e: TextError, t: Translate): string {
  return Object.entries(e.params).reduce((s, [k, v]) => s.replaceAll(`{${k}}`, v), t(`rules.err.${e.key}` as MessageKey));
}

const HINT_KEY = "dataset.cell-filter-hint";

/** Once per browser, a short note: right-clicking a cell filters by it. */
export function CellFilterHint() {
  const { t } = useI18n();
  useEffect(() => {
    try { if (localStorage.getItem(HINT_KEY) === "1") return; } catch { return; }
    const id = setTimeout(() => {
      try { localStorage.setItem(HINT_KEY, "1"); } catch { /* shows again next time */ }
      toast(t("rules.hint_cell"), { duration: 8000 });
    }, 800);
    return () => clearTimeout(id);
  }, [t]);
  return null;
}

/** The quick rules a cell offers: keep or drop its value (or its emptiness). */
export function cellRules(row: FieldRow, value: unknown): { keep: Rule; drop?: Rule } | null {
  const kind = kindOf(row.field);
  const blankValue = value === undefined || value === null || value === "" || (Array.isArray(value) && value.length === 0);
  if (blankValue) return { keep: { field: row.key, op: "empty" }, drop: { field: row.key, op: "not_empty" } };
  if (kind === "strings" || kind === "refs") return Array.isArray(value) && value.length ? { keep: { field: row.key, op: "has", value: value[0] } } : null;
  if (kind === "date" && typeof value === "string") return { keep: { field: row.key, op: "eq", value: value.slice(0, 10) }, drop: { field: row.key, op: "neq", value: value.slice(0, 10) } };
  if (["text", "number", "enum", "boolean", "ref"].includes(kind) && ["string", "number", "boolean"].includes(typeof value))
    return { keep: { field: row.key, op: "eq", value }, drop: { field: row.key, op: "neq", value } };
  return null;
}

/**
 * Right-click (or long-press) on a list cell: keep only records with that value,
 * or leave them out. The rule joins the conditions already applied.
 */
export function CellFilter({ row, value, text, onAdd, render }: { row: FieldRow; value: unknown; text: string; onAdd: (rule: Rule) => void; render: React.ReactElement }) {
  const { t } = useI18n();
  const quick = cellRules(row, value);
  if (!quick) return render;
  const blankValue = quick.keep.op === "empty";
  const item = "flex cursor-default items-center gap-1.5 rounded-md px-1.5 py-1 text-sm outline-hidden select-none focus:bg-accent focus:text-accent-foreground [&_svg]:size-4";
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger render={render} />
      <ContextMenu.Portal>
        <ContextMenu.Positioner className="isolate z-50 outline-none">
          {/* React carries clicks up through the portal: the row would open the record. */}
          <ContextMenu.Popup aria-label={t("rules.quick")} onClick={(e) => e.stopPropagation()} className="min-w-52 max-w-80 rounded-lg bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10 outline-none">
            <p className="truncate px-1.5 py-1 text-xs font-medium text-muted-foreground">{row.field.name}: {blankValue ? "—" : text}</p>
            <ContextMenu.Item className={item} onClick={() => onAdd(quick.keep)}><Filter />{t(blankValue ? "rules.keep_empty" : "rules.keep_value")}</ContextMenu.Item>
            {quick.drop && <ContextMenu.Item className={item} onClick={() => onAdd(quick.drop!)}><FilterX />{t(blankValue ? "rules.drop_empty" : "rules.drop_value")}</ContextMenu.Item>}
          </ContextMenu.Popup>
        </ContextMenu.Positioner>
      </ContextMenu.Portal>
    </ContextMenu.Root>
  );
}
