"use client";

import { useId, useState } from "react";
import { ContextMenu } from "@base-ui/react/context-menu";
import { Popover } from "@base-ui/react/popover";
import { useQuery } from "@tanstack/react-query";
import { Filter, FilterX, Plus, Search, SlidersHorizontal, Trash2, X } from "lucide-react";
import { IconAction } from "@/components/bits";
import { VIRTUAL_TYPES, type Field, type FieldRow } from "@/components/field-builder";
import { RecordPicker, RefChip, useRecordLabel } from "@/components/schema-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { MessageKey } from "@/lib/i18n/messages";
import { records } from "@/lib/api/endpoints";
import { useI18n } from "@/lib/i18n/provider";

export type RuleOp = "eq" | "neq" | "contains" | "not_contains" | "starts_with" | "gt" | "gte" | "lt" | "lte" | "between" | "empty" | "not_empty" | "in" | "not_in" | "has" | "has_any";
export interface Rule { field: string; op: RuleOp; value?: unknown }
/** Rules joined by AND (any: false) or OR; groups bracket rules of their own, one level deep. */
export interface Where { any: boolean; rules: Rule[]; groups?: { any: boolean; rules: Rule[] }[] }

type Translate = ReturnType<typeof useI18n>["t"];

const MAX_RULES = 20;
const MAX_GROUPS = 10;

/** True when the filter keeps every record. */
export const whereEmpty = (w: Where) => w.rules.length === 0 && !(w.groups ?? []).some((g) => g.rules.length > 0);

/** The URL text of a filter, or "" when it filters nothing. */
export const whereText = (w: Where) => (whereEmpty(w) ? "" : JSON.stringify(w));

/**
 * w AND every rule of extra, still one level deep. An OR at the top spreads
 * over its parts: (a OR (b AND c)) AND x = (a AND x) OR (b AND c AND x).
 */
export function andWhere(w: Where, extra: Rule[]): Where {
  if (!extra.length) return w;
  if (whereEmpty(w)) return { any: false, rules: extra };
  const groups = (w.groups ?? []).filter((g) => g.rules.length);
  if (!w.any) return { any: false, rules: [...w.rules, ...extra], ...(groups.length ? { groups } : {}) };
  if (!groups.length) return { any: false, rules: extra, groups: [{ any: true, rules: w.rules }] };
  // Every part as an AND list: a lone rule, an AND group, or each rule of an OR group (OR inside OR is flat).
  const parts = [...w.rules.map((r) => [r]), ...groups.flatMap((g) => (g.any ? g.rules.map((r) => [r]) : [g.rules]))];
  return { any: true, rules: [], groups: parts.map((rules) => ({ any: false, rules: [...rules, ...extra] })) };
}

/** The rules kept in the URL as JSON; anything else reads as none. */
/**
 * Both filters at once. One side must be a plain AND list (or empty): groups go
 * one level deep, so two OR filters cannot nest; null then.
 */
export function bothWhere(a: Where, b: Where): Where | null {
  const plain = (w: Where) => whereEmpty(w) || (!w.any && !(w.groups ?? []).some((g) => g.rules.length));
  if (plain(b)) return andWhere(a, whereEmpty(b) ? [] : b.rules);
  if (plain(a)) return andWhere(b, whereEmpty(a) ? [] : a.rules);
  return null;
}

export function parseWhere(raw: string): Where {
  try {
    const v = JSON.parse(raw) as Where;
    if (!v || !Array.isArray(v.rules)) return { any: false, rules: [] };
    const groups = Array.isArray(v.groups) ? v.groups.filter((g) => g && Array.isArray(g.rules)).map((g) => ({ any: !!g.any, rules: g.rules })) : [];
    return { any: !!v.any, rules: v.rules, ...(groups.length ? { groups } : {}) };
  } catch {
    return { any: false, rules: [] };
  }
}

/**
 * Field filters of older links and saved views ({"field": value}), read as
 * "equals" conditions: the Conditions button now does what the Filters panel did.
 */
export function legacyRules(raw: string): Rule[] {
  try {
    const v: unknown = JSON.parse(raw);
    if (!v || typeof v !== "object" || Array.isArray(v)) return [];
    return Object.entries(v).filter(([, x]) => ["string", "number", "boolean"].includes(typeof x)).map(([field, value]) => ({ field, op: "eq" as const, value }));
  } catch {
    return [];
  }
}

/** The conditions of a saved view's query, old filters included. */
export function viewWhere(query: string): Where {
  const p = new URLSearchParams(query);
  return andWhere(parseWhere(p.get("where") ?? ""), legacyRules(p.get("filter") ?? ""));
}

/** How a field compares, from its type (a formula: from what it gives). */
export type Kind = "text" | "lookup" | "number" | "date" | "enum" | "strings" | "boolean" | "ref" | "refs" | "other";

export function kindOf(f: Pick<Field, "type" | "result">): Kind {
  switch (f.type) {
    case "string": case "text": case "email": case "url": case "created_by": case "updated_by": return "text";
    case "number": case "integer": case "autonumber": case "rollup": return "number";
    case "date": case "datetime": return "date";
    case "enum": return "enum";
    case "strings": return "strings";
    case "boolean": return "boolean";
    case "reference": return "ref";
    case "references": return "refs";
    case "lookup": return "lookup";
    case "formula": return f.result === "text" ? "text" : f.result === "boolean" ? "boolean" : f.result === "date" ? "date" : "number";
    default: return "other";
  }
}

/** Rule operators the editor offers; the last two are date shortcuts saved as "between". */
export type EditOp = RuleOp | "last_days" | "next_days";

/** The operators that make sense for a kind of field. */
export function opsFor(kind: Kind): EditOp[] {
  switch (kind) {
    case "text": case "lookup": return ["contains", "not_contains", "starts_with", "eq", "neq", "empty", "not_empty"];
    case "number": return ["eq", "neq", "gt", "gte", "lt", "lte", "between", "empty", "not_empty"];
    case "date": return ["eq", "lt", "gt", "lte", "gte", "between", "last_days", "next_days", "neq", "empty", "not_empty"];
    case "enum": case "ref": return ["eq", "neq", "in", "not_in", "empty", "not_empty"];
    case "strings": case "refs": return ["has", "has_any", "empty", "not_empty"];
    case "boolean": return ["eq", "neq", "empty"];
    default: return ["empty", "not_empty"];
  }
}

const needsValue = (op: EditOp) => op !== "empty" && op !== "not_empty";
const LIST_OPS: EditOp[] = ["in", "not_in", "has_any"];

/** A rule as the editor holds it: text while typed, "@n" for today plus n days, picked items in list. */
export type DraftRule = { field: string; op: EditOp; raw: string; raw2: string; list: string[] };

const todayOf = (v: unknown): number | undefined => {
  if (!v || typeof v !== "object" || Array.isArray(v)) return undefined;
  const n = (v as { today?: unknown }).today;
  return typeof n === "number" && Number.isInteger(n) ? n : undefined;
};

/** A saved rule back in the editor. */
export function toDraft(r: Rule, kind: Kind): DraftRule {
  const enc = (v: unknown): string => {
    const n = todayOf(v);
    if (n !== undefined) return `@${n}`;
    if (kind === "date" && typeof v === "string") return v.slice(0, 10);
    return v === undefined || v === null ? "" : String(v);
  };
  const d: DraftRule = { field: r.field, op: r.op, raw: "", raw2: "", list: [] };
  if (r.op === "between" && Array.isArray(r.value) && r.value.length === 2) {
    const [a, b] = r.value.map(todayOf);
    if (a !== undefined && b === 0 && a < 0) return { ...d, op: "last_days", raw: String(-a) };
    if (a === 0 && b !== undefined && b > 0) return { ...d, op: "next_days", raw: String(b) };
    return { ...d, raw: enc(r.value[0]), raw2: enc(r.value[1]) };
  }
  if (Array.isArray(r.value)) return kind === "strings" ? { ...d, raw: r.value.join(", ") } : { ...d, list: r.value.map(String) };
  return { ...d, raw: enc(r.value) };
}

/** The rule a draft gives, or null while its value is missing. */
export function fromDraft(d: DraftRule, kind: Kind): Rule | null {
  const days = Number.parseInt(d.raw, 10);
  if (d.op === "last_days" || d.op === "next_days") {
    if (!Number.isFinite(days) || days < 0) return null;
    return { field: d.field, op: "between", value: d.op === "last_days" ? [{ today: -days }, { today: 0 }] : [{ today: 0 }, { today: days }] };
  }
  const op = d.op as RuleOp;
  if (!needsValue(op)) return { field: d.field, op };
  const one = (raw: string): unknown => {
    const v = raw.trim();
    if (v === "") return undefined;
    if (kind === "number") return Number.isFinite(Number(v)) ? Number(v) : undefined;
    if (kind === "boolean") return v === "true";
    if (kind === "date" && /^@-?\d+$/.test(v)) return { today: Number(v.slice(1)) };
    return kind === "date" ? v : raw;
  };
  if (LIST_OPS.includes(op)) {
    const list = kind === "strings" ? d.raw.split(",").map((x) => x.trim()).filter(Boolean) : d.list;
    return list.length ? { field: d.field, op, value: list } : null;
  }
  if (op === "between") {
    const a = one(d.raw), b = one(d.raw2);
    return a === undefined || b === undefined ? null : { field: d.field, op, value: [a, b] };
  }
  const v = one(d.raw);
  return v === undefined ? null : { field: d.field, op, value: v };
}

/** A field the editor can compare: a stored one, a lookup or rollup, or the record's own date. */
type Entry = { key: string; name: string; kind: Kind; options: string[]; target: string; stored: boolean };

// Groups and free JSON have no single value to compare.
function entriesOf(rows: FieldRow[], t: Translate): Entry[] {
  const fields = rows.filter((r) => !r.advanced && r.field.type !== "group" && r.field.type !== "json").map((r): Entry => ({
    key: r.key, name: r.field.name, kind: kindOf(r.field),
    options: r.field.type === "enum" ? r.field.options.split(",").map((o) => o.trim()).filter(Boolean) : [],
    target: r.field.type === "reference" || r.field.type === "references" ? r.field.ref : "",
    stored: !VIRTUAL_TYPES.includes(r.field.type),
  }));
  return [...fields,
    { key: "$created_at", name: t("rules.created_at"), kind: "date", options: [], target: "", stored: false },
    { key: "$updated_at", name: t("rules.updated_at"), kind: "date", options: [], target: "", stored: false }];
}

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

/** What a rule says, as text with record names. */
function RuleText({ entry, rule, t }: { entry: Entry | undefined; rule: Rule; t: Translate }) {
  const kind = entry?.kind ?? "text";
  const d = toDraft(rule, kind);
  const name = entry?.name ?? rule.field;
  const op = opLabel(d.op, kind, t);
  if (!needsValue(d.op)) return <>{name} {op}</>;
  if (d.op === "last_days" || d.op === "next_days") return <>{name} {op} {d.raw} {t("rules.days")}</>;
  const one = (raw: string) => (kind === "date" ? dateText(raw, t) : kind === "boolean" ? t(raw === "true" ? "common.yes" : "common.no") : kind === "ref" || kind === "refs" ? <RefName target={entry?.target ?? ""} id={raw} /> : raw);
  if (d.op === "between") return <>{name} {op} {one(d.raw)} {t("rules.and")} {one(d.raw2)}</>;
  if (d.list.length) return <>{name} {op} {d.list.map((v, i) => <span key={v}>{i > 0 && ", "}{one(v)}</span>)}</>;
  return <>{name} {op} {one(d.raw)}</>;
}

function opLabel(op: EditOp, kind: Kind, t: Translate): string {
  return kind === "date" && ["eq", "neq", "lt", "gt", "lte", "gte"].includes(op) ? t(`rules.op_date.${op}` as MessageKey) : t(`rules.op.${op}` as MessageKey);
}

/** Plain text of a rule, for the chip's remove button. */
function ruleWords(entry: Entry | undefined, rule: Rule, t: Translate): string {
  const d = toDraft(rule, entry?.kind ?? "text");
  const value = !needsValue(d.op) ? "" : d.list.length ? d.list.join(", ") : d.op === "between" ? `${d.raw} – ${d.raw2}` : d.raw;
  return `${entry?.name ?? rule.field} ${opLabel(d.op, entry?.kind ?? "text", t)} ${value}`.trim();
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
function TextValue({ collectionId, entry, raw, label, onChange }: { collectionId?: string; entry: Entry; raw: string; label: string; onChange: (raw: string) => void }) {
  const { t } = useI18n();
  const listId = useId();
  const ask = !!collectionId && entry.stored && entry.key[0] !== "$";
  const top = useQuery({
    queryKey: ["records", collectionId, "values", entry.key],
    queryFn: ({ signal }) => records.stats({ collection_id: collectionId!, values: entry.key }, signal),
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
function OptionsValue({ options, picked, label, onChange }: { options: string[]; picked: string[]; label: string; onChange: (list: string[]) => void }) {
  return (
    <div role="group" aria-label={label} className="flex flex-wrap gap-1">
      {options.map((o) => {
        const on = picked.includes(o);
        return (
          <Button key={o} type="button" size="sm" variant={on ? "secondary" : "outline"} aria-pressed={on} className="h-7 px-2 text-xs"
            onClick={() => onChange(on ? picked.filter((x) => x !== o) : [...picked, o])}>{o}</Button>
        );
      })}
    </div>
  );
}

type DraftGroup = { any: boolean; rules: DraftRule[] };

/**
 * A "Conditions" button: rules that compare fields (greater than, contains,
 * empty…), joined by AND or OR, and groups of rules for mixes such as
 * (A and B) or (C and D). Applied rules show as removable chips. With the
 * collection's id, text values offer the field's commonest values.
 */
export function RecordRules({ fields: all, value, onChange, collectionId }: { fields: FieldRow[]; value: Where; onChange: (w: Where) => void; collectionId?: string }) {
  const { t } = useI18n();
  const entries = entriesOf(all, t);
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<{ any: boolean; groups: DraftGroup[] }>({ any: false, groups: [] });
  const entryOf = (key: string) => entries.find((f) => f.key === key);
  const kindOfKey = (key: string): Kind => entryOf(key)?.kind ?? "text";
  const joinWord = (any: boolean) => t(any ? "rules.or" : "rules.and");
  const groups = (value.groups ?? []).filter((g) => g.rules.length);
  const count = value.rules.length + groups.reduce((n, g) => n + g.rules.length, 0);
  const blank = (): DraftRule => ({ field: entries[0].key, op: opsFor(entries[0].kind)[0], raw: "", raw2: "", list: [] });
  const draftOf = (r: Rule) => toDraft(r, kindOfKey(r.field));

  const openWith = (o: boolean) => {
    if (o) {
      // Without groups the rules are one group; with them, a loose rule is a group of one.
      const next = groups.length
        ? { any: value.any, groups: [...value.rules.map((r) => ({ any: false, rules: [draftOf(r)] })), ...groups.map((g) => ({ any: g.any, rules: g.rules.map(draftOf) }))] }
        : { any: false, groups: [{ any: value.any, rules: value.rules.map(draftOf) }] };
      if (next.groups.length === 1 && next.groups[0].rules.length === 0) next.groups[0].rules.push(blank());
      setDraft(next);
    }
    setOpen(o);
  };
  const apply = () => {
    const done = draft.groups.map((g) => ({ any: g.any, rules: g.rules.map((r) => fromDraft(r, kindOfKey(r.field))).filter((r): r is Rule => !!r) })).filter((g) => g.rules.length);
    if (done.length <= 1) onChange({ any: done[0]?.any ?? false, rules: done[0]?.rules ?? [] });
    else onChange({ any: draft.any, rules: done.filter((g) => g.rules.length === 1).map((g) => g.rules[0]), groups: done.filter((g) => g.rules.length > 1) });
    setOpen(false);
  };
  const setGroup = (gi: number, patch: (g: DraftGroup) => DraftGroup) => setDraft((d) => ({ ...d, groups: d.groups.map((g, j) => (j === gi ? patch(g) : g)) }));
  const setRule = (gi: number, i: number, patch: Partial<DraftRule>) =>
    setGroup(gi, (g) => ({ ...g, rules: g.rules.map((r, j) => {
      if (j !== i) return r;
      const next = { ...r, ...patch };
      // A new field starts empty, keeping the operator only when it still fits.
      if (patch.field && patch.field !== r.field) {
        const ops = opsFor(kindOfKey(patch.field));
        Object.assign(next, { raw: "", raw2: "", list: [], op: ops.includes(next.op) ? next.op : ops[0] });
      }
      // A day count and a date do not share a value.
      if (patch.op && (["last_days", "next_days"].includes(patch.op) !== ["last_days", "next_days"].includes(r.op))) Object.assign(next, { raw: "", raw2: "" });
      return next;
    }) }));
  const total = draft.groups.reduce((n, g) => n + g.rules.length, 0);
  const grouped = draft.groups.length > 1;

  if (entries.length === 0) return null;
  const fieldItems = Object.fromEntries(entries.map((f) => [f.key, f.name]));
  const joinPicker = (any: boolean, label: string, onPick: (any: boolean) => void) => (
    <div role="radiogroup" aria-label={label} className="flex items-center gap-2 text-sm">
      <span className="text-muted-foreground">{label}</span>
      {([false, true] as const).map((a) => (
        <Button key={String(a)} type="button" size="sm" role="radio" aria-checked={any === a} variant={any === a ? "secondary" : "outline"} onClick={() => onPick(a)}>
          {t(a ? "rules.any" : "rules.all")}
        </Button>
      ))}
    </div>
  );
  const valueOf = (gi: number, r: DraftRule, i: number, n: number) => {
    const entry = entryOf(r.field);
    if (!entry || !needsValue(r.op)) return <span />;
    const label = String(n);
    const set = (patch: Partial<DraftRule>) => setRule(gi, i, patch);
    const kind = entry.kind;
    if (r.op === "last_days" || r.op === "next_days")
      return <div className="flex items-center gap-1.5"><Input type="number" min={0} inputMode="numeric" aria-label={`${t("rules.value")} ${label}`} value={r.raw} onChange={(e) => set({ raw: e.target.value })} /><span className="text-sm text-muted-foreground">{t("rules.days")}</span></div>;
    if (kind === "ref" || kind === "refs") {
      const many = LIST_OPS.includes(r.op);
      return <RecordsValue entry={entry} many={many} ids={many ? r.list : r.raw ? [r.raw] : []} label={label} onChange={(ids) => set(many ? { list: ids } : { raw: ids[0] ?? "" })} />;
    }
    if (kind === "enum" && LIST_OPS.includes(r.op)) return <OptionsValue options={entry.options} picked={r.list} label={`${t("rules.value")} ${label}`} onChange={(list) => set({ list })} />;
    if (kind === "boolean" || kind === "enum") {
      const items = kind === "boolean" ? { true: t("common.yes"), false: t("common.no") } : Object.fromEntries(entry.options.map((o) => [o, o]));
      return (
        <Select value={r.raw || null} items={items} onValueChange={(v) => set({ raw: String(v) })}>
          <SelectTrigger className="w-full" aria-label={`${t("rules.value")} ${label}`}><SelectValue placeholder={t("rules.value")} /></SelectTrigger>
          <SelectContent>{Object.entries(items).map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}</SelectContent>
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
    return <TextValue collectionId={collectionId} entry={entry} raw={r.raw} label={label} onChange={(raw) => set({ raw })} />;
  };
  const ruleRow = (gi: number, r: DraftRule, i: number, n: number) => {
    const kind = kindOfKey(r.field);
    const ops = Object.fromEntries(opsFor(kind).map((o) => [o, opLabel(o, kind, t)]));
    return (
      <li key={i} className="grid grid-cols-[minmax(0,1fr)_10rem_minmax(0,1.3fr)_auto] items-start gap-1.5 max-sm:grid-cols-2">
        <Select value={r.field} items={fieldItems} onValueChange={(v) => setRule(gi, i, { field: String(v) })}>
          <SelectTrigger className="w-full" aria-label={`${t("rules.field")} ${n}`}><SelectValue /></SelectTrigger>
          <SelectContent>{entries.map((f) => <SelectItem key={f.key} value={f.key}>{f.name}</SelectItem>)}</SelectContent>
        </Select>
        <Select value={r.op} items={ops} onValueChange={(v) => setRule(gi, i, { op: v as EditOp })}>
          <SelectTrigger className="w-full" aria-label={`${t("rules.op_label")} ${n}`}><SelectValue /></SelectTrigger>
          <SelectContent>{Object.entries(ops).map(([o, label]) => <SelectItem key={o} value={o}>{label}</SelectItem>)}</SelectContent>
        </Select>
        {valueOf(gi, r, i, n)}
        <IconAction label={t("rules.remove")} target={entryOf(r.field)?.name ?? r.field} onClick={() => setGroup(gi, (g) => ({ ...g, rules: g.rules.filter((_, j) => j !== i) }))}><Trash2 className="size-4" /></IconAction>
      </li>
    );
  };
  const chipOf = (rules: Rule[], any: boolean) => rules.map((r, i) => <span key={i}>{i > 0 && ` ${joinWord(any)} `}<RuleText entry={entryOf(r.field)} rule={r} t={t} /></span>);
  const wordsOf = (rules: Rule[], any: boolean) => rules.map((r) => ruleWords(entryOf(r.field), r, t)).join(` ${joinWord(any)} `);
  // Rules are numbered across the groups, so each control keeps a distinct name.
  let n = 0;
  return (
    <>
      <Popover.Root open={open} onOpenChange={openWith}>
        <Popover.Trigger render={<Button variant="outline" />}>
          <SlidersHorizontal className="size-4" />{t("rules.title")}{count > 0 && <span className="rounded-full bg-primary/15 px-1.5 text-xs text-primary">{count}</span>}
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Positioner sideOffset={4} align="start" collisionPadding={16} className="z-50">
            <Popover.Popup aria-label={t("rules.title")} className="w-[38rem] max-w-[calc(100vw-32px)] space-y-3 rounded-lg bg-popover p-3 text-popover-foreground shadow-md ring-1 ring-foreground/10">
              {/* React carries submit up through the portal: stop it before an outer form (a widget, an automation) takes it. */}
              <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); e.stopPropagation(); apply(); }}>
                {grouped && joinPicker(draft.any, t("rules.join_groups"), (any) => setDraft((d) => ({ ...d, any })))}
                <div className="max-h-96 space-y-2 overflow-y-auto">
                  {draft.groups.map((g, gi) => (
                    <div key={gi}>
                      {grouped && gi > 0 && <p className="py-1 text-center text-xs font-medium uppercase text-muted-foreground">{joinWord(draft.any)}</p>}
                      <section aria-label={grouped ? t("rules.group").replace("{n}", String(gi + 1)) : undefined} className={grouped ? "space-y-2 rounded-lg border bg-muted/30 p-2" : "space-y-2"}>
                        {(grouped || g.rules.length > 1) && (
                          <div className="flex items-center justify-between gap-2">
                            {g.rules.length > 1 ? joinPicker(g.any, t("rules.join"), (any) => setGroup(gi, (x) => ({ ...x, any }))) : <span className="text-xs font-medium text-muted-foreground">{t("rules.group").replace("{n}", String(gi + 1))}</span>}
                            {grouped && <IconAction label={t("rules.remove_group")} target={String(gi + 1)} onClick={() => setDraft((d) => ({ ...d, groups: d.groups.filter((_, j) => j !== gi) }))}><X className="size-4" /></IconAction>}
                          </div>
                        )}
                        <ul className="space-y-2">{g.rules.map((r, i) => ruleRow(gi, r, i, ++n))}</ul>
                        <Button type="button" variant="ghost" size="sm" disabled={total >= MAX_RULES} onClick={() => setGroup(gi, (x) => ({ ...x, rules: [...x.rules, blank()] }))}>
                          <Plus className="size-4" />{t("rules.add")}
                        </Button>
                      </section>
                    </div>
                  ))}
                </div>
                <Button type="button" variant="outline" size="sm" disabled={total >= MAX_RULES || draft.groups.length >= MAX_GROUPS}
                  onClick={() => setDraft((d) => ({ any: d.groups.length === 1 ? true : d.any, groups: [...d.groups, { any: false, rules: [blank()] }] }))}>
                  <Plus className="size-4" />{t("rules.add_group")}
                </Button>
                <div className="flex justify-between gap-2 border-t pt-3">
                  <Button type="button" variant="ghost" onClick={() => { onChange({ any: false, rules: [] }); setOpen(false); }}>{t("filters.clear")}</Button>
                  <Button type="submit">{t("filters.apply")}</Button>
                </div>
              </form>
            </Popover.Popup>
          </Popover.Positioner>
        </Popover.Portal>
      </Popover.Root>
      {count > 0 && (
        <ul className="flex basis-full flex-wrap items-center gap-1.5" aria-label={t("rules.active")}>
          {value.rules.length + groups.length > 1 && <li className="text-xs text-muted-foreground">{t(value.any ? "rules.any" : "rules.all")}:</li>}
          {value.rules.map((r, i) => (
            <li key={`r${i}`} className="flex items-center gap-1 rounded-full border bg-muted/40 py-0.5 pl-2.5 pr-0.5 text-xs">
              <span>{chipOf([r], false)}</span>
              <button type="button" className="grid size-6 place-items-center rounded-full hover:bg-accent" aria-label={`${t("rules.remove")}: ${wordsOf([r], false)}`}
                onClick={() => onChange({ ...value, rules: value.rules.filter((_, j) => j !== i) })}><X className="size-3" /></button>
            </li>
          ))}
          {groups.map((g, i) => (
            <li key={`g${i}`} className="flex items-center gap-1 rounded-full border bg-muted/40 py-0.5 pl-2.5 pr-0.5 text-xs">
              <span>{g.rules.length > 1 && "("}{chipOf(g.rules, g.any)}{g.rules.length > 1 && ")"}</span>
              <button type="button" className="grid size-6 place-items-center rounded-full hover:bg-accent" aria-label={`${t("rules.remove_group")}: ${wordsOf(g.rules, g.any)}`}
                onClick={() => onChange({ ...value, groups: groups.filter((_, j) => j !== i) })}><X className="size-3" /></button>
            </li>
          ))}
        </ul>
      )}
    </>
  );
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
