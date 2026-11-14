"use client";

import { useId, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, ChevronDown, ChevronRight, GripVertical, MoreHorizontal, Pencil, Plus, RotateCcw, Trash2 } from "lucide-react";
import { IconAction } from "@/components/bits";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { collections } from "@/lib/api/endpoints";
import type { MessageKey } from "@/lib/i18n/messages";
import type { FileRef } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";

export const FIELD_TYPES = ["string", "text", "number", "integer", "boolean", "enum", "strings", "date", "datetime", "email", "url", "reference", "references", "file", "files", "autonumber", "formula", "created_by", "updated_by", "group", "objects", "json"] as const;
export type FieldType = (typeof FIELD_TYPES)[number];
/** Types the server fills on every write; the form shows them read-only. */
export const AUTO_TYPES: readonly FieldType[] = ["autonumber", "formula", "created_by", "updated_by"];
/** Groups hold one level of subfields: no group inside a group; the server checks references and files on top-level fields only. */
const SUBFIELD_TYPES = FIELD_TYPES.filter((t) => t !== "group" && t !== "objects" && t !== "reference" && t !== "references" && t !== "file" && t !== "files" && !AUTO_TYPES.includes(t));
/** A list of objects' items may also point at a record: the server checks a reference subfield of each item. */
const ITEM_TYPES: readonly FieldType[] = [...SUBFIELD_TYPES, "reference"];
/** Plain values the server can compare across records: these can be marked unique. */
const UNIQUE_TYPES: readonly FieldType[] = ["string", "number", "integer", "enum", "date", "datetime", "email", "url"];
/** The keyword the server reads (schema.UniqueKeyword): no two records share the value. */
const UNIQUE = "x-unique";
/** The server keeps these fields off share links (schema.PrivateKeyword). */
const PRIVATE = "x-private";
/** The keywords the server reads (schema.RefKeyword, schema.FileKeyword): a record of another collection, an attached file. */
export const REF = "x-ref";
/** The keywords the server reads (schema.AutoKeyword, schema.FormulaKeyword): fields it fills itself. */
export const AUTO = "x-auto";
export const FORMULA = "x-formula";
export const FILE = "x-file";
/** Types with no default the builder edits; any default is kept as it was. */
const isObjectType = (t: FieldType) => t === "group" || t === "objects" || t === "json" || t === "file" || t === "files" || t === "reference" || t === "references" || AUTO_TYPES.includes(t);
/** How a record stores an attached file (tools.upload answers it). */
const FILE_ITEM = { type: "object", required: ["id"], properties: { id: { type: "string" }, name: { type: "string" }, size: { type: "integer" }, type: { type: "string" } } };
const FILE_PROP = { ...FILE_ITEM, [FILE]: true };
const FILES_PROP = { type: "array", [FILE]: true, items: FILE_ITEM, maxItems: 20 };

/** The files a file field holds: one {id, ...} or a list of them. */
export function fileRefs(v: unknown): FileRef[] {
  const list = Array.isArray(v) ? v : v ? [v] : [];
  return list.filter((f): f is FileRef => !!f && typeof f === "object" && typeof (f as FileRef).id === "string");
}
export const isImage = (f: FileRef) => /^image\//.test(f.type ?? "") || /\.(png|jpe?g|gif|webp|avif|svg)$/i.test(f.name ?? "");

/** Long text: SchemaForm draws a text area for strings allowed this long. */
export const LONG_TEXT = 5000;
/** The shortest maxLength SchemaForm (uiFor) draws as a text area. */
const TEXT_AREA_FROM = 500;

/** String formats the builder offers as types; the server asserts them. */
const FORMATS: Partial<Record<FieldType, string>> = { date: "date", datetime: "date-time", email: "email", url: "uri" };

/**
 * A field as the builder edits it. Rules are text while typed: empty means none.
 * `key` is where the value is stored, `name` its label.
 */
export type Field = {
  name: string;
  key: string;
  type: FieldType;
  required: boolean;
  /** No two records may hold the same value; top-level fields only. */
  unique: boolean;
  /** Kept off share links. */
  private: boolean;
  options: string;
  description: string;
  placeholder: string;
  default: string;
  min: string;
  max: string;
  /** A reference's collection id. */
  ref: string;
};
/** A top-level field: `key` is where its value is stored, `field.name` its label. Advanced ones are edited as JSON. */
export type FieldRow = { key: string; field: Field; advanced: boolean };

type Schema = Record<string, unknown>;
type Props = Record<string, Record<string, unknown>>;

export const blankField: Field = { name: "", key: "", type: "string", required: false, unique: false, private: false, options: "", description: "", placeholder: "", default: "", min: "", max: "", ref: "" };

const optionsOf = (f: Pick<Field, "options">) => f.options.split(",").map((o) => o.trim()).filter(Boolean);

/** The JSON Schema keywords holding a type's lower and upper limit, if it has them. */
function limitKeys(type: FieldType): [string, string] | null {
  switch (type) {
    case "string": case "text": case "email": case "url": return ["minLength", "maxLength"];
    case "number": case "integer": return ["minimum", "maximum"];
    case "strings": case "references": case "objects": return ["minItems", "maxItems"];
    default: return null;
  }
}

/** Keywords the builder writes; any other keyword of a property is kept as it was. */
const RULE_KEYS = ["title", "description", "examples", "default", "minLength", "maxLength", "minimum", "maximum", "minItems", "maxItems", UNIQUE, PRIVATE, REF];

/** A storage key from a label: "Nome do Cliente" → "nome_do_cliente". */
export function keyOf(name: string): string {
  const k = name.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "").slice(0, 64);
  return !k ? "" : /^[a-z]/.test(k) ? k : `f_${k}`;
}
const KEY_FORMAT = /^[a-z][a-z0-9_]{0,63}$/;

/** The default value as stored, or undefined for none (or one that cannot be read as the type). */
function parseDefault(f: Field): unknown {
  const raw = f.default.trim();
  if (!raw || isObjectType(f.type)) return undefined;
  switch (f.type) {
    case "number": case "integer": return raw === "" || !Number.isFinite(Number(raw)) ? undefined : Number(raw);
    case "boolean": return raw === "true";
    // datetime-local has no zone; the schema's date-time wants one.
    case "datetime": return Number.isNaN(Date.parse(raw)) ? undefined : new Date(raw).toISOString();
    case "strings": return raw.split(",").map((s) => s.trim()).filter(Boolean);
    default: return raw;
  }
}

/** "2026-11-12T13:05:00.000Z" as the local time a datetime-local input shows. */
function localDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

function defaultText(v: unknown, type: FieldType): string {
  if (v === undefined || v === null) return "";
  if (type === "datetime" && typeof v === "string") return localDateTime(v);
  if (Array.isArray(v)) return v.join(", ");
  return typeof v === "object" ? JSON.stringify(v) : String(v);
}

/** The bare property of a type, before its rules. */
function typeBase(f: Field): Record<string, unknown> {
  const format = FORMATS[f.type];
  if (format) return { type: "string", format };
  switch (f.type) {
    case "text": return { type: "string" };
    case "enum": return { type: "string", enum: optionsOf(f) };
    case "strings": return { type: "array", items: { type: "string" } };
    case "group": return { type: "object", properties: {} };
    case "objects": return { type: "array", items: { type: "object", properties: {} } };
    case "json": return { type: "object" };
    case "reference": return { type: "string", format: "uuid" };
    case "references": return { type: "array", items: { type: "string", format: "uuid" }, uniqueItems: true };
    case "autonumber": return { type: "integer", [AUTO]: "number" };
    case "formula": return { type: "number", [FORMULA]: f.options.trim() };
    case "created_by": case "updated_by": return { type: "string", [AUTO]: f.type };
    case "file": return FILE_PROP;
    case "files": return FILES_PROP;
    default: return { type: f.type };
  }
}

/** Writes the builder's rules onto a property, removing the ones left empty. */
function withRules(base: Record<string, unknown>, f: Field): Record<string, unknown> {
  const out = { ...base };
  for (const k of RULE_KEYS) if (k !== "default" || !isObjectType(f.type)) delete out[k];
  out.title = f.name.trim();
  if (f.description.trim()) out.description = f.description.trim();
  if (f.placeholder.trim()) out.examples = [f.placeholder.trim()];
  const def = parseDefault(f);
  if (def !== undefined) out.default = def;
  const limits = limitKeys(f.type);
  if (limits) {
    if (f.min.trim()) out[limits[0]] = Number(f.min);
    if (f.max.trim()) out[limits[1]] = Number(f.max);
  }
  if (f.type === "text" && out.maxLength === undefined) out.maxLength = LONG_TEXT;
  if (f.unique && UNIQUE_TYPES.includes(f.type)) out[UNIQUE] = true;
  if (f.private) out[PRIVATE] = true;
  if (f.type === "enum") out.enum = optionsOf(f);
  if (f.type === "reference" || f.type === "references") out[REF] = f.ref;
  // Title first, as people read it in the JSON.
  const { title, ...rest } = out;
  return { title, ...rest };
}

/** The builder type that draws this property, or undefined when only JSON can describe it. */
export function typeOf(p: unknown): FieldType | undefined {
  if (!p || typeof p !== "object") return undefined;
  const s = p as Record<string, unknown>;
  // Options are edited comma separated, so an option holding a comma stays in JSON.
  if (Array.isArray(s.enum)) return s.enum.every((v) => typeof v === "string" && !v.includes(",")) ? "enum" : undefined;
  if (typeof s[FORMULA] === "string" && s.type === "number") return "formula";
  if (s[AUTO] === "number" && s.type === "integer") return "autonumber";
  if ((s[AUTO] === "created_by" || s[AUTO] === "updated_by") && s.type === "string") return s[AUTO] as FieldType;
  if (s.type === "string" && typeof s[REF] === "string") return "reference";
  if (s.type === "array" && typeof s[REF] === "string") return "references";
  if (s.type === "object" && s[FILE] === true) return "file";
  if (s.type === "array" && s[FILE] === true) return "files";
  switch (s.type) {
    case "string": {
      const format = Object.entries(FORMATS).find(([, f]) => f === s.format)?.[0] as FieldType | undefined;
      if (format) return format;
      return typeof s.maxLength === "number" && s.maxLength >= TEXT_AREA_FROM ? "text" : "string";
    }
    case "number":
    case "integer":
    case "boolean": return s.type;
    case "array": {
      const items = s.items as Record<string, unknown> | undefined;
      if (items?.type === "object" && items.properties && typeof items.properties === "object" && !Array.isArray(items.properties))
        return Object.values(items.properties).every((sub) => { const t = typeOf(sub); return t && ITEM_TYPES.includes(t); }) ? "objects" : undefined;
      return items?.type === "string" && !items.enum && !items.format ? "strings" : undefined;
    }
    case "object": {
      // No properties: anything goes, edited as JSON in the record form.
      if (s.properties === undefined) return s.patternProperties === undefined && s.additionalProperties === undefined ? "json" : undefined;
      if (!s.properties || typeof s.properties !== "object") return undefined;
      return Object.values(s.properties).every((sub) => { const t = typeOf(sub); return t && t !== "group"; }) ? "group" : undefined;
    }
  }
  return undefined;
}

/** JSON with object keys sorted, so a reordered property compares equal. */
function canonical(v: unknown): string {
  return JSON.stringify(v, (_, x) => (x && typeof x === "object" && !Array.isArray(x) ? Object.fromEntries(Object.entries(x).sort(([a], [b]) => a.localeCompare(b))) : x));
}

const propsOf = (schema: Schema) => (schema.properties ?? {}) as Props;
const requiredOf = (schema: Schema) => (Array.isArray(schema.required) ? (schema.required as string[]) : []);

/** The first plain text value a record has, in field order, to name it in lists and titles. */
export function recordTitle(schema: Schema | null, data: unknown): string | undefined {
  const obj = data && typeof data === "object" ? (data as Record<string, unknown>) : {};
  const v = fieldsOf(schema).filter((r) => !r.advanced).map((r) => obj[r.key]).find((x) => typeof x === "string" && x.trim());
  return typeof v === "string" ? v : undefined;
}

export function fieldsOf(schema: Schema | null): FieldRow[] {
  if (!schema) return [];
  const required = requiredOf(schema);
  return Object.entries(propsOf(schema)).map(([key, p]) => {
    const type = typeOf(p) ?? "string";
    const limits = limitKeys(type);
    const num = (k?: string) => (k && typeof p?.[k] === "number" ? String(p[k]) : "");
    const examples = Array.isArray(p?.examples) ? p.examples : [];
    return {
      key,
      advanced: !typeOf(p),
      field: {
        name: typeof p?.title === "string" && p.title ? p.title : key,
        key,
        type,
        required: required.includes(key),
        unique: p?.[UNIQUE] === true,
        private: p?.[PRIVATE] === true,
        options: Array.isArray(p?.enum) ? p.enum.join(", ") : typeof p?.[FORMULA] === "string" ? p[FORMULA] : "",
        description: typeof p?.description === "string" ? p.description : "",
        placeholder: typeof examples[0] === "string" ? examples[0] : "",
        default: isObjectType(type) ? "" : defaultText(p?.default, type),
        min: num(limits?.[0]),
        // The long-text default limit is how the type is stored, not a rule someone set.
        max: type === "text" && p?.maxLength === LONG_TEXT ? "" : num(limits?.[1]),
        ref: typeof p?.[REF] === "string" ? p[REF] : "",
      },
    };
  });
}

/**
 * Why the field cannot be saved yet, or null. `taken` says whether a key is used
 * by another field.
 */
export function fieldProblem(f: Field, taken: (key: string) => boolean, initialKey?: string): MessageKey | null {
  if (!f.name.trim()) return "collections.field_name_needed";
  const key = f.key.trim();
  if (key !== initialKey && !KEY_FORMAT.test(key)) return "collections.field_key_invalid";
  if (key !== initialKey && taken(key)) return "collections.field_exists";
  if (f.type === "enum" && optionsOf(f).length === 0) return "collections.field_options_needed";
  if (f.type === "formula" && !f.options.trim()) return "collections.field_formula_needed";
  if ((f.type === "reference" || f.type === "references") && !f.ref) return "collections.field_ref_needed";
  const limits = limitKeys(f.type);
  const lo = f.min.trim() ? Number(f.min) : undefined;
  const hi = f.max.trim() ? Number(f.max) : undefined;
  if (limits) {
    const counts = limits[0] !== "minimum"; // lengths and item counts are whole numbers
    const bad = (n?: number) => n !== undefined && (!Number.isFinite(n) || (counts && (!Number.isInteger(n) || n < 0)));
    if (bad(lo) || bad(hi) || (lo !== undefined && hi !== undefined && lo > hi)) return "collections.field_limits_invalid";
    if (f.type === "text" && hi !== undefined && hi < TEXT_AREA_FROM) return "collections.field_text_max";
    if (f.type === "string" && hi !== undefined && hi >= TEXT_AREA_FROM) return "collections.field_string_max";
  }
  const raw = f.default.trim();
  if (raw) {
    const def = parseDefault(f);
    const within = (n: number) => (lo === undefined || n >= lo) && (hi === undefined || n <= hi);
    const ok =
      f.type === "number" ? def !== undefined && within(def as number)
      : f.type === "integer" ? def !== undefined && Number.isInteger(def) && within(def as number)
      : f.type === "enum" ? optionsOf(f).includes(raw)
      : f.type === "strings" ? within((def as string[]).length)
      : f.type === "boolean" || f.type === "date" ? true
      : f.type === "datetime" ? def !== undefined
      : within(raw.length);
    if (!ok) return "collections.field_default_invalid";
  }
  return null;
}

/** The schema with these properties (in this order) and required list, every other key kept. */
function rebuild(schema: Schema, entries: [string, unknown][], required: string[]): Schema {
  const rest = { ...schema };
  delete rest.required;
  return { ...rest, type: schema.type ?? "object", properties: Object.fromEntries(entries), ...(required.length ? { required } : {}) };
}

/** Adds one top-level field to an object schema, keeping every other key as it was. */
export function addField(schema: Schema, f: Field): Schema {
  const key = f.key.trim() || keyOf(f.name);
  const required = requiredOf(schema);
  return rebuild(schema, [...Object.entries(propsOf(schema)), [key, withRules(typeBase(f), f)]], f.required && !AUTO_TYPES.includes(f.type) && !required.includes(key) ? [...required, key] : required);
}

/**
 * Changes a field in place, under a new key when `f.key` differs. On the same
 * type the property's other keywords (pattern, format…) stay; a new type starts fresh.
 */
export function updateField(schema: Schema, key: string, f: Field): Schema {
  const to = f.key.trim() || key;
  const entries = Object.entries(propsOf(schema)).map(([k, old]): [string, unknown] => {
    if (k !== key) return [k, old];
    return [to, typeOf(old) === f.type ? withRules(old, f) : withRules(typeBase(f), f)];
  });
  const required = requiredOf(schema).filter((k) => k !== key);
  return rebuild(schema, entries, f.required && !AUTO_TYPES.includes(f.type) ? [...required, to] : required);
}

/** Replaces one property as a whole, as a group's subfield editor hands it back. */
export function setProp(schema: Schema, key: string, prop: Schema): Schema {
  return rebuild(schema, Object.entries(propsOf(schema)).map(([k, p]) => [k, k === key ? prop : p]), requiredOf(schema));
}

export function removeField(schema: Schema, key: string): Schema {
  return rebuild(schema, Object.entries(propsOf(schema)).filter(([k]) => k !== key), requiredOf(schema).filter((k) => k !== key));
}

/** Moves a field to position `to`; the form follows this order. */
export function moveFieldTo(schema: Schema, key: string, to: number): Schema {
  const entries = Object.entries(propsOf(schema));
  const i = entries.findIndex(([k]) => k === key);
  if (i < 0 || to < 0 || to >= entries.length || to === i) return schema;
  const [moved] = entries.splice(i, 1);
  entries.splice(to, 0, moved);
  return rebuild(schema, entries, requiredOf(schema));
}

/** Moves a field one place up (-1) or down (+1). */
export function moveField(schema: Schema, key: string, delta: -1 | 1): Schema {
  const i = Object.keys(propsOf(schema)).indexOf(key);
  return i < 0 ? schema : moveFieldTo(schema, key, i + delta);
}

/** Puts a removed field back as it was in `original`, near its old place. */
export function restoreField(schema: Schema, original: Schema, key: string): Schema {
  const prop = propsOf(original)[key];
  if (!prop || key in propsOf(schema)) return schema;
  const entries = Object.entries(propsOf(schema));
  entries.splice(Math.min(Object.keys(propsOf(original)).indexOf(key), entries.length), 0, [key, prop]);
  const required = requiredOf(schema);
  return rebuild(schema, entries, requiredOf(original).includes(key) ? [...required, key] : required);
}

export type SchemaChange = { kind: "added" | "removed" | "required" | "unique" | "type" | "rules"; name: string; risky: boolean };

/** What saving `next` over `original` changes, field by field; risky ones can leave records out of format. */
export function schemaChanges(original: Schema | null, next: Schema | null): SchemaChange[] {
  if (!original || !next) return [];
  const before = new Map(fieldsOf(original).map((r) => [r.key, r]));
  const after = fieldsOf(next);
  const out: SchemaChange[] = [];
  for (const r of after) {
    const old = before.get(r.key);
    if (!old) {
      out.push({ kind: "added", name: r.field.name, risky: r.field.required });
      continue;
    }
    if (r.field.required && !old.field.required) out.push({ kind: "required", name: r.field.name, risky: true });
    // Safe for records: the server refuses the save if stored values repeat.
    if (r.field.unique && !old.field.unique) out.push({ kind: "unique", name: r.field.name, risky: false });
    // Unique has its own line above; the rest of the keywords compare as rules.
    const strip = (p: Record<string, unknown>) => ({ ...p, [UNIQUE]: undefined });
    const [was, now] = [strip(propsOf(original)[r.key]), strip(propsOf(next)[r.key])];
    if (r.advanced !== old.advanced || r.field.type !== old.field.type) out.push({ kind: "type", name: r.field.name, risky: true });
    else if ((r.field.type === "group" || r.field.type === "objects") && !r.advanced) {
      // A group (or a list's item) reports its subfields by name; its own rules apart from them.
      const inner = (p: Record<string, unknown>) => (r.field.type === "objects" ? (p.items ?? {}) as Record<string, unknown> : p);
      const bare = (p: Record<string, unknown>) => canonical(r.field.type === "objects" ? { ...p, items: { ...inner(p), properties: undefined, required: undefined } } : { ...p, properties: undefined, required: undefined });
      if (bare(was) !== bare(now)) out.push({ kind: "rules", name: r.field.name, risky: true });
      for (const c of schemaChanges(inner(was), inner(now))) out.push({ ...c, name: `${r.field.name} › ${c.name}` });
    } else if (canonical(was) !== canonical(now)) out.push({ kind: "rules", name: r.field.name, risky: true });
  }
  const kept = new Set(after.map((r) => r.key));
  for (const r of before.values()) if (!kept.has(r.key)) out.push({ kind: "removed", name: r.field.name, risky: true });
  return out;
}

/** The form that writes one field into the schema, so nobody types JSON for the common cases. */
function FieldForm({ label, types: offered = FIELD_TYPES, uniqueAllowed = true, initial, initialKey, keyLocked, taken, submitLabel, onSubmit, onCancel }: {
  /** Names the group for assistive tech: adding, or editing which field. */
  label: string;
  types?: readonly FieldType[];
  /** False inside a group: the server checks unique values on top-level fields only. */
  uniqueAllowed?: boolean;
  initial: Field;
  /** The key the field has today; unset for a new field. */
  initialKey?: string;
  keyLocked?: boolean;
  taken: (key: string) => boolean;
  submitLabel: string;
  onSubmit: (f: Field) => void;
  onCancel?: () => void;
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
  const hasRules = !!(initial.description || initial.placeholder || initial.default || initial.min || initial.max);
  const submit = () => {
    setTried(true);
    if (problem) return;
    onSubmit({ ...field, key: field.key.trim() });
    setField(blankField);
    setKeyTouched(false);
    setTried(false);
  };
  // Enter saves the field instead of submitting the collection form.
  const enter = (e: React.KeyboardEvent) => { if (e.key === "Enter" && (e.target as HTMLElement).tagName === "INPUT") { e.preventDefault(); submit(); } };
  const note = `${id}-note`;

  return (
    <div role="group" aria-label={label} className="@container space-y-3 rounded-lg border border-dashed p-3" onKeyDown={enter}>
      <div className="grid items-end gap-3 @sm:grid-cols-[minmax(0,1fr)_12rem_auto]">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${id}-name`}>{t("collections.field_name")}</Label>
          <Input id={`${id}-name`} value={field.name} autoFocus={!!onCancel} maxLength={120} aria-invalid={shown === "collections.field_name_needed"} aria-describedby={note} onChange={(e) => set({ name: e.target.value })} />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${id}-type`}>{t("collections.field_type")}</Label>
          <Select value={field.type} items={types} onValueChange={(v) => set({ type: v as FieldType, default: "", min: "", max: "" })}>
            <SelectTrigger id={`${id}-type`} className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>
              {offered.map((k) => <SelectItem key={k} value={k} description={t(`collections.type_hint.${k}`)}>{types[k]}</SelectItem>)}
            </SelectContent>
          </Select>
        </div>
        <div className="flex h-[var(--control-h)] items-center gap-4">
          {!AUTO_TYPES.includes(field.type) && (
            <label className="flex items-center gap-2 text-sm">
              <Checkbox checked={field.required} onCheckedChange={(on) => set({ required: on === true })} />
              {t("collections.field_required")}
            </label>
          )}
          {uniqueAllowed && UNIQUE_TYPES.includes(field.type) && (
            <label className="flex items-center gap-2 text-sm" title={t("collections.field_unique_hint")}>
              <Checkbox checked={field.unique} onCheckedChange={(on) => set({ unique: on === true })} />
              {t("collections.field_unique")}
            </label>
          )}
          {uniqueAllowed && (
            <label className="flex items-center gap-2 text-sm" title={t("collections.field_private_hint")}>
              <Checkbox checked={field.private} onCheckedChange={(on) => set({ private: on === true })} />
              {t("collections.field_private")}
            </label>
          )}
        </div>
      </div>
      {field.type === "enum" && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${id}-options`}>{t("collections.field_options")}</Label>
          <Input id={`${id}-options`} value={field.options} aria-invalid={shown === "collections.field_options_needed"} onChange={(e) => set({ options: e.target.value })} />
        </div>
      )}
      {field.type === "formula" && (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor={`${id}-formula`}>{t("collections.field_formula")}</Label>
          <Input id={`${id}-formula`} className="font-mono" placeholder="preco * quantidade" value={field.options} aria-invalid={shown === "collections.field_formula_needed"} aria-describedby={`${id}-formula-hint`} onChange={(e) => set({ options: e.target.value })} />
          <p id={`${id}-formula-hint`} className="text-xs text-muted-foreground">{t("collections.field_formula_hint")}</p>
        </div>
      )}
      {AUTO_TYPES.includes(field.type) && field.type !== "formula" && <p className="text-xs text-muted-foreground">{t("collections.field_auto_hint")}</p>}
      {(field.type === "reference" || field.type === "references") && <RefSelect id={`${id}-ref`} value={field.ref} invalid={shown === "collections.field_ref_needed"} onChange={(ref) => set({ ref })} />}
      {(field.type === "group" || field.type === "objects" || field.type === "json") && (
        <p className={cn("text-xs", field.type === "json" ? "text-warn" : "text-muted-foreground")}>
          {t(field.type === "json" ? "collections.json_hint" : field.type === "objects" ? "collections.objects_hint" : "collections.group_hint")}
        </p>
      )}
      <details className="group" open={hasRules || undefined}>
        <summary className="flex w-fit cursor-pointer list-none items-center gap-1.5 text-sm font-medium text-muted-foreground hover:text-foreground [&::-webkit-details-marker]:hidden">
          <ChevronRight className="size-4 transition-transform group-open:rotate-90" aria-hidden />
          {t("collections.field_more")}
        </summary>
        <div className="mt-3 grid gap-3 @sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={`${id}-key`}>{t("collections.field_key")}</Label>
            <Input id={`${id}-key`} className="font-mono" value={field.key} maxLength={64} disabled={keyLocked} spellCheck={false} aria-describedby={`${id}-key-hint`}
              aria-invalid={shown === "collections.field_key_invalid" || shown === "collections.field_exists"}
              onChange={(e) => { setKeyTouched(true); setField((f) => ({ ...f, key: e.target.value })); }} />
            <p id={`${id}-key-hint`} className="text-xs text-muted-foreground">{t(keyLocked ? "collections.field_key_locked" : "collections.field_key_hint")}</p>
          </div>
          {!objectType && <div className="flex flex-col gap-1.5">
            <Label htmlFor={`${id}-default`}>{t("collections.field_default")}</Label>
            {field.type === "boolean" || field.type === "enum" ? (
              <DefaultSelect id={`${id}-default`} field={field} onChange={(v) => set({ default: v })} />
            ) : (
              <Input id={`${id}-default`} value={field.default} aria-invalid={shown === "collections.field_default_invalid"}
                type={field.type === "number" || field.type === "integer" ? "number" : field.type === "date" ? "date" : field.type === "datetime" ? "datetime-local" : "text"}
                aria-describedby={field.type === "strings" ? `${id}-default-hint` : undefined} onChange={(e) => set({ default: e.target.value })} />
            )}
            {field.type === "strings" && <p id={`${id}-default-hint`} className="text-xs text-muted-foreground">{t("collections.field_default_list_hint")}</p>}
          </div>}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor={`${id}-desc`}>{t("collections.field_description")}</Label>
            <Input id={`${id}-desc`} value={field.description} maxLength={500} aria-describedby={`${id}-desc-hint`} onChange={(e) => set({ description: e.target.value })} />
            <p id={`${id}-desc-hint`} className="text-xs text-muted-foreground">{t("collections.field_description_hint")}</p>
          </div>
          {field.type !== "group" && field.type !== "objects" && field.type !== "file" && field.type !== "files" && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor={`${id}-ph`}>{t("collections.field_placeholder")}</Label>
              <Input id={`${id}-ph`} value={field.placeholder} maxLength={200} onChange={(e) => set({ placeholder: e.target.value })} />
            </div>
          )}
          {limits && (
            <>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-min`}>{t(minLabel)}</Label>
                <Input id={`${id}-min`} type="number" value={field.min} aria-invalid={shown === "collections.field_limits_invalid"} onChange={(e) => set({ min: e.target.value })} />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor={`${id}-max`}>{t(maxLabel)}</Label>
                <Input id={`${id}-max`} type="number" value={field.max} placeholder={field.type === "text" ? String(LONG_TEXT) : undefined}
                  aria-invalid={shown === "collections.field_limits_invalid" || shown === "collections.field_text_max" || shown === "collections.field_string_max"} onChange={(e) => set({ max: e.target.value })} />
              </div>
            </>
          )}
        </div>
      </details>
      <div className="flex flex-wrap items-center justify-end gap-2">
        {shown && <p id={note} role="alert" className="mr-auto text-xs text-destructive">{t(shown)}</p>}
        {onCancel && <Button type="button" variant="ghost" onClick={onCancel}>{t("common.cancel")}</Button>}
        <Button type="button" onClick={submit}>
          {!initialKey && <Plus className="size-4" />}{submitLabel}
        </Button>
      </div>
    </div>
  );
}

/** Picks the collection a reference points at, among those the user reaches. */
function RefSelect({ id, value, invalid, onChange }: { id: string; value: string; invalid: boolean; onChange: (v: string) => void }) {
  const { t } = useI18n();
  // shortcut: the first 100 collections by name; a search box when people reach more.
  const list = useQuery({ queryKey: ["collections", "options"], queryFn: () => collections.list({ page: 1, limit: 100, sort: "name", order: "asc" }) });
  const items = Object.fromEntries((list.data?.items ?? []).map((c) => [c.id, c.name]));
  if (value && !(value in items)) items[value] = t("collections.field_ref_unknown");
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{t("collections.field_ref")}</Label>
      <Select value={value || null} items={items} onValueChange={(v) => onChange(String(v ?? ""))}>
        <SelectTrigger id={id} className="w-full" aria-invalid={invalid}><SelectValue placeholder={t("collections.field_ref_pick")} /></SelectTrigger>
        <SelectContent>
          {Object.entries(items).map(([v, label]) => <SelectItem key={v} value={v}>{label}</SelectItem>)}
        </SelectContent>
      </Select>
      <p className="text-xs text-muted-foreground">{t("collections.field_ref_hint")}</p>
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

/**
 * The schema's fields as a list: add, edit in place, remove and reorder (drag or
 * arrows) without touching JSON. Against `original`, the saved schema, rows show
 * what is new or changed and removed fields can come back.
 */
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
  const { t } = useI18n();
  const [editing, setEditing] = useState<string | null>(null);
  const [dragging, setDragging] = useState<string | null>(null);
  const [over, setOver] = useState<number | null>(null);
  // One form at a time per list: adding closes the edit and the other way round.
  const [adding, setAdding] = useState(false);
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const rows = fieldsOf(schema);
  const keys = new Set(rows.map((r) => r.key));
  const saved = original ? propsOf(original) : {};
  const savedRequired = original ? requiredOf(original) : [];
  const removed = fieldsOf(original ?? null).filter((r) => !keys.has(r.key));

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

  return (
    <div className="space-y-3">
      {rows.length === 0 ? (
        <p className="rounded-lg border border-dashed px-4 py-4 text-center text-sm text-muted-foreground">{t(group ? "collections.subfields_empty" : "collections.fields_empty")}</p>
      ) : (
        <ul aria-label={group ? `${t("collections.subfields")}: ${group}` : t("collections.fields")} className="divide-y rounded-lg border">
          {rows.map(({ key, field, advanced }, i) => {
            const badge = mark(key, field.required);
            if (editable && key === editing) {
              return (
                <li key={key} className="bg-muted/30 p-2">
                  <FieldForm
                    label={`${t("collections.field_editing")} ${field.name}`}
                    types={items ? ITEM_TYPES : group ? SUBFIELD_TYPES : FIELD_TYPES}
                    uniqueAllowed={!group}
                    initial={field}
                    initialKey={key}
                    keyLocked={keysLocked && key in saved}
                    taken={(k) => k !== key && keys.has(k)}
                    submitLabel={t("collections.field_save")}
                    onSubmit={(f) => { onChange(updateField(schema, key, f)); setEditing(null); }}
                    onCancel={() => setEditing(null)}
                  />
                </li>
              );
            }
            const prop = propsOf(schema)[key];
            const isList = field.type === "objects" && !advanced;
            const isGroup = (field.type === "group" || isList) && !advanced;
            const open = isGroup && !collapsed.has(key);
            // A list of objects keeps its subfields on the item schema.
            const inner = (p: Schema | undefined) => (isList ? ((p?.items ?? {}) as Schema) : p);
            const subCount = isGroup ? Object.keys(propsOf(inner(prop) ?? {})).length : 0;
            return (
              <li key={key}>
              {/* The drag handlers sit on the row, so a group's subfields drag on their own. */}
              <div
                className={cn("group/row flex items-center gap-x-2 gap-y-1 px-2 py-2 transition-colors", dragging === key && "opacity-50", over === i && dragging !== key && "bg-primary/10")}
                draggable={editable}
                onDragStart={(e) => { setDragging(key); e.dataTransfer.effectAllowed = "move"; }}
                onDragOver={(e) => { if (!dragging) return; e.preventDefault(); setOver(i); }}
                onDragLeave={() => setOver((o) => (o === i ? null : o))}
                onDrop={(e) => { e.preventDefault(); drop(i); }}
                onDragEnd={() => { setDragging(null); setOver(null); }}
              >
                {editable && <GripVertical className="size-4 shrink-0 cursor-grab text-muted-foreground" aria-label={t("collections.field_drag")} />}
                {isGroup && (
                  <IconAction label={t(open ? "collections.group_collapse" : "collections.group_expand")} target={field.name} expanded={open} className="-mx-1"
                    onClick={() => setCollapsed((c) => { const n = new Set(c); if (open) n.add(key); else n.delete(key); return n; })}>
                    {open ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                  </IconAction>
                )}
                <div className="min-w-0 flex-1">
                  <p className="flex items-center gap-2 truncate text-sm font-medium">
                    <span className="truncate">
                      {field.name}
                      {field.required && <span className="ml-1 text-destructive" title={t("collections.field_required")}>*</span>}
                    </span>
                    {badge && <Badge variant="outline" className={badge === "collections.field_new" ? "border-primary/40 text-primary" : "text-muted-foreground"}>{t(badge)}</Badge>}
                  </p>
                  <p className="truncate text-xs text-muted-foreground">
                    {advanced ? t("collections.type.advanced") : t(`collections.type.${field.type}`)}
                    {field.type === "enum" && !advanced && `: ${field.options}`}
                    {field.unique && ` · ${t("collections.field_unique").toLowerCase()}`}
                    {isGroup && ` · ${subCount === 1 ? t("collections.subfield_one") : t("collections.subfield_many").replace("{n}", String(subCount))}`}
                    <span className="font-mono"> · {key}</span>
                  </p>
                </div>
                {editable && (
                  // Phones: one menu instead of four buttons, so the name keeps the row.
                  <DropdownMenu>
                    <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="icon" className="ml-auto shrink-0 sm:hidden" aria-label={`${t("common.actions")}: ${field.name}`} />}><MoreHorizontal className="size-4" /></DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem disabled={i === 0} onClick={() => onChange(moveField(schema, key, -1))}><ArrowUp className="size-4" />{t("collections.field_up")}</DropdownMenuItem>
                      <DropdownMenuItem disabled={i === rows.length - 1} onClick={() => onChange(moveField(schema, key, 1))}><ArrowDown className="size-4" />{t("collections.field_down")}</DropdownMenuItem>
                      <DropdownMenuItem disabled={advanced} onClick={() => { setAdding(false); setEditing(key); }}><Pencil className="size-4" />{t("collections.field_edit")}</DropdownMenuItem>
                      <DropdownMenuItem variant="destructive" onClick={() => { if (editing === key) setEditing(null); onChange(removeField(schema, key)); }}><Trash2 className="size-4" />{t("collections.field_remove")}</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
                {editable && (
                  <div className="ml-auto flex shrink-0 max-sm:hidden">
                    {/* With a mouse the arrows wait for hover or focus; touch and keyboard keep them, as drag's stand-in. */}
                    <span className="flex transition-opacity [@media(hover:hover)]:opacity-0 group-hover/row:opacity-100 group-focus-within/row:opacity-100">
                    <IconAction label={t("collections.field_up")} target={field.name} disabled={i === 0} onClick={() => onChange(moveField(schema, key, -1))}><ArrowUp className="size-4" /></IconAction>
                    <IconAction label={t("collections.field_down")} target={field.name} disabled={i === rows.length - 1} onClick={() => onChange(moveField(schema, key, 1))}><ArrowDown className="size-4" /></IconAction>
                    </span>
                    <IconAction label={t("collections.field_edit")} target={field.name} disabled={advanced} onClick={() => { setAdding(false); setEditing(key); }}><Pencil className="size-4" /></IconAction>
                    <IconAction label={t("collections.field_remove")} target={field.name} className="text-destructive" onClick={() => { if (editing === key) setEditing(null); onChange(removeField(schema, key)); }}><Trash2 className="size-4" /></IconAction>
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
      {editable && !adding && (
        <Button type="button" variant="outline" size={group ? "sm" : "default"} className="w-full border-dashed" onClick={() => { setEditing(null); setAdding(true); }}>
          <Plus className="size-4" />{t(group ? "collections.add_subfield" : "collections.add_field")}
        </Button>
      )}
      {editable && adding && (
        <FieldForm
          key="new"
          label={group ? `${t("collections.add_subfield")}: ${group}` : t("collections.add_field")}
          types={items ? ITEM_TYPES : group ? SUBFIELD_TYPES : FIELD_TYPES}
          uniqueAllowed={!group}
          initial={blankField}
          taken={(k) => keys.has(k)}
          submitLabel={t(group ? "collections.add_subfield" : "collections.add_field")}
          // Stays open for the next one; Cancel closes it.
          onSubmit={(f) => onChange(addField(schema, f))}
          onCancel={() => setAdding(false)}
        />
      )}
    </div>
  );
}
