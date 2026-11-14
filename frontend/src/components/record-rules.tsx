"use client";

import { useState } from "react";
import { Popover } from "@base-ui/react/popover";
import { Plus, SlidersHorizontal, Trash2, X } from "lucide-react";
import { IconAction } from "@/components/bits";
import { type FieldRow, type FieldType } from "@/components/field-builder";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";

export type RuleOp = "eq" | "neq" | "contains" | "gt" | "gte" | "lt" | "lte" | "empty" | "not_empty" | "in" | "has";
export interface Rule { field: string; op: RuleOp; value?: unknown }
/** Rules joined by AND (any: false) or OR; groups bracket rules of their own, one level deep. */
export interface Where { any: boolean; rules: Rule[]; groups?: { any: boolean; rules: Rule[] }[] }

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

const TEXT: FieldType[] = ["string", "text", "email", "url", "reference"];
const ORDERED: FieldType[] = ["number", "integer", "date", "datetime"];

/** The operators that make sense for a field's type. */
export function opsFor(type: FieldType): RuleOp[] {
  if (TEXT.includes(type)) return ["contains", "eq", "neq", "empty", "not_empty"];
  if (ORDERED.includes(type)) return ["eq", "neq", "gt", "gte", "lt", "lte", "empty", "not_empty"];
  if (type === "enum") return ["eq", "neq", "in", "empty", "not_empty"];
  if (type === "strings") return ["has", "empty", "not_empty"];
  if (type === "boolean") return ["eq", "neq", "empty"];
  return ["empty", "not_empty"];
}

const needsValue = (op: RuleOp) => op !== "empty" && op !== "not_empty";

/** The rule's typed value from what was typed: numbers for numeric fields, lists for "in". */
export function ruleValue(type: FieldType, op: RuleOp, raw: string): unknown {
  if (op === "in") return raw.split(",").map((s) => s.trim()).filter(Boolean);
  if (type === "number" || type === "integer") return raw.trim() === "" || !Number.isFinite(Number(raw)) ? undefined : Number(raw);
  if (type === "boolean") return raw === "true";
  if (type === "datetime" && raw && !Number.isNaN(Date.parse(raw))) return new Date(raw).toISOString();
  return raw;
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

const filterable = (rows: FieldRow[]) => rows.filter((r) => !r.advanced && r.field.type !== "group" && r.field.type !== "json");

type DraftRule = { field: string; op: RuleOp; raw: string };
type DraftGroup = { any: boolean; rules: DraftRule[] };

const toRaw = (r: Rule): DraftRule => ({ field: r.field, op: r.op, raw: Array.isArray(r.value) ? r.value.join(", ") : r.value === undefined ? "" : String(r.value) });

/**
 * A "Conditions" button: rules that compare fields (greater than, contains,
 * empty…), joined by AND or OR, and groups of rules for mixes such as
 * (A and B) or (C and D). Applied rules show as removable chips.
 */
export function RecordRules({ fields: all, value, onChange }: { fields: FieldRow[]; value: Where; onChange: (w: Where) => void }) {
  const { t } = useI18n();
  const fields = filterable(all);
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<{ any: boolean; groups: DraftGroup[] }>({ any: false, groups: [] });
  const typeOf = (key: string) => fields.find((f) => f.key === key)?.field.type ?? "string";
  const nameOf = (key: string) => fields.find((f) => f.key === key)?.field.name ?? key;
  const opName = (op: RuleOp) => t(`rules.op.${op}` as MessageKey);
  const show = (r: Rule) => {
    if (!needsValue(r.op)) return `${nameOf(r.field)} ${opName(r.op)}`;
    const v = Array.isArray(r.value) ? r.value.join(", ") : typeof r.value === "boolean" ? t(r.value ? "common.yes" : "common.no") : String(r.value);
    return `${nameOf(r.field)} ${opName(r.op)} ${v}`;
  };
  const joinWord = (any: boolean) => t(any ? "rules.or" : "rules.and");
  const showGroup = (g: { any: boolean; rules: Rule[] }) => (g.rules.length > 1 ? `(${g.rules.map(show).join(` ${joinWord(g.any)} `)})` : show(g.rules[0]));
  const groups = (value.groups ?? []).filter((g) => g.rules.length);
  const count = value.rules.length + groups.reduce((n, g) => n + g.rules.length, 0);
  const blank = (): DraftRule => ({ field: fields[0].key, op: opsFor(fields[0].field.type)[0], raw: "" });

  const openWith = (o: boolean) => {
    if (o) {
      // Without groups the rules are one group; with them, a loose rule is a group of one.
      const next = groups.length
        ? { any: value.any, groups: [...value.rules.map((r) => ({ any: false, rules: [toRaw(r)] })), ...groups.map((g) => ({ any: g.any, rules: g.rules.map(toRaw) }))] }
        : { any: false, groups: [{ any: value.any, rules: value.rules.map(toRaw) }] };
      if (next.groups.length === 1 && next.groups[0].rules.length === 0 && fields[0]) next.groups[0].rules.push(blank());
      setDraft(next);
    }
    setOpen(o);
  };
  const typed = (r: DraftRule): Rule | null => {
    if (!needsValue(r.op)) return { field: r.field, op: r.op };
    const v = ruleValue(typeOf(r.field), r.op, r.raw);
    return v === undefined || v === "" || (Array.isArray(v) && !v.length) ? null : { field: r.field, op: r.op, value: v };
  };
  const apply = () => {
    const done = draft.groups.map((g) => ({ any: g.any, rules: g.rules.map(typed).filter((r): r is Rule => !!r) })).filter((g) => g.rules.length);
    if (done.length <= 1) onChange({ any: done[0]?.any ?? false, rules: done[0]?.rules ?? [] });
    else onChange({ any: draft.any, rules: done.filter((g) => g.rules.length === 1).map((g) => g.rules[0]), groups: done.filter((g) => g.rules.length > 1) });
    setOpen(false);
  };
  const setGroup = (gi: number, patch: (g: DraftGroup) => DraftGroup) => setDraft((d) => ({ ...d, groups: d.groups.map((g, j) => (j === gi ? patch(g) : g)) }));
  const setRule = (gi: number, i: number, patch: Partial<DraftRule>) =>
    setGroup(gi, (g) => ({ ...g, rules: g.rules.map((r, j) => {
      if (j !== i) return r;
      const next = { ...r, ...patch };
      // A new field keeps the operator only when it still fits.
      if (patch.field && !opsFor(typeOf(patch.field)).includes(next.op)) next.op = opsFor(typeOf(patch.field))[0];
      return next;
    }) }));
  const total = draft.groups.reduce((n, g) => n + g.rules.length, 0);
  const grouped = draft.groups.length > 1;

  if (fields.length === 0) return null;
  const fieldItems = Object.fromEntries(fields.map((f) => [f.key, f.field.name]));
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
  const ruleRow = (gi: number, r: DraftRule, i: number, n: number) => {
    const type = typeOf(r.field);
    const ops = Object.fromEntries(opsFor(type).map((o) => [o, opName(o)]));
    const enumOptions = type === "enum" ? (fields.find((f) => f.key === r.field)?.field.options ?? "").split(",").map((o) => o.trim()).filter(Boolean) : [];
    return (
      <li key={i} className="grid grid-cols-[minmax(0,1fr)_9rem_minmax(0,1fr)_auto] items-center gap-1.5 max-sm:grid-cols-2">
        <Select value={r.field} items={fieldItems} onValueChange={(v) => setRule(gi, i, { field: String(v) })}>
          <SelectTrigger className="w-full" aria-label={`${t("rules.field")} ${n}`}><SelectValue /></SelectTrigger>
          <SelectContent>{fields.map((f) => <SelectItem key={f.key} value={f.key}>{f.field.name}</SelectItem>)}</SelectContent>
        </Select>
        <Select value={r.op} items={ops} onValueChange={(v) => setRule(gi, i, { op: v as RuleOp })}>
          <SelectTrigger className="w-full" aria-label={`${t("rules.op_label")} ${n}`}><SelectValue /></SelectTrigger>
          <SelectContent>{Object.entries(ops).map(([o, label]) => <SelectItem key={o} value={o}>{label}</SelectItem>)}</SelectContent>
        </Select>
        {!needsValue(r.op) ? <span /> : type === "boolean" || (type === "enum" && r.op !== "in") ? (
          <Select value={r.raw || null} items={type === "boolean" ? { true: t("common.yes"), false: t("common.no") } : Object.fromEntries(enumOptions.map((o) => [o, o]))} onValueChange={(v) => setRule(gi, i, { raw: String(v) })}>
            <SelectTrigger className="w-full" aria-label={`${t("rules.value")} ${n}`}><SelectValue placeholder={t("rules.value")} /></SelectTrigger>
            <SelectContent>
              {(type === "boolean" ? [["true", t("common.yes")], ["false", t("common.no")]] : enumOptions.map((o) => [o, o])).map(([v, label]) => <SelectItem key={v} value={v}>{label}</SelectItem>)}
            </SelectContent>
          </Select>
        ) : (
          <Input aria-label={`${t("rules.value")} ${n}`} value={r.raw} placeholder={r.op === "in" ? t("rules.in_hint") : t("rules.value")}
            type={r.op === "in" ? "text" : type === "number" || type === "integer" ? "number" : type === "date" ? "date" : type === "datetime" ? "datetime-local" : "text"}
            onChange={(e) => setRule(gi, i, { raw: e.target.value })} />
        )}
        <IconAction label={t("rules.remove")} target={nameOf(r.field)} onClick={() => setGroup(gi, (g) => ({ ...g, rules: g.rules.filter((_, j) => j !== i) }))}><Trash2 className="size-4" /></IconAction>
      </li>
    );
  };
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
              <span>{show(r)}</span>
              <button type="button" className="grid size-6 place-items-center rounded-full hover:bg-accent" aria-label={`${t("rules.remove")}: ${show(r)}`}
                onClick={() => onChange({ ...value, rules: value.rules.filter((_, j) => j !== i) })}><X className="size-3" /></button>
            </li>
          ))}
          {groups.map((g, i) => (
            <li key={`g${i}`} className="flex items-center gap-1 rounded-full border bg-muted/40 py-0.5 pl-2.5 pr-0.5 text-xs">
              <span>{showGroup(g)}</span>
              <button type="button" className="grid size-6 place-items-center rounded-full hover:bg-accent" aria-label={`${t("rules.remove_group")}: ${showGroup(g)}`}
                onClick={() => onChange({ ...value, groups: groups.filter((_, j) => j !== i) })}><X className="size-3" /></button>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
