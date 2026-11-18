"use client";

import { useId, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Braces, ChevronDown, ChevronRight, Copy, Eye, GripVertical, ListTodo, MapPin, MoreHorizontal, Plus, RotateCcw, Star, Trash2, Users, Gauge, Search, type LucideIcon } from "lucide-react";
import { IconAction } from "@/components/bits";
import {
  AUTO_TYPES, FIELD_TYPES, FORMULA_RESULTS, ITEM_TYPES, OPTION_COLORS, PATTERN_TYPES, ROLLUP_FNS, SHOW_OPS, SUBFIELD_TYPES, UNIQUE_TYPES, VIRTUAL_TYPES,
  addField, addProp, blankField, typeOf, typesKeepingValues, isNumberField, canonical, duplicateField, fieldProblem, fieldsOf, isObjectType, keyOf, limitKeys, moveField, moveFieldTo, optionsOf, propOf,
  propsOf, removeField, requiredOf, restoreField, setProp, updateField,
  type Field, type FieldRow, type FieldType, type Schema, type ShowWhen,
} from "@/components/field-builder";
import { optionColor } from "@/components/option-chip";
import { TYPE_ICONS } from "@/components/type-icons";
import { SchemaForm } from "@/components/schema-form";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { collections } from "@/lib/api/endpoints";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";

// The Fields page: a list of fields, each edited in a side panel with a live
// preview of itself, a type gallery grouped by kind, and ready-made fields.


/** The gallery's sections, in the order people look for them. */
const TYPE_GROUPS: [string, FieldType[]][] = [
  ["text", ["string", "text", "email", "url"]],
  ["number", ["number", "integer"]],
  ["choice", ["enum", "boolean", "strings"]],
  ["date", ["date", "datetime"]],
  ["relation", ["reference", "references", "lookup", "rollup"]],
  ["file", ["file", "files"]],
  ["auto", ["autonumber", "formula", "created_by", "updated_by"]],
  ["structure", ["group", "objects", "json"]],
];

type Preset = { id: string; icon: LucideIcon; make: (t: (k: MessageKey) => string, pt: boolean) => [string, Schema] };
const field = (patch: Partial<Field>): Field => ({ ...blankField, ...patch, key: patch.key ?? keyOf(patch.name ?? "") });
const group = (name: string, subs: Partial<Field>[]): [string, Schema] => {
  const inner = subs.reduce<Schema>((s, f) => addField(s, field(f)), { type: "object", properties: {} });
  return [keyOf(name), { title: name, ...inner }];
};
/** Ready-made fields: a status and a priority with colors, an address, a contact, a rating. */
const PRESETS: Preset[] = [
  { id: "status", icon: ListTodo, make: (t) => { const o = [t("presets.status.todo"), t("presets.status.doing"), t("presets.status.done")]; const name = t("presets.status"); return [keyOf(name), propOf(field({ name, type: "enum", options: o.join(", "), default: o[0], colors: { [o[0]]: "gray", [o[1]]: "blue", [o[2]]: "green" } }))]; } },
  { id: "priority", icon: Gauge, make: (t) => { const o = [t("presets.priority.low"), t("presets.priority.mid"), t("presets.priority.high")]; const name = t("presets.priority"); return [keyOf(name), propOf(field({ name, type: "enum", options: o.join(", "), colors: { [o[0]]: "gray", [o[1]]: "amber", [o[2]]: "red" } }))]; } },
  { id: "address", icon: MapPin, make: (t, pt) => group(t("presets.address"), [
    { name: t("presets.address.street") }, { name: t("presets.address.number"), max: "20" }, { name: t("presets.address.district") },
    { name: t("presets.address.city") }, { name: t("presets.address.state"), max: "40" },
    // A Brazilian CEP has a known shape; other postal codes vary too much to check.
    { name: t("presets.address.zip"), placeholder: pt ? "00000-000" : "", pattern: pt ? "^\\d{5}-?\\d{3}$" : "" },
  ]) },
  { id: "contact", icon: Users, make: (t) => group(t("presets.contact"), [{ name: t("presets.contact.name") }, { name: t("presets.contact.email"), type: "email" }, { name: t("presets.contact.phone"), max: "30" }]) },
  { id: "rating", icon: Star, make: (t) => { const name = t("presets.rating"); return [keyOf(name), propOf(field({ name, type: "integer", min: "1", max: "5", description: t("presets.rating_hint") }))]; } },
];

/** The type gallery: every type a field may take, grouped and explained; at the top level, ready-made fields too. */
function TypeGallery({ offered, presets, onPick, onPreset }: { offered: readonly FieldType[]; presets: boolean; onPick: (t: FieldType) => void; onPreset: (p: Preset) => void }) {
  const { t } = useI18n();
  const [q, setQ] = useState("");
  const term = q.trim().toLocaleLowerCase();
  const match = (label: string, hint: string) => !term || label.toLocaleLowerCase().includes(term) || hint.toLocaleLowerCase().includes(term);
  const sections = TYPE_GROUPS.map(([id, types]) => [id, types.filter((ty) => offered.includes(ty) && match(t(`collections.type.${ty}`), t(`collections.type_hint.${ty}`)))] as const).filter(([, types]) => types.length);
  const shownPresets = presets ? PRESETS.filter((p) => match(t(`presets.${p.id}` as MessageKey), t(`presets.${p.id}_desc` as MessageKey))) : [];
  return (
    <div className="space-y-4">
      <div className="relative">
        <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
        <Input type="search" autoFocus aria-label={t("fields.type_search")} placeholder={t("fields.type_search")} className="pl-8" value={q} onChange={(e) => setQ(e.target.value)} />
      </div>
      {shownPresets.length > 0 && (
        <section aria-labelledby="gallery-presets" className="space-y-1.5">
          <h3 id="gallery-presets" className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t("fields.presets")}</h3>
          <div className="grid gap-1.5 sm:grid-cols-2">
            {shownPresets.map((p) => <GalleryItem key={p.id} icon={p.icon} label={t(`presets.${p.id}` as MessageKey)} hint={t(`presets.${p.id}_desc` as MessageKey)} onClick={() => onPreset(p)} />)}
          </div>
        </section>
      )}
      {sections.map(([id, types]) => (
        <section key={id} aria-labelledby={`gallery-${id}`} className="space-y-1.5">
          <h3 id={`gallery-${id}`} className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{t(`fields.kind.${id}` as MessageKey)}</h3>
          <div className="grid gap-1.5 sm:grid-cols-2">
            {types.map((ty) => <GalleryItem key={ty} icon={TYPE_ICONS[ty]} label={t(`collections.type.${ty}`)} hint={t(`collections.type_hint.${ty}`)} onClick={() => onPick(ty)} />)}
          </div>
        </section>
      ))}
      {!sections.length && !shownPresets.length && <p className="text-sm text-muted-foreground">{t("fields.type_none")}</p>}
    </div>
  );
}

function GalleryItem({ icon: Icon, label, hint, onClick }: { icon: LucideIcon; label: string; hint: string; onClick: () => void }) {
  const id = useId();
  return (
    <button type="button" aria-label={label} aria-describedby={id} onClick={onClick} className="flex items-start gap-2.5 rounded-md border px-3 py-2 text-left outline-none transition-colors hover:border-primary/40 hover:bg-primary/5 focus-visible:ring-2 focus-visible:ring-ring">
      <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
      <span className="min-w-0">
        <span className="block text-sm font-medium">{label}</span>
        <span id={id} className="block text-xs text-muted-foreground">{hint}</span>
      </span>
    </button>
  );
}

/** The options of a list of options, one row each: name, color, order. */
function OptionsEditor({ id, field, invalid, set }: { id: string; field: Field; invalid: boolean; set: (p: Partial<Field>) => void }) {
  const { t } = useI18n();
  const options = optionsOf(field);
  const [draft, setDraft] = useState("");
  // A comma would split the option in two: the list is stored comma separated.
  const clean = (s: string) => s.replace(/,/g, " ").trim();
  const write = (next: string[], colors = field.colors) => set({ options: next.join(", "), colors: Object.fromEntries(Object.entries(colors).filter(([o]) => next.includes(o))) });
  const add = () => {
    const v = clean(draft);
    if (!v || options.includes(v)) return;
    write([...options, v]);
    setDraft("");
  };
  const rename = (i: number, raw: string) => {
    const v = clean(raw);
    if (!v || (options.includes(v) && options[i] !== v)) return;
    const next = options.map((o, j) => (j === i ? v : o));
    const colors = { ...field.colors };
    if (colors[options[i]]) { colors[v] = colors[options[i]]; delete colors[options[i]]; }
    write(next, colors);
    if (field.default === options[i]) set({ default: v });
  };
  const move = (i: number, d: -1 | 1) => { const next = [...options]; [next[i], next[i + d]] = [next[i + d], next[i]]; write(next); };
  const colorNames = Object.fromEntries(OPTION_COLORS.map((c) => [c, t(`fields.color.${c}` as MessageKey)]));
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-medium">{t("fields.options")}</legend>
      {options.length > 0 && (
        <ul className="space-y-1.5" aria-label={t("fields.options")}>
          {options.map((o, i) => (
            <li key={o} className="flex items-center gap-1.5">
              <Select value={field.colors[o] || "none"} items={{ none: t("fields.color.none"), ...colorNames }} onValueChange={(v) => write(options, { ...field.colors, ...(v === "none" ? { [o]: "" } : { [o]: String(v) }) })}>
                <SelectTrigger size="sm" className="w-9 justify-center px-0 [&>svg:last-child]:hidden" aria-label={`${t("fields.color")}: ${o}`}>
                  <span className={cn("size-3.5 rounded-full border", optionColor(field.colors[o]).dot)} aria-hidden />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="none">{t("fields.color.none")}</SelectItem>
                  {OPTION_COLORS.map((c) => <SelectItem key={c} value={c}><span className={cn("mr-2 inline-block size-3 rounded-full", optionColor(c).dot)} aria-hidden />{colorNames[c]}</SelectItem>)}
                </SelectContent>
              </Select>
              <Input aria-label={`${t("fields.option")} ${i + 1}`} className="h-8 min-w-0 flex-1" defaultValue={o} maxLength={100} onBlur={(e) => rename(i, e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); e.currentTarget.blur(); } }} />
              <IconAction label={t("collections.field_up")} target={o} disabled={i === 0} onClick={() => move(i, -1)}><ArrowUp className="size-4" /></IconAction>
              <IconAction label={t("collections.field_down")} target={o} disabled={i === options.length - 1} onClick={() => move(i, 1)}><ArrowDown className="size-4" /></IconAction>
              <IconAction label={t("fields.option_remove")} target={o} className="text-muted-foreground hover:text-destructive" onClick={() => write(options.filter((x) => x !== o))}><Trash2 className="size-4" /></IconAction>
            </li>
          ))}
        </ul>
      )}
      <div className="flex gap-2">
        <Input id={`${id}-option`} aria-label={t("fields.option_new")} placeholder={t("fields.option_new")} aria-invalid={invalid && !options.length} maxLength={100} value={draft}
          onChange={(e) => setDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); add(); } }} />
        <Button type="button" variant="outline" disabled={!clean(draft)} onClick={add}><Plus className="size-4" />{t("fields.option_add")}</Button>
      </div>
    </fieldset>
  );
}

/** When a field shows: always, or when another field matches. */
function ShowWhenSettings({ id, field, siblings, invalid, set }: { id: string; field: Field; siblings: FieldRow[]; invalid: boolean; set: (p: Partial<Field>) => void }) {
  const { t } = useI18n();
  const when = field.showWhen;
  const others = siblings.filter((r) => r.key !== field.key && !r.advanced && !["group", "objects", "json", "file", "files"].includes(r.field.type));
  const target = others.find((r) => r.key === when?.field)?.field;
  const ops = Object.fromEntries(SHOW_OPS.map((o) => [o, t(`fields.show_op.${o}` as MessageKey)]));
  const put = (p: Partial<ShowWhen>) => set({ showWhen: { field: "", op: "eq", value: "", ...when, ...p } });
  const valueChoices = target?.type === "enum" ? optionsOf(target).map((o) => [o, o]) : target?.type === "boolean" ? [["true", t("common.yes")], ["false", t("common.no")]] : null;
  return (
    <div className="space-y-2">
      <label className="flex items-center gap-2 text-sm">
        <Checkbox checked={!!when} disabled={!others.length} onCheckedChange={(on) => set({ showWhen: on === true ? { field: others[0]?.key ?? "", op: "eq", value: "" } : null, ...(on === true ? { required: false } : {}) })} />
        {t("fields.show_when")}
      </label>
      {!others.length && <p className="text-xs text-muted-foreground">{t("fields.show_when_none")}</p>}
      {when && (
        <div className="grid gap-2 @sm:grid-cols-3">
          <Select value={when.field || null} items={Object.fromEntries(others.map((r) => [r.key, r.field.name]))} onValueChange={(v) => put({ field: String(v ?? ""), value: "" })}>
            <SelectTrigger id={`${id}-show-field`} className="w-full" aria-label={t("fields.show_field")} aria-invalid={invalid && !when.field}><SelectValue placeholder={t("fields.show_field")} /></SelectTrigger>
            <SelectContent>{others.map((r) => <SelectItem key={r.key} value={r.key}>{r.field.name}</SelectItem>)}</SelectContent>
          </Select>
          <Select value={when.op} items={ops} onValueChange={(v) => put({ op: v as ShowWhen["op"] })}>
            <SelectTrigger className="w-full" aria-label={t("fields.show_op")}><SelectValue /></SelectTrigger>
            <SelectContent>{SHOW_OPS.map((o) => <SelectItem key={o} value={o}>{ops[o]}</SelectItem>)}</SelectContent>
          </Select>
          {(when.op === "eq" || when.op === "neq") && (valueChoices ? (
            <Select value={when.value || null} items={Object.fromEntries(valueChoices)} onValueChange={(v) => put({ value: String(v ?? "") })}>
              <SelectTrigger className="w-full" aria-label={t("fields.show_value")} aria-invalid={invalid && !when.value}><SelectValue placeholder={t("fields.show_value")} /></SelectTrigger>
              <SelectContent>{valueChoices.map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}</SelectContent>
            </Select>
          ) : (
            <Input aria-label={t("fields.show_value")} placeholder={t("fields.show_value")} aria-invalid={invalid && !when.value.trim()} value={when.value} onChange={(e) => put({ value: e.target.value })} />
          ))}
          <p className="text-xs text-muted-foreground @sm:col-span-3">{t("fields.show_when_hint")}</p>
        </div>
      )}
    </div>
  );
}

/** A titled part of the field panel. */
function Part({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className="space-y-3 border-t pt-4 first:border-t-0 first:pt-0">
      <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</h3>
      {children}
    </section>
  );
}

/** The fields of another collection, for a lookup or rollup to read. */
function useFieldsOf(collectionId: string) {
  const q = useQuery({ queryKey: ["collection", collectionId], queryFn: () => collections.get(collectionId), enabled: !!collectionId, staleTime: 30_000 });
  return fieldsOf((q.data?.schema as Schema | undefined) ?? null).filter((r) => !r.advanced && !VIRTUAL_TYPES.includes(r.field.type));
}

function PickField({ id, label, value, rows, invalid, onChange }: { id: string; label: string; value: string; rows: FieldRow[]; invalid: boolean; onChange: (key: string) => void }) {
  const items = Object.fromEntries(rows.map((r) => [r.key, r.field.name]));
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Select value={value || null} items={items} onValueChange={(v) => onChange(String(v ?? ""))}>
        <SelectTrigger id={id} className="w-full" aria-invalid={invalid && !value}><SelectValue placeholder={label} /></SelectTrigger>
        <SelectContent>{rows.map((r) => <SelectItem key={r.key} value={r.key}>{r.field.name}</SelectItem>)}</SelectContent>
      </Select>
    </div>
  );
}

/** A lookup: which reference of this record, and which field of the record it names. */
function LookupSettings({ id, field, siblings, invalid, set }: { id: string; field: Field; siblings: FieldRow[]; invalid: boolean; set: (p: Partial<Field>) => void }) {
  const { t } = useI18n();
  const refs = siblings.filter((r) => r.field.type === "reference" || r.field.type === "references");
  const target = refs.find((r) => r.key === field.via)?.field.ref ?? "";
  const shown = useFieldsOf(target);
  return (
    <div className="space-y-2">
      <div className="grid gap-3 @sm:grid-cols-2">
        <PickField id={`${id}-via`} label={t("collections.field_lookup_via")} value={field.via} rows={refs} invalid={invalid} onChange={(via) => set({ via, source: "" })} />
        <PickField id={`${id}-source`} label={t("collections.field_lookup_source")} value={field.source} rows={shown} invalid={invalid} onChange={(source) => set({ source })} />
      </div>
      <p className="text-xs text-muted-foreground">{t(refs.length ? "collections.field_lookup_hint" : "collections.field_lookup_no_refs")}</p>
    </div>
  );
}

/** A rollup: another collection, its reference naming this record, a function and a number field. */
function RollupSettings({ id, field, invalid, set }: { id: string; field: Field; invalid: boolean; set: (p: Partial<Field>) => void }) {
  const { t } = useI18n();
  const rows = useFieldsOf(field.ref);
  const refs = rows.filter((r) => r.field.type === "reference" || r.field.type === "references");
  const numbers = rows.filter((r) => ["number", "integer", "formula", "autonumber"].includes(r.field.type));
  const fns = Object.fromEntries(ROLLUP_FNS.map((f) => [f, t(`collections.rollup_fn.${f}` as MessageKey)]));
  return (
    <div className="space-y-2">
      <div className="grid gap-3 @sm:grid-cols-2">
        <RefSelect id={`${id}-ref`} value={field.ref} invalid={invalid && !field.ref} label="collections.field_rollup_collection" hint="collections.field_rollup_hint" onChange={(ref) => set({ ref, via: "", source: "" })} />
        <PickField id={`${id}-via`} label={t("collections.field_rollup_via")} value={field.via} rows={refs} invalid={invalid} onChange={(via) => set({ via })} />
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${id}-fn`}>{t("collections.field_rollup_fn")}</Label>
          <Select value={field.fn || null} items={fns} onValueChange={(v) => set({ fn: String(v ?? ""), ...(v === "count" ? { source: "" } : {}) })}>
            <SelectTrigger id={`${id}-fn`} className="w-full" aria-invalid={invalid && !field.fn}><SelectValue placeholder={t("collections.field_rollup_fn")} /></SelectTrigger>
            <SelectContent>{ROLLUP_FNS.map((f) => <SelectItem key={f} value={f}>{fns[f]}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        {field.fn && field.fn !== "count" && <PickField id={`${id}-source`} label={t("collections.field_rollup_source")} value={field.source} rows={numbers} invalid={invalid} onChange={(source) => set({ source })} />}
      </div>
    </div>
  );
}

/** Picks the collection a reference points at, among those the user reaches. */
function RefSelect({ id, value, invalid, label = "collections.field_ref", hint = "collections.field_ref_hint", onChange }: { id: string; value: string; invalid: boolean; label?: MessageKey; hint?: MessageKey; onChange: (v: string) => void }) {
  const { t } = useI18n();
  // shortcut: up to 1000 collections by name in one select; a search box when people reach more.
  const list = useQuery({ queryKey: ["collections", "options"], queryFn: ({ signal }) => collections.options(signal), staleTime: 30_000 });
  const items = Object.fromEntries((list.data?.items ?? []).map((c) => [c.id, c.name]));
  if (value && !(value in items)) items[value] = t("collections.field_ref_unknown");
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{t(label)}</Label>
      <Select value={value || null} items={items} onValueChange={(v) => onChange(String(v ?? ""))}>
        <SelectTrigger id={id} className="w-full" aria-invalid={invalid}><SelectValue placeholder={t("collections.field_ref_pick")} /></SelectTrigger>
        <SelectContent>
          {Object.entries(items).map(([v, label]) => <SelectItem key={v} value={v}>{label}</SelectItem>)}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t(hint)}</p>
    </div>
  );
}

function DefaultSelect({ id, field, onChange }: { id: string; field: Field; onChange: (v: string) => void }) {
  const { t } = useI18n();
  const NONE = "\u0000none";
  const choices = field.type === "boolean" ? [["true", t("common.yes")], ["false", t("common.no")]] : optionsOf(field).map((o) => [o, o]);
  const items: Record<string, string> = { [NONE]: t("collections.field_default_none"), ...Object.fromEntries(choices) };
  return (
    <Select value={field.default || NONE} items={items} onValueChange={(v) => onChange(v === NONE ? "" : String(v))}>
      <SelectTrigger id={id} className="w-full"><SelectValue /></SelectTrigger>
      <SelectContent>
        {Object.entries(items).map(([v, label]) => <SelectItem key={v} value={v}>{label}</SelectItem>)}
      </SelectContent>
    </Select>
  );
}


/** A side panel: title on top, the body scrolls, the footer stays in view. */
export function SidePanel({ open, onOpenChange, title, description, footer, wide, keepOnOutsideClick, children }: {
  open: boolean; onOpenChange: (open: boolean) => void; title: string; description?: string; footer?: React.ReactNode; wide?: boolean;
  /** A popover of the panel renders outside it (the conditions): a click there must not close the panel. */
  keepOnOutsideClick?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange} disablePointerDismissal={keepOnOutsideClick}>
      <SheetContent side="right" className={cn("gap-0 data-[side=right]:w-full", wide ? "data-[side=right]:sm:max-w-3xl" : "data-[side=right]:sm:max-w-xl")}>
        <SheetHeader className="border-b pr-12">
          <SheetTitle>{title}</SheetTitle>
          {description && <SheetDescription>{description}</SheetDescription>}
        </SheetHeader>
        <div className="@container min-h-0 flex-1 overflow-y-auto p-4">{children}</div>
        {footer && <SheetFooter className="mt-0 border-t">{footer}</SheetFooter>}
      </SheetContent>
    </Sheet>
  );
}

/** One field, shown as the record form would show it, with its own test value. */
function FieldPreview({ field }: { field: Field }) {
  const { t } = useI18n();
  const [data, setData] = useState<unknown>({});
  const key = field.key.trim() || "preview";
  // The condition and the section belong to the whole form; a broken pattern would stop the check.
  const prop = { ...propOf({ ...field, name: field.name.trim() || t(`collections.type.${field.type}`), showWhen: null, section: "", width: "" }) };
  if (fieldProblem(field, () => false) === "collections.field_pattern_invalid") delete prop.pattern;
  const schema = { type: "object", properties: { [key]: prop }, required: field.required ? [key] : [] };
  return (
    <div className="rounded-lg border bg-background p-3">
      <SchemaForm key={`${field.type}-${canonical(prop)}`} schema={schema} data={data} onChange={setData} idPrefix="field-preview" liveValidate />
    </div>
  );
}

/** Field keys a formula may use, offered as buttons that write them in. */
function FormulaHelp({ siblings, self, onInsert }: { siblings: FieldRow[]; self: string; onInsert: (key: string) => void }) {
  const { t } = useI18n();
  const usable = siblings.filter((r) => r.key !== self && !r.advanced && ["number", "integer", "string", "text", "boolean", "date", "datetime", "enum", "formula", "autonumber"].includes(r.field.type));
  if (!usable.length) return null;
  return (
    <div className="flex flex-wrap items-center gap-1.5" aria-label={t("fields.formula_insert")} role="group">
      <span className="text-xs text-muted-foreground">{t("fields.formula_insert")}</span>
      {usable.map((r) => (
        <Button key={r.key} type="button" size="sm" variant="outline" className="h-6 px-2 font-mono text-xs" title={r.field.name} onClick={() => onInsert(r.key)}>{r.key}</Button>
      ))}
    </div>
  );
}

/** The panel that writes one field: basics, what its type needs, rules, display, advanced, then a preview. */
function FieldForm({ formId, offered, topLevel, initial, initialKey, keyLocked, typeLocked, taken, siblings, onSubmit }: {
  formId: string;
  offered: readonly FieldType[];
  /** Top-level fields may be unique, private, conditional and placed in sections. */
  topLevel: boolean;
  initial: Field;
  initialKey?: string;
  keyLocked?: boolean;
  /** The records hold values: only types that accept them are offered. */
  typeLocked?: boolean;
  taken: (key: string) => boolean;
  siblings: FieldRow[];
  onSubmit: (f: Field) => void;
}) {
  const { t } = useI18n();
  const id = useId();
  const [field, setField] = useState<Field>(initial);
  // A new field's key follows its name until someone types a key.
  const [keyTouched, setKeyTouched] = useState(!!initialKey);
  const [tried, setTried] = useState(false);
  const set = (patch: Partial<Field>) => setField((f) => ({ ...f, ...patch, ...(patch.name !== undefined && !keyTouched ? { key: keyOf(patch.name) } : {}) }));
  const problem = fieldProblem(field, taken, initialKey);
  // The key clash shows at once; the rest once a save was tried.
  const shown = problem === "collections.field_exists" || tried ? problem : null;
  const types = Object.fromEntries(offered.map((k) => [k, t(`collections.type.${k}`)]));
  const objectType = isObjectType(field.type);
  const limits = limitKeys(field.type);
  const [minLabel, maxLabel]: [MessageKey, MessageKey] = limits?.[0] === "minimum" ? ["collections.field_min", "collections.field_max"]
    : limits?.[0] === "minItems" ? ["collections.field_min_items", "collections.field_max_items"]
    : ["collections.field_min_length", "collections.field_max_length"];
  const isDate = field.type === "date" || field.type === "datetime";
  const numberFormats = { plain: t("dashboard.format.number"), currency: t("dashboard.format.currency"), percent: t("dashboard.format.percent"), compact: t("dashboard.format.compact") };
  const decimalItems = { auto: t("dashboard.decimals_auto"), ...Object.fromEntries(["0", "1", "2", "3"].map((d) => [d, d])) };
  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    // The panel is portaled, but React still bubbles the submit to the collection's form.
    e.stopPropagation();
    setTried(true);
    if (!problem) return onSubmit({ ...field, key: field.key.trim() });
    const form = e.currentTarget as HTMLFormElement;
    requestAnimationFrame(() => (form.querySelector("[aria-invalid=true]") as HTMLElement | null)?.focus());
  };
  const insertKey = (k: string) => set({ options: `${field.options}${field.options && !/\s$/.test(field.options) ? " " : ""}${k}` });
  const TypeIcon = TYPE_ICONS[field.type];

  return (
    <form id={formId} noValidate onSubmit={submit} className="space-y-5">
      <Part title={t("fields.part.basic")}>
        <div className="grid gap-3 @sm:grid-cols-[minmax(0,1fr)_13rem]">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={`${id}-name`}>{t("collections.field_name")}</Label>
            <Input id={`${id}-name`} value={field.name} autoFocus maxLength={120} aria-invalid={shown === "collections.field_name_needed"} onChange={(e) => set({ name: e.target.value })} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={`${id}-type`}>{t("collections.field_type")}</Label>
            <Select value={field.type} items={types} onValueChange={(v) => set({ type: v as FieldType, default: "", min: "", max: "", today: false, pattern: "" })}>
              <SelectTrigger id={`${id}-type`} className="w-full"><TypeIcon className="size-4 text-muted-foreground" aria-hidden /><SelectValue /></SelectTrigger>
              <SelectContent>
                {offered.map((k) => { const I = TYPE_ICONS[k]; return <SelectItem key={k} value={k}><I className="size-4 text-muted-foreground" aria-hidden />{types[k]}</SelectItem>; })}
              </SelectContent>
            </Select>
          </div>
        </div>
        <p className="text-xs text-muted-foreground">{t(`collections.type_hint.${field.type}`)}</p>
        {typeLocked && <p className="text-xs text-muted-foreground">{t("fields.type_locked")}</p>}
        {!AUTO_TYPES.includes(field.type) && (
          <label className="flex w-fit items-center gap-2 text-sm">
            <Checkbox checked={field.required} disabled={!!field.showWhen} onCheckedChange={(on) => set({ required: on === true })} />
            {t("collections.field_required")}
            {field.showWhen && <span className="text-xs text-muted-foreground">({t("fields.required_conditional")})</span>}
          </label>
        )}
      </Part>

      {(field.type === "enum" || field.type === "formula" || field.type === "lookup" || field.type === "rollup" || field.type === "reference" || field.type === "references" || field.type === "group" || field.type === "objects" || field.type === "json" || AUTO_TYPES.includes(field.type)) && (
        <Part title={t("fields.part.type")}>
          {field.type === "enum" && <OptionsEditor id={id} field={field} invalid={shown === "collections.field_options_needed"} set={set} />}
          {field.type === "formula" && (
            <div className="space-y-2">
              <div className="grid gap-3 @sm:grid-cols-[minmax(0,1fr)_10rem]">
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor={`${id}-formula`}>{t("collections.field_formula")}</Label>
                  <Input id={`${id}-formula`} className="font-mono" placeholder="preco * quantidade" value={field.options} aria-invalid={shown === "collections.field_formula_needed"} aria-describedby={`${id}-formula-hint`} onChange={(e) => set({ options: e.target.value })} />
                </div>
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor={`${id}-result`}>{t("collections.field_formula_result")}</Label>
                  <Select value={field.result || "number"} items={Object.fromEntries(FORMULA_RESULTS.map((r) => [r, t(`collections.formula_result.${r}`)]))} onValueChange={(v) => set({ result: String(v) })}>
                    <SelectTrigger id={`${id}-result`} className="w-full"><SelectValue /></SelectTrigger>
                    <SelectContent>{FORMULA_RESULTS.map((r) => <SelectItem key={r} value={r}>{t(`collections.formula_result.${r}`)}</SelectItem>)}</SelectContent>
                  </Select>
                </div>
              </div>
              <FormulaHelp siblings={siblings} self={field.key} onInsert={insertKey} />
              <p id={`${id}-formula-hint`} className="text-xs text-muted-foreground">{t("collections.field_formula_hint")}</p>
            </div>
          )}
          {AUTO_TYPES.includes(field.type) && field.type !== "formula" && !VIRTUAL_TYPES.includes(field.type) && <p className="text-xs text-muted-foreground">{t("collections.field_auto_hint")}</p>}
          {field.type === "lookup" && <LookupSettings id={id} field={field} siblings={siblings} invalid={shown === "collections.field_lookup_needed"} set={set} />}
          {field.type === "rollup" && <RollupSettings id={id} field={field} invalid={shown === "collections.field_rollup_needed"} set={set} />}
          {(field.type === "reference" || field.type === "references") && <RefSelect id={`${id}-ref`} value={field.ref} invalid={shown === "collections.field_ref_needed"} onChange={(ref) => set({ ref })} />}
          {(field.type === "group" || field.type === "objects" || field.type === "json") && (
            <p className={cn("text-xs", field.type === "json" ? "text-warn" : "text-muted-foreground")}>
              {t(field.type === "json" ? "collections.json_hint" : field.type === "objects" ? "collections.objects_hint" : "collections.group_hint")}
            </p>
          )}
        </Part>
      )}

      {(limits || PATTERN_TYPES.includes(field.type) || (topLevel && UNIQUE_TYPES.includes(field.type))) && (
        <Part title={t("fields.part.rules")}>
          {limits && (
            <div className="grid gap-3 @sm:grid-cols-2">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-min`}>{t(minLabel)}</Label>
                <Input id={`${id}-min`} type="number" value={field.min} aria-invalid={shown === "collections.field_limits_invalid"} onChange={(e) => set({ min: e.target.value })} />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-max`}>{t(maxLabel)}</Label>
                <Input id={`${id}-max`} type="number" value={field.max} placeholder={t("collections.field_no_max")}
                  aria-invalid={shown === "collections.field_limits_invalid" || shown === "collections.field_string_max"} onChange={(e) => set({ max: e.target.value })} />
              </div>
            </div>
          )}
          {PATTERN_TYPES.includes(field.type) && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={`${id}-pattern`}>{t("fields.pattern")}</Label>
              <Input id={`${id}-pattern`} className="font-mono" spellCheck={false} maxLength={500} placeholder="^[A-Z]{3}-\d{4}$" value={field.pattern} aria-invalid={shown === "collections.field_pattern_invalid"} aria-describedby={`${id}-pattern-hint`} onChange={(e) => set({ pattern: e.target.value })} />
              <p id={`${id}-pattern-hint`} className="text-xs text-muted-foreground">{t("fields.pattern_hint")}</p>
            </div>
          )}
          {topLevel && UNIQUE_TYPES.includes(field.type) && (
            <label className="flex w-fit items-center gap-2 text-sm">
              <Checkbox checked={field.unique} onCheckedChange={(on) => set({ unique: on === true })} />
              {t("collections.field_unique")}
              <span className="text-xs text-muted-foreground">{t("collections.field_unique_hint")}</span>
            </label>
          )}
        </Part>
      )}

      <Part title={t("fields.part.display")}>
        <div className="grid gap-3 @sm:grid-cols-2">
          <div className="flex flex-col gap-1.5 @sm:col-span-2">
            <Label htmlFor={`${id}-desc`}>{t("collections.field_description")}</Label>
            <Input id={`${id}-desc`} value={field.description} maxLength={500} aria-describedby={`${id}-desc-hint`} onChange={(e) => set({ description: e.target.value })} />
            <p id={`${id}-desc-hint`} className="text-xs text-muted-foreground">{t("collections.field_description_hint")}</p>
          </div>
          {field.type !== "group" && field.type !== "objects" && field.type !== "file" && field.type !== "files" && !AUTO_TYPES.includes(field.type) && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={`${id}-ph`}>{t("collections.field_placeholder")}</Label>
              <Input id={`${id}-ph`} value={field.placeholder} maxLength={200} onChange={(e) => set({ placeholder: e.target.value })} />
            </div>
          )}
          {!objectType && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={`${id}-default`}>{t("collections.field_default")}</Label>
              {field.type === "boolean" || field.type === "enum" ? (
                <DefaultSelect id={`${id}-default`} field={field} onChange={(v) => set({ default: v })} />
              ) : (
                <Input id={`${id}-default`} value={field.today ? "" : field.default} disabled={field.today} aria-invalid={shown === "collections.field_default_invalid"}
                  type={field.type === "number" || field.type === "integer" ? "number" : field.type === "date" ? "date" : field.type === "datetime" ? "datetime-local" : "text"}
                  aria-describedby={field.type === "strings" ? `${id}-default-hint` : undefined} onChange={(e) => set({ default: e.target.value })} />
              )}
              {isDate && (
                <label className="flex w-fit items-center gap-2 text-sm">
                  <Checkbox checked={field.today} onCheckedChange={(on) => set({ today: on === true, default: "" })} />
                  {t(field.type === "date" ? "fields.default_today" : "fields.default_now")}
                </label>
              )}
              {field.type === "strings" && <p id={`${id}-default-hint`} className="text-xs text-muted-foreground">{t("collections.field_default_list_hint")}</p>}
            </div>
          )}
          {isNumberField(field) && (
            <>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-format`}>{t("fields.number_format")}</Label>
                <Select value={field.numberFormat || "plain"} items={numberFormats} onValueChange={(v) => set({ numberFormat: v === "plain" ? "" : (v as Field["numberFormat"]) })}>
                  <SelectTrigger id={`${id}-format`} className="w-full"><SelectValue /></SelectTrigger>
                  <SelectContent>{Object.entries(numberFormats).map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}</SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{t("fields.number_format_hint")}</p>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-decimals`}>{t("dashboard.decimals")}</Label>
                <Select value={field.decimals || "auto"} items={decimalItems} onValueChange={(v) => set({ decimals: v === "auto" ? "" : String(v) })}>
                  <SelectTrigger id={`${id}-decimals`} className="w-full"><SelectValue /></SelectTrigger>
                  <SelectContent>{Object.entries(decimalItems).map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}</SelectContent>
                </Select>
              </div>
            </>
          )}
          {topLevel && (
            <>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-section`}>{t("fields.section")}</Label>
                <Input id={`${id}-section`} list={`${id}-sections`} value={field.section} maxLength={80} aria-describedby={`${id}-section-hint`} onChange={(e) => set({ section: e.target.value })} />
                <datalist id={`${id}-sections`}>{[...new Set(siblings.map((r) => r.field.section).filter(Boolean))].map((s) => <option key={s} value={s} />)}</datalist>
                <p id={`${id}-section-hint`} className="text-xs text-muted-foreground">{t("fields.section_hint")}</p>
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-width`}>{t("fields.width")}</Label>
                <Select value={field.width || "auto"} items={{ auto: t("fields.width.auto"), half: t("fields.width.half"), full: t("fields.width.full") }} onValueChange={(v) => set({ width: v === "auto" ? "" : (v as Field["width"]) })}>
                  <SelectTrigger id={`${id}-width`} className="w-full"><SelectValue /></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="auto">{t("fields.width.auto")}</SelectItem>
                    <SelectItem value="half">{t("fields.width.half")}</SelectItem>
                    <SelectItem value="full">{t("fields.width.full")}</SelectItem>
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{t("fields.width_hint")}</p>
              </div>
              <div className="@sm:col-span-2">
                <ShowWhenSettings id={id} field={field} siblings={siblings} invalid={shown === "collections.field_show_needed"} set={set} />
              </div>
            </>
          )}
        </div>
      </Part>

      <Part title={t("fields.part.advanced")}>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${id}-key`}>{t("collections.field_key")}</Label>
          <Input id={`${id}-key`} className="font-mono" value={field.key} maxLength={64} disabled={keyLocked} spellCheck={false} aria-describedby={`${id}-key-hint`}
            aria-invalid={shown === "collections.field_key_invalid" || shown === "collections.field_exists"}
            onChange={(e) => { setKeyTouched(true); setField((f) => ({ ...f, key: e.target.value })); }} />
          <p id={`${id}-key-hint`} className="text-xs text-muted-foreground">{t(keyLocked ? "collections.field_key_locked" : "collections.field_key_hint")}</p>
        </div>
        {topLevel && (
          <label className="flex w-fit items-center gap-2 text-sm">
            <Checkbox checked={field.private} onCheckedChange={(on) => set({ private: on === true })} />
            {t("collections.field_private")}
            <span className="text-xs text-muted-foreground">{t("collections.field_private_hint")}</span>
          </label>
        )}
      </Part>

      {field.type !== "group" && field.type !== "objects" && (
        <Part title={t("fields.preview")}>
          <FieldPreview field={field} />
        </Part>
      )}
      {shown && <p role="alert" className="text-sm text-destructive">{t(shown)}</p>}
      {/* Enter in any input saves, as the old inline form did. */}
      <button type="submit" hidden aria-hidden tabIndex={-1} />
    </form>
  );
}

type Panel = { mode: "add"; type: FieldType | null } | { mode: "edit"; key: string } | null;

/** The list of fields: each row opens its panel; groups and lists open their subfields in place. */
export function FieldsEditor({ schema, original, editable, keysLocked = false, group, items = false, onChange }: {
  schema: Schema | null;
  original?: Schema | null;
  editable: boolean;
  /** Records exist: existing keys stay as they are. */
  keysLocked?: boolean;
  /** Set when this edits a group's subfields: the group's name. */
  group?: string;
  /** The subfields are a list's item: they may also be references. */
  items?: boolean;
  onChange: (next: Schema) => void;
}) {
  const { t, locale } = useI18n();
  const formId = useId();
  const [panel, setPanel] = useState<Panel>(null);
  const [dragging, setDragging] = useState<string | null>(null);
  const [over, setOver] = useState<number | null>(null);
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const [q, setQ] = useState("");
  const rows = fieldsOf(schema);
  const keys = new Set(rows.map((r) => r.key));
  const saved = original ? propsOf(original) : {};
  const savedRequired = original ? requiredOf(original) : [];
  const removed = fieldsOf(original ?? null).filter((r) => !keys.has(r.key));
  const offered = items ? ITEM_TYPES : group ? SUBFIELD_TYPES : FIELD_TYPES;
  const topLevel = !group;

  if (!schema) return <p className="text-sm text-muted-foreground">{t("collections.fields_need_json")}</p>;
  const mark = (key: string, required: boolean): MessageKey | null => {
    if (!original) return null;
    if (!(key in saved)) return "collections.field_new";
    return canonical(saved[key]) !== canonical(propsOf(schema)[key]) || required !== savedRequired.includes(key) ? "collections.field_changed" : null;
  };
  const drop = (to: number) => {
    if (dragging) onChange(moveFieldTo(schema, dragging, to));
    setDragging(null);
    setOver(null);
  };
  // Search shows once the list is long; while it filters, order stays put.
  const searching = rows.length > 12 && q.trim() !== "";
  const term = q.trim().toLocaleLowerCase();
  const visible = searching ? rows.filter((r) => r.field.name.toLocaleLowerCase().includes(term) || r.key.includes(term)) : rows;
  const freeKey = (base: string) => { let k = base, n = 2; while (keys.has(k)) k = `${base}_${n++}`.slice(0, 64); return k; };
  const addPreset = (p: Preset) => {
    const [key, prop] = p.make(t, locale === "pt-BR");
    onChange(addProp(schema, freeKey(key), prop));
    setPanel(null);
  };
  const editing = panel?.mode === "edit" ? rows.find((r) => r.key === panel.key) : undefined;
  const typed = panel?.mode === "add" && panel.type;
  const title = panel?.mode === "edit" ? `${t("collections.field_editing")} ${editing?.field.name ?? ""}`
    : group ? `${t("collections.add_subfield")}: ${group}` : t("collections.add_field");
  // A heading where a new form section starts, as the record form will show it.
  const headings = new Map(topLevel && !searching ? rows.flatMap((r, i) => {
    const before = rows.slice(0, i).reverse().find((x) => x.field.section)?.field.section ?? "";
    return r.field.section && r.field.section !== before ? [[r.key, r.field.section] as const] : [];
  }) : []);

  return (
    <div className="space-y-3">
      {rows.length > 12 && (
        <div className="relative">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
          <Input type="search" aria-label={t("fields.search")} placeholder={t("fields.search")} className="pl-8" value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
      )}
      {rows.length === 0 ? (
        <p className="rounded-lg border border-dashed px-4 py-4 text-center text-sm text-muted-foreground">{t(group ? "collections.subfields_empty" : "collections.fields_empty")}</p>
      ) : !visible.length ? (
        <p className="px-1 text-sm text-muted-foreground">{t("fields.search_none")}</p>
      ) : (
        <ul aria-label={group ? `${t("collections.subfields")}: ${group}` : t("collections.fields")} className="divide-y rounded-lg border">
          {visible.map(({ key, field, advanced }) => {
            const i = rows.findIndex((r) => r.key === key);
            const badge = mark(key, field.required);
            const prop = propsOf(schema)[key];
            const isList = field.type === "objects" && !advanced;
            const isGroup = (field.type === "group" || isList) && !advanced;
            const open = isGroup && !collapsed.has(key);
            // A list of objects keeps its subfields on the item schema.
            const inner = (p: Schema | undefined) => (isList ? ((p?.items ?? {}) as Schema) : p);
            const subCount = isGroup ? Object.keys(propsOf(inner(prop) ?? {})).length : 0;
            const Icon = advanced ? Braces : TYPE_ICONS[field.type];
            const heading = headings.get(key);
            const colors = field.type === "enum" ? optionsOf(field).slice(0, 4) : [];
            return (
              <li key={key}>
              {heading && <p className="border-b bg-muted/40 px-3 py-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{heading}</p>}
              {/* The drag handlers sit on the row, so a group's subfields drag on their own. */}
              <div
                className={cn("group/row flex items-center gap-x-2 gap-y-1 px-2 py-2 transition-colors", dragging === key && "opacity-50", over === i && dragging !== key && "bg-primary/10")}
                draggable={editable && !searching}
                onDragStart={(e) => { setDragging(key); e.dataTransfer.effectAllowed = "move"; }}
                onDragOver={(e) => { if (!dragging) return; e.preventDefault(); setOver(i); }}
                onDragLeave={() => setOver((o) => (o === i ? null : o))}
                onDrop={(e) => { e.preventDefault(); drop(i); }}
                onDragEnd={() => { setDragging(null); setOver(null); }}
              >
                {editable && !searching && <GripVertical className="size-4 shrink-0 cursor-grab text-muted-foreground" aria-label={t("collections.field_drag")} />}
                {isGroup && (
                  <IconAction label={t(open ? "collections.group_collapse" : "collections.group_expand")} target={field.name} expanded={open} className="-mx-1"
                    onClick={() => setCollapsed((c) => { const n = new Set(c); if (open) n.add(key); else n.delete(key); return n; })}>
                    {open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                  </IconAction>
                )}
                <span className="grid size-8 shrink-0 place-items-center rounded-md bg-muted text-muted-foreground" title={advanced ? t("collections.type.advanced") : t(`collections.type.${field.type}`)}>
                  <Icon className="size-4" aria-hidden />
                </span>
                <button type="button" aria-label={`${t("collections.field_edit")}: ${field.name}`} disabled={!editable || advanced} onClick={() => setPanel({ mode: "edit", key })}
                  className="min-w-0 flex-1 rounded-sm text-left outline-none focus-visible:ring-2 focus-visible:ring-ring enabled:cursor-pointer">
                  <span className="flex flex-wrap items-center gap-1.5 text-sm font-medium">
                    <span className="truncate">{field.name}</span>
                    {field.required && <Badge variant="outline" className="text-muted-foreground">{t("collections.field_required")}</Badge>}
                    {field.unique && <Badge variant="outline" className="text-muted-foreground">{t("collections.field_unique")}</Badge>}
                    {field.private && <Badge variant="outline" className="text-muted-foreground">{t("collections.field_private")}</Badge>}
                    {field.showWhen && <Badge variant="outline" className="text-muted-foreground" title={t("fields.show_when")}><Eye className="size-3" aria-hidden />{t("fields.conditional")}</Badge>}
                    {badge && <Badge variant="outline" className={badge === "collections.field_new" ? "border-primary/40 text-primary" : "border-warn/40 text-warn"}>{t(badge)}</Badge>}
                  </span>
                  <span className="flex min-w-0 flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground">
                    <span className="shrink-0">{advanced ? t("collections.type.advanced") : t(`collections.type.${field.type}`)}</span>
                    {colors.length > 0 && <span className="flex min-w-0 flex-wrap items-center gap-x-1.5">· {colors.map((o) => <span key={o} className="inline-flex items-center gap-1"><span className={cn("size-2 rounded-full", optionColor(field.colors[o]).dot)} aria-hidden />{o}</span>)}{optionsOf(field).length > 4 && "…"}</span>}
                    {isGroup && <span className="shrink-0">· {subCount === 1 ? t("collections.subfield_one") : t("collections.subfield_many").replace("{n}", String(subCount))}</span>}
                    {field.width === "half" && <span className="shrink-0">· {t("fields.width.half")}</span>}
                    {/* The key is for builders: it shows on hover or focus. */}
                    <span className="truncate font-mono opacity-0 transition-opacity max-sm:hidden group-hover/row:opacity-100 group-focus-within/row:opacity-100">· {key}</span>
                  </span>
                </button>
                {editable && (
                  // Phones: one menu instead of five buttons, so the name keeps the row.
                  <DropdownMenu>
                    <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="icon" className="ml-auto shrink-0 sm:hidden" aria-label={`${t("common.actions")}: ${field.name}`} />}><MoreHorizontal className="size-4" /></DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem disabled={i === 0 || searching} onClick={() => onChange(moveField(schema, key, -1))}><ArrowUp className="size-4" />{t("collections.field_up")}</DropdownMenuItem>
                      <DropdownMenuItem disabled={i === rows.length - 1 || searching} onClick={() => onChange(moveField(schema, key, 1))}><ArrowDown className="size-4" />{t("collections.field_down")}</DropdownMenuItem>
                      <DropdownMenuItem onClick={() => onChange(duplicateField(schema, key, t("fields.copy_suffix")))}><Copy className="size-4" />{t("fields.duplicate")}</DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem variant="destructive" onClick={() => onChange(removeField(schema, key))}><Trash2 className="size-4" />{t("collections.field_remove")}</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
                {editable && (
                  <div className="ml-auto flex shrink-0 max-sm:hidden">
                    {/* With a mouse the arrows wait for hover or focus; touch and keyboard keep them, as drag's stand-in. */}
                    <span className="flex transition-opacity [@media(hover:hover)]:opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100">
                      <IconAction label={t("collections.field_up")} target={field.name} disabled={i === 0 || searching} onClick={() => onChange(moveField(schema, key, -1))}><ArrowUp className="size-4" /></IconAction>
                      <IconAction label={t("collections.field_down")} target={field.name} disabled={i === rows.length - 1 || searching} onClick={() => onChange(moveField(schema, key, 1))}><ArrowDown className="size-4" /></IconAction>
                      <IconAction label={t("fields.duplicate")} target={field.name} onClick={() => onChange(duplicateField(schema, key, t("fields.copy_suffix")))}><Copy className="size-4" /></IconAction>
                    </span>
                    <IconAction label={t("collections.field_remove")} target={field.name} className="text-muted-foreground hover:text-destructive" onClick={() => onChange(removeField(schema, key))}><Trash2 className="size-4" /></IconAction>
                  </div>
                )}
              </div>
              {open && (
                <div className="mb-3 ml-3 mr-2 border-l-2 pl-2 sm:ml-8 sm:pl-3">
                  <FieldsEditor
                    schema={inner(prop) ?? null}
                    // A new group is marked once, not again on each of its subfields.
                    original={original && key in saved ? inner(saved[key]) : undefined}
                    editable={editable}
                    keysLocked={keysLocked && key in saved}
                    group={field.name}
                    items={isList}
                    onChange={(sub) => onChange(setProp(schema, key, isList ? { ...prop, items: sub } : sub))}
                  />
                </div>
              )}
              </li>
            );
          })}
        </ul>
      )}
      {editable && removed.length > 0 && original && (
        <ul aria-label={t("collections.field_removed")} className="flex flex-wrap items-center gap-2 text-sm">
          <li className="text-muted-foreground">{t("collections.field_removed")}</li>
          {removed.map((r) => (
            <li key={r.key} className="flex items-center gap-1 rounded-full border border-dashed py-0.5 pl-2.5 pr-0.5">
              <span className="line-through">{r.field.name}</span>
              <Button type="button" size="sm" variant="ghost" className="h-6 px-2" onClick={() => onChange(restoreField(schema, original, r.key))}>
                <RotateCcw className="size-3.5" />{t("collections.field_restore")}
              </Button>
            </li>
          ))}
        </ul>
      )}
      {editable && (
        <Button type="button" variant="outline" size={group ? "sm" : "default"} className="w-full border-dashed" onClick={() => setPanel({ mode: "add", type: null })}>
          <Plus className="size-4" />{t(group ? "collections.add_subfield" : "collections.add_field")}
        </Button>
      )}
      <SidePanel
        open={panel !== null}
        onOpenChange={(o) => !o && setPanel(null)}
        title={title}
        description={panel?.mode === "add" && !typed ? t("fields.gallery_hint") : undefined}
        footer={(panel?.mode === "edit" || typed) && (
          <div className="flex flex-wrap items-center justify-end gap-2">
            {typed && <Button type="button" variant="ghost" className="mr-auto" onClick={() => setPanel({ mode: "add", type: null })}>{t("fields.change_type")}</Button>}
            <Button type="button" variant="outline" onClick={() => setPanel(null)}>{t("common.cancel")}</Button>
            <Button type="submit" form={formId}>{panel?.mode === "edit" ? t("collections.field_save") : <><Plus className="size-4" />{t(group ? "collections.add_subfield" : "collections.add_field")}</>}</Button>
          </div>
        )}
      >
        {panel?.mode === "add" && !panel.type && (
          <TypeGallery offered={offered} presets={topLevel} onPick={(type) => setPanel({ mode: "add", type })} onPreset={addPreset} />
        )}
        {typed && (
          <FieldForm key={`new-${typed}`} formId={formId} offered={offered} topLevel={topLevel} initial={{ ...blankField, type: typed }} taken={(k) => keys.has(k)} siblings={rows}
            onSubmit={(f) => { onChange(addField(schema, f)); setPanel(null); }} />
        )}
        {editing && (
          <FieldForm key={editing.key} formId={formId} topLevel={topLevel} initial={editing.field} initialKey={editing.key}
            // A field whose records hold values keeps a type that still accepts them.
            offered={keysLocked && editing.key in saved ? typesKeepingValues(typeOf(saved[editing.key]) ?? editing.field.type, offered) : offered}
            typeLocked={keysLocked && editing.key in saved}
            keyLocked={keysLocked && editing.key in saved} taken={(k) => k !== editing.key && keys.has(k)} siblings={rows}
            onSubmit={(f) => { onChange(updateField(schema, editing.key, f)); setPanel(null); }} />
        )}
      </SidePanel>
    </div>
  );
}
