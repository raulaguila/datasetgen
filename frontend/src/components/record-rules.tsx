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
export interface Where { any: boolean; rules: Rule[] }

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
export function parseWhere(raw: string): Where {
  try {
    const v = JSON.parse(raw) as Where;
    return v && Array.isArray(v.rules) ? { any: !!v.any, rules: v.rules } : { any: false, rules: [] };
  } catch {
    return { any: false, rules: [] };
  }
}

const filterable = (rows: FieldRow[]) => rows.filter((r) => !r.advanced && r.field.type !== "group" && r.field.type !== "json");

/**
 * A "Conditions" button: rules that compare fields (greater than, contains,
 * empty…), joined by AND or OR, shown as removable chips when applied.
 */
export function RecordRules({ fields: all, value, onChange }: { fields: FieldRow[]; value: Where; onChange: (w: Where) => void }) {
  const { t } = useI18n();
  const fields = filterable(all);
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<{ any: boolean; rules: { field: string; op: RuleOp; raw: string }[] }>({ any: false, rules: [] });
  const typeOf = (key: string) => fields.find((f) => f.key === key)?.field.type ?? "string";
  const nameOf = (key: string) => fields.find((f) => f.key === key)?.field.name ?? key;
  const opName = (op: RuleOp) => t(`rules.op.${op}` as MessageKey);
  const show = (r: Rule) => {
    if (!needsValue(r.op)) return `${nameOf(r.field)} ${opName(r.op)}`;
    const v = Array.isArray(r.value) ? r.value.join(", ") : typeof r.value === "boolean" ? t(r.value ? "common.yes" : "common.no") : String(r.value);
    return `${nameOf(r.field)} ${opName(r.op)} ${v}`;
  };

  const openWith = (o: boolean) => {
    if (o) setDraft({ any: value.any, rules: value.rules.map((r) => ({ field: r.field, op: r.op, raw: Array.isArray(r.value) ? r.value.join(", ") : r.value === undefined ? "" : String(r.value) })) });
    if (o && value.rules.length === 0 && fields[0]) setDraft({ any: false, rules: [{ field: fields[0].key, op: opsFor(fields[0].field.type)[0], raw: "" }] });
    setOpen(o);
  };
  const apply = () => {
    const rules: Rule[] = [];
    for (const r of draft.rules) {
      if (!needsValue(r.op)) { rules.push({ field: r.field, op: r.op }); continue; }
      const v = ruleValue(typeOf(r.field), r.op, r.raw);
      if (v === undefined || v === "" || (Array.isArray(v) && !v.length)) continue;
      rules.push({ field: r.field, op: r.op, value: v });
    }
    onChange({ any: draft.any, rules });
    setOpen(false);
  };
  const setRule = (i: number, patch: Partial<{ field: string; op: RuleOp; raw: string }>) =>
    setDraft((d) => ({ ...d, rules: d.rules.map((r, j) => {
      if (j !== i) return r;
      const next = { ...r, ...patch };
      // A new field keeps the operator only when it still fits.
      if (patch.field && !opsFor(typeOf(patch.field)).includes(next.op)) next.op = opsFor(typeOf(patch.field))[0];
      return next;
    }) }));

  if (fields.length === 0) return null;
  const fieldItems = Object.fromEntries(fields.map((f) => [f.key, f.field.name]));
  return (
    <>
      <Popover.Root open={open} onOpenChange={openWith}>
        <Popover.Trigger render={<Button variant="outline" />}>
          <SlidersHorizontal className="size-4" />{t("rules.title")}{value.rules.length > 0 && <span className="rounded-full bg-primary/15 px-1.5 text-xs text-primary">{value.rules.length}</span>}
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Positioner sideOffset={4} align="start" collisionPadding={16} className="z-50">
            <Popover.Popup aria-label={t("rules.title")} className="w-[36rem] max-w-[calc(100vw-32px)] space-y-3 rounded-lg bg-popover p-3 text-popover-foreground shadow-md ring-1 ring-foreground/10">
              <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); apply(); }}>
                {draft.rules.length > 1 && (
                  <div role="radiogroup" aria-label={t("rules.join")} className="flex items-center gap-2 text-sm">
                    <span className="text-muted-foreground">{t("rules.join")}</span>
                    {([false, true] as const).map((any) => (
                      <Button key={String(any)} type="button" size="sm" role="radio" aria-checked={draft.any === any} variant={draft.any === any ? "secondary" : "outline"} onClick={() => setDraft((d) => ({ ...d, any }))}>
                        {t(any ? "rules.any" : "rules.all")}
                      </Button>
                    ))}
                  </div>
                )}
                <ul className="max-h-80 space-y-2 overflow-y-auto">
                  {draft.rules.map((r, i) => {
                    const type = typeOf(r.field);
                    const ops = Object.fromEntries(opsFor(type).map((o) => [o, opName(o)]));
                    const enumOptions = type === "enum" ? (fields.find((f) => f.key === r.field)?.field.options ?? "").split(",").map((o) => o.trim()).filter(Boolean) : [];
                    return (
                      <li key={i} className="grid grid-cols-[minmax(0,1fr)_9rem_minmax(0,1fr)_auto] items-center gap-1.5 max-sm:grid-cols-2">
                        <Select value={r.field} items={fieldItems} onValueChange={(v) => setRule(i, { field: String(v) })}>
                          <SelectTrigger className="w-full" aria-label={`${t("rules.field")} ${i + 1}`}><SelectValue /></SelectTrigger>
                          <SelectContent>{fields.map((f) => <SelectItem key={f.key} value={f.key}>{f.field.name}</SelectItem>)}</SelectContent>
                        </Select>
                        <Select value={r.op} items={ops} onValueChange={(v) => setRule(i, { op: v as RuleOp })}>
                          <SelectTrigger className="w-full" aria-label={`${t("rules.op_label")} ${i + 1}`}><SelectValue /></SelectTrigger>
                          <SelectContent>{Object.entries(ops).map(([o, label]) => <SelectItem key={o} value={o}>{label}</SelectItem>)}</SelectContent>
                        </Select>
                        {!needsValue(r.op) ? <span /> : type === "boolean" || (type === "enum" && r.op !== "in") ? (
                          <Select value={r.raw || null} items={type === "boolean" ? { true: t("common.yes"), false: t("common.no") } : Object.fromEntries(enumOptions.map((o) => [o, o]))} onValueChange={(v) => setRule(i, { raw: String(v) })}>
                            <SelectTrigger className="w-full" aria-label={`${t("rules.value")} ${i + 1}`}><SelectValue placeholder={t("rules.value")} /></SelectTrigger>
                            <SelectContent>
                              {(type === "boolean" ? [["true", t("common.yes")], ["false", t("common.no")]] : enumOptions.map((o) => [o, o])).map(([v, label]) => <SelectItem key={v} value={v}>{label}</SelectItem>)}
                            </SelectContent>
                          </Select>
                        ) : (
                          <Input aria-label={`${t("rules.value")} ${i + 1}`} value={r.raw} placeholder={r.op === "in" ? t("rules.in_hint") : t("rules.value")}
                            type={r.op === "in" ? "text" : type === "number" || type === "integer" ? "number" : type === "date" ? "date" : type === "datetime" ? "datetime-local" : "text"}
                            onChange={(e) => setRule(i, { raw: e.target.value })} />
                        )}
                        <IconAction label={t("rules.remove")} target={nameOf(r.field)} onClick={() => setDraft((d) => ({ ...d, rules: d.rules.filter((_, j) => j !== i) }))}><Trash2 className="size-4" /></IconAction>
                      </li>
                    );
                  })}
                </ul>
                <Button type="button" variant="outline" size="sm" disabled={draft.rules.length >= 20}
                  onClick={() => setDraft((d) => ({ ...d, rules: [...d.rules, { field: fields[0].key, op: opsFor(fields[0].field.type)[0], raw: "" }] }))}>
                  <Plus className="size-4" />{t("rules.add")}
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
      {value.rules.length > 0 && (
        <ul className="flex basis-full flex-wrap items-center gap-1.5" aria-label={t("rules.active")}>
          {value.rules.length > 1 && <li className="text-xs text-muted-foreground">{t(value.any ? "rules.any" : "rules.all")}:</li>}
          {value.rules.map((r, i) => (
            <li key={i} className="flex items-center gap-1 rounded-full border bg-muted/40 py-0.5 pl-2.5 pr-0.5 text-xs">
              <span>{show(r)}</span>
              <button type="button" className="grid size-6 place-items-center rounded-full hover:bg-accent" aria-label={`${t("rules.remove")}: ${show(r)}`}
                onClick={() => onChange({ ...value, rules: value.rules.filter((_, j) => j !== i) })}><X className="size-3" /></button>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
