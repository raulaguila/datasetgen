// The field model: how a field the builder edits maps to and from JSON Schema.
// The editor UI is in fields-editor.tsx.
import type { MessageKey } from "@/lib/i18n/messages";
import type { FileRef } from "@/lib/api/types";

export const FIELD_TYPES = ["string", "text", "number", "integer", "boolean", "enum", "strings", "date", "datetime", "email", "url", "reference", "references", "file", "files", "autonumber", "formula", "lookup", "rollup", "created_by", "updated_by", "group", "objects", "json"] as const;
export type FieldType = (typeof FIELD_TYPES)[number];
/** Types the server fills on every write; the form shows them read-only. */
export const AUTO_TYPES: readonly FieldType[] = ["autonumber", "formula", "lookup", "rollup", "created_by", "updated_by"];
/** Computed when records are read, never stored: lists cannot filter or group by them. */
export const VIRTUAL_TYPES: readonly FieldType[] = ["lookup", "rollup"];
export const ROLLUP_FNS = ["count", "sum", "avg", "min", "max"] as const;
export const FORMULA_RESULTS = ["number", "text", "boolean", "date"] as const;
/** Groups hold one level of subfields: no group inside a group; the server checks references and files on top-level fields only. */
export const SUBFIELD_TYPES = FIELD_TYPES.filter((t) => t !== "group" && t !== "objects" && t !== "reference" && t !== "references" && t !== "file" && t !== "files" && !AUTO_TYPES.includes(t));
/** A list of objects' items may also point at a record: the server checks a reference subfield of each item. */
export const ITEM_TYPES: readonly FieldType[] = [...SUBFIELD_TYPES, "reference"];
/** Plain values the server can compare across records: these can be marked unique. */
export const UNIQUE_TYPES: readonly FieldType[] = ["string", "number", "integer", "enum", "date", "datetime", "email", "url"];
/** The keyword the server reads (schema.UniqueKeyword): no two records share the value. */
const UNIQUE = "x-unique";
/** The server keeps these fields off share links (schema.PrivateKeyword). */
const PRIVATE = "x-private";
/** The keywords the server reads (schema.RefKeyword, schema.FileKeyword): a record of another collection, an attached file. */
export const REF = "x-ref";
/** The keywords the server reads (schema.AutoKeyword, schema.FormulaKeyword): fields it fills itself. */
export const AUTO = "x-auto";
export const FORMULA = "x-formula";
export const LOOKUP = "x-lookup";
export const ROLLUP = "x-rollup";
export const FILE = "x-file";
/** Display keywords only the web app reads: the server ignores them. */
/** An option's color, by option: {"Feito": "green"}. */
export const COLORS = "x-colors";
/** A heading the form shows above this field. */
export const SECTION = "x-section";
/** "half": half the form's width where two fit; "full": the whole row in a two-column form. */
export const WIDTH = "x-width";
/** The form shows the field only when another top-level field matches: {field, op, value}. */
export const SHOW_WHEN = "x-show-when";
/** "today": a new record's date (or date and time) starts as now. */
export const DEFAULT_NOW = "x-default";
/** The colors an option may take; record-view and the board draw them. */
export const OPTION_COLORS = ["gray", "red", "orange", "amber", "green", "teal", "blue", "violet", "pink"] as const;
export type OptionColor = (typeof OPTION_COLORS)[number];
export const SHOW_OPS = ["eq", "neq", "filled", "empty"] as const;
export type ShowWhen = { field: string; op: (typeof SHOW_OPS)[number]; value: string };
/** Types with no default the builder edits; any default is kept as it was. */
export const isObjectType = (t: FieldType) => t === "group" || t === "objects" || t === "json" || t === "file" || t === "files" || t === "reference" || t === "references" || AUTO_TYPES.includes(t);
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
export const TEXT_AREA_FROM = 500;

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
  /** A reference's collection id; a rollup's: the collection it sums up. */
  ref: string;
  /** Lookup: this record's reference field. Rollup: the other collection's reference field naming this record. */
  via: string;
  /** The field read there (a rollup's count needs none). */
  source: string;
  /** A rollup's function: count, sum, avg, min or max. */
  fn: string;
  /** What a formula gives: number (default), text, boolean or date. */
  result: string;
  /** Options' colors, by option. */
  colors: Record<string, string>;
  /** A regular expression short text must match. */
  pattern: string;
  /** A date starts as today on a new record. */
  today: boolean;
  /** Top-level only: a heading above the field, its width and when it shows. */
  section: string;
  width: "" | "half" | "full";
  showWhen: ShowWhen | null;
};
/** A top-level field: `key` is where its value is stored, `field.name` its label. Advanced ones are edited as JSON. */
export type FieldRow = { key: string; field: Field; advanced: boolean };

export type Schema = Record<string, unknown>;
type Props = Record<string, Record<string, unknown>>;

export const blankField: Field = {
  name: "", key: "", type: "string", required: false, unique: false, private: false, options: "", description: "", placeholder: "", default: "", min: "", max: "",
  ref: "", via: "", source: "", fn: "", result: "", colors: {}, pattern: "", today: false, section: "", width: "", showWhen: null,
};
/** Short and long text take a pattern; the formats (e-mail, URL) have their own. */
export const PATTERN_TYPES: readonly FieldType[] = ["string", "text"];

export const optionsOf = (f: Pick<Field, "options">) => f.options.split(",").map((o) => o.trim()).filter(Boolean);

/** The JSON Schema keywords holding a type's lower and upper limit, if it has them. */
export function limitKeys(type: FieldType): [string, string] | null {
  switch (type) {
    case "string": case "text": case "email": case "url": return ["minLength", "maxLength"];
    case "number": case "integer": return ["minimum", "maximum"];
    case "strings": case "references": case "objects": return ["minItems", "maxItems"];
    default: return null;
  }
}

/** Keywords the builder writes; any other keyword of a property is kept as it was. */
const RULE_KEYS = ["title", "description", "examples", "default", "minLength", "maxLength", "minimum", "maximum", "minItems", "maxItems", "pattern", UNIQUE, PRIVATE, REF, COLORS, SECTION, WIDTH, SHOW_WHEN, DEFAULT_NOW];
/** Keywords that change how the form looks, never which records are valid: saving them is safe. */
export const DISPLAY_KEYS = ["title", "description", "examples", COLORS, SECTION, WIDTH, SHOW_WHEN, DEFAULT_NOW];

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
    case "formula": return { ...(f.result === "date" ? { type: "string", format: "date" } : { type: f.result === "text" ? "string" : f.result === "boolean" ? "boolean" : "number" }), [FORMULA]: f.options.trim() };
    case "created_by": case "updated_by": return { type: "string", [AUTO]: f.type };
    // No type: a lookup shows whatever the other field holds.
    case "lookup": return { [LOOKUP]: { ref: f.via, field: f.source } };
    case "rollup": return { type: "number", [ROLLUP]: { collection: f.ref, ref: f.via, fn: f.fn, ...(f.fn !== "count" ? { field: f.source } : {}) } };
    case "file": return FILE_PROP;
    case "files": return FILES_PROP;
    default: return { type: f.type };
  }
}

/** The property a field writes, as a new one. */
export const propOf = (f: Field) => withRules(typeBase(f), f);

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
  if (f.type === "enum") {
    out.enum = optionsOf(f);
    const colors = Object.fromEntries(Object.entries(f.colors).filter(([o, c]) => (out.enum as string[]).includes(o) && OPTION_COLORS.includes(c as OptionColor)));
    if (Object.keys(colors).length) out[COLORS] = colors;
  }
  if (PATTERN_TYPES.includes(f.type) && f.pattern.trim()) out.pattern = f.pattern.trim();
  if ((f.type === "date" || f.type === "datetime") && f.today) out[DEFAULT_NOW] = "today";
  if (f.section.trim()) out[SECTION] = f.section.trim();
  if (f.width) out[WIDTH] = f.width;
  if (f.showWhen?.field) out[SHOW_WHEN] = f.showWhen.op === "filled" || f.showWhen.op === "empty" ? { field: f.showWhen.field, op: f.showWhen.op } : { ...f.showWhen };
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
  if (s[LOOKUP] && typeof s[LOOKUP] === "object") return "lookup";
  if (s[ROLLUP] && typeof s[ROLLUP] === "object") return "rollup";
  if (typeof s[FORMULA] === "string" && (s.type === "number" || s.type === "string" || s.type === "boolean")) return "formula";
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
export function canonical(v: unknown): string {
  return JSON.stringify(v, (_, x) => (x && typeof x === "object" && !Array.isArray(x) ? Object.fromEntries(Object.entries(x).sort(([a], [b]) => a.localeCompare(b))) : x));
}

export const propsOf = (schema: Schema) => (schema.properties ?? {}) as Props;
export const requiredOf = (schema: Schema) => (Array.isArray(schema.required) ? (schema.required as string[]) : []);

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
    const lookup = p?.[LOOKUP] as Record<string, unknown> | undefined;
    const rollup = p?.[ROLLUP] as Record<string, unknown> | undefined;
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
        ref: typeof p?.[REF] === "string" ? p[REF] : typeof rollup?.collection === "string" ? rollup.collection : "",
        via: typeof (lookup ?? rollup)?.ref === "string" ? String((lookup ?? rollup)!.ref) : "",
        source: typeof (lookup ?? rollup)?.field === "string" ? String((lookup ?? rollup)!.field) : "",
        fn: typeof rollup?.fn === "string" ? rollup.fn : "",
        result: type !== "formula" ? "" : p?.type === "boolean" ? "boolean" : p?.type === "string" ? (p?.format === "date" ? "date" : "text") : "number",
        colors: p?.[COLORS] && typeof p[COLORS] === "object" ? Object.fromEntries(Object.entries(p[COLORS] as Record<string, unknown>).filter(([, c]) => typeof c === "string")) as Record<string, string> : {},
        pattern: typeof p?.pattern === "string" ? p.pattern : "",
        today: p?.[DEFAULT_NOW] === "today",
        section: typeof p?.[SECTION] === "string" ? p[SECTION] : "",
        width: p?.[WIDTH] === "half" || p?.[WIDTH] === "full" ? p[WIDTH] : "",
        showWhen: showWhenOf(p?.[SHOW_WHEN]),
      },
    };
  });
}

function showWhenOf(v: unknown): ShowWhen | null {
  if (!v || typeof v !== "object") return null;
  const w = v as Record<string, unknown>;
  if (typeof w.field !== "string" || !SHOW_OPS.includes(w.op as ShowWhen["op"])) return null;
  return { field: w.field, op: w.op as ShowWhen["op"], value: w.value === undefined || w.value === null ? "" : String(w.value) };
}

/** Whether a field with this condition shows for these values; no condition always shows. */
export function shows(when: ShowWhen | null | undefined, data: unknown): boolean {
  if (!when) return true;
  const v = data && typeof data === "object" ? (data as Record<string, unknown>)[when.field] : undefined;
  const empty = v === undefined || v === null || v === "" || (Array.isArray(v) && v.length === 0);
  switch (when.op) {
    case "filled": return !empty;
    case "empty": return empty;
    case "eq": return !empty && (Array.isArray(v) ? v.map(String).includes(when.value) : String(v) === when.value);
    case "neq": return empty || (Array.isArray(v) ? !v.map(String).includes(when.value) : String(v) !== when.value);
  }
}

/** A JavaScript-valid pattern; the server (Go) refuses the few forms it lacks, such as lookahead. */
const patternOK = (p: string) => { try { new RegExp(p, "u"); return true; } catch { return false; } };

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
  if (f.type === "lookup" && (!f.via || !f.source)) return "collections.field_lookup_needed";
  if (f.type === "rollup" && (!f.ref || !f.via || !f.fn || (f.fn !== "count" && !f.source))) return "collections.field_rollup_needed";
  if (PATTERN_TYPES.includes(f.type) && f.pattern.trim() && !patternOK(f.pattern.trim())) return "collections.field_pattern_invalid";
  if (f.showWhen) {
    if (!f.showWhen.field) return "collections.field_show_needed";
    if ((f.showWhen.op === "eq" || f.showWhen.op === "neq") && !f.showWhen.value.trim()) return "collections.field_show_needed";
    // A hidden field cannot be filled, so the form could never be saved.
    if (f.required) return "collections.field_show_required";
  }
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

/** Copies a field just below itself, under a free key and "(copy)" name; a copy is never unique. */
export function duplicateField(schema: Schema, key: string, suffix: string): Schema {
  const entries = Object.entries(propsOf(schema));
  const i = entries.findIndex(([k]) => k === key);
  if (i < 0) return schema;
  let to = `${key}_2`.slice(0, 64);
  for (let n = 3; key && to in propsOf(schema); n++) to = `${key}_${n}`.slice(0, 64);
  const prop = { ...(entries[i][1] as Record<string, unknown>) };
  delete prop[UNIQUE];
  if (typeof prop.title === "string") prop.title = `${prop.title} ${suffix}`;
  entries.splice(i + 1, 0, [to, prop]);
  return rebuild(schema, entries, requiredOf(schema));
}

/** Adds a ready-made property (a template) under a free key. */
export function addProp(schema: Schema, key: string, prop: Record<string, unknown>): Schema {
  let to = key;
  for (let n = 2; to in propsOf(schema); n++) to = `${key}_${n}`;
  return rebuild(schema, [...Object.entries(propsOf(schema)), [to, prop]], requiredOf(schema));
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

export type SchemaChange = { kind: "added" | "removed" | "required" | "unique" | "type" | "rules" | "display"; name: string; risky: boolean };

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
    const look = (p: Record<string, unknown>) => canonical(Object.fromEntries(DISPLAY_KEYS.map((k) => [k, p?.[k]])));
    const rules = (p: Record<string, unknown>) => ({ ...strip(p), ...Object.fromEntries(DISPLAY_KEYS.map((k) => [k, undefined])) });
    const [was, now] = [strip(propsOf(original)[r.key]), strip(propsOf(next)[r.key])];
    if (r.advanced !== old.advanced || r.field.type !== old.field.type) out.push({ kind: "type", name: r.field.name, risky: true });
    else if ((r.field.type === "group" || r.field.type === "objects") && !r.advanced) {
      // A group (or a list's item) reports its subfields by name; its own rules apart from them.
      const inner = (p: Record<string, unknown>) => (r.field.type === "objects" ? (p.items ?? {}) as Record<string, unknown> : p);
      const bare = (p: Record<string, unknown>) => canonical(r.field.type === "objects" ? { ...p, items: { ...inner(p), properties: undefined, required: undefined } } : { ...p, properties: undefined, required: undefined });
      if (bare(was) !== bare(now)) out.push({ kind: "rules", name: r.field.name, risky: true });
      for (const c of schemaChanges(inner(was), inner(now))) out.push({ ...c, name: `${r.field.name} › ${c.name}` });
    } else if (canonical(rules(propsOf(original)[r.key])) !== canonical(rules(propsOf(next)[r.key]))) out.push({ kind: "rules", name: r.field.name, risky: true });
    else if (look(propsOf(original)[r.key]) !== look(propsOf(next)[r.key])) out.push({ kind: "display", name: r.field.name, risky: false });
  }
  const kept = new Set(after.map((r) => r.key));
  for (const r of before.values()) if (!kept.has(r.key)) out.push({ kind: "removed", name: r.field.name, risky: true });
  return out;
}

