import { addField, blankField, fieldsOf, keyOf, propOf, propsOf, type Field, type FieldType } from "@/components/field-builder";
import { parseImport, type ParsedFile, type ParsedRecord } from "@/lib/import-file";

/**
 * Our export writes ' before a cell starting with = + - @ tab or CR, so a
 * spreadsheet does not run it as a formula (mapper.CSVCell); reading it back drops that '.
 * shortcut: a value really written as '-x comes back as -x; rare enough to accept.
 */
const unguard = (cell: string) => (/^'[=+\-@\t\r]/.test(cell) ? cell.slice(1) : cell);

/**
 * Splits CSV text into rows of cells, fed in pieces of any size: quoted cells may
 * hold commas, quotes ("") and line breaks. The separator is whichever of , and ;
 * the header line uses more. Rows with no filled cell are dropped.
 */
export class CsvReader {
  private row: string[] = [];
  private cell = "";
  private quoted = false;
  private pendingQuote = false; // a " closing a quoted cell, unless the next char is another "
  private pendingCR = false;
  private sep: string | null = null;
  private head = "";
  private started = false;

  /** Rows completed by this piece of text. */
  push(text: string): string[][] {
    if (!this.started) {
      text = text.replace(/^\uFEFF/, "");
      this.started = text.length > 0;
    }
    if (this.sep === null) {
      this.head += text;
      const nl = this.head.search(/\r|\n/);
      if (nl < 0) return [];
      const line = this.head.slice(0, nl);
      this.sep = (line.match(/;/g)?.length ?? 0) > (line.match(/,/g)?.length ?? 0) ? ";" : ",";
      text = this.head;
      this.head = "";
    }
    return this.scan(text);
  }

  /** The last row, once the text has ended. */
  end(): string[][] {
    const out = this.sep === null ? ((this.sep = this.head.includes(";") && !this.head.includes(",") ? ";" : ","), this.scan(this.head)) : [];
    this.pendingQuote = false;
    this.row.push(this.cell);
    this.cell = "";
    if (this.row.some((v) => v !== "")) out.push(this.row.map(unguard));
    this.row = [];
    return out;
  }

  private scan(text: string): string[][] {
    const out: string[][] = [];
    const sep = this.sep!;
    let start = 0; // plain runs are copied in one slice, not char by char
    const flush = (i: number) => { if (i > start) this.cell += text.slice(start, i); };
    for (let i = 0; i < text.length; i++) {
      const c = text[i];
      if (this.pendingCR) {
        this.pendingCR = false;
        if (c === "\n") { start = i + 1; continue; }
      }
      if (this.pendingQuote) {
        this.pendingQuote = false;
        if (c === '"') { this.cell += '"'; start = i + 1; continue; } // "" inside quotes
        this.quoted = false;
      }
      if (this.quoted) {
        if (c === '"') { flush(i); this.pendingQuote = true; start = i + 1; }
        continue;
      }
      if (c === '"') { flush(i); this.quoted = true; start = i + 1; }
      else if (c === sep) { flush(i); this.row.push(this.cell); this.cell = ""; start = i + 1; }
      else if (c === "\n" || c === "\r") {
        flush(i);
        this.row.push(this.cell); this.cell = "";
        if (this.row.some((v) => v !== "")) out.push(this.row.map(unguard));
        this.row = [];
        if (c === "\r") this.pendingCR = true;
        start = i + 1;
      }
    }
    flush(text.length);
    return out;
  }
}

/** Splits CSV text into rows of cells: quoted cells may hold commas, quotes ("") and line breaks. */
export function parseCsv(text: string): string[][] {
  const reader = new CsvReader();
  return [...reader.push(text), ...reader.end()];
}

const DATE = /^\d{4}-\d{2}-\d{2}$/;
const DATETIME = /^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})$/;
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const URL_RE = /^https?:\/\/\S+$/;

/** CSV exports store objects and lists as JSON inside a quoted cell. */
export function structuredCell(cell: string): unknown {
  if (/^\s*[\[{]/.test(cell)) {
    try { return JSON.parse(cell); } catch { /* Ordinary text stays text. */ }
  }
  return cell;
}

/** What a run of scalar values (all one JS type) has looked like so far. */
class Scalars {
  n = 0;
  bool = true;
  num = true;
  int = true;
  date = true;
  datetime = true;
  email = true;
  url = true;
  str = true;
  long = false;
  constructor(private fromText: boolean) {}

  add(v: unknown) {
    if (v === "") return;
    this.n++;
    const s = typeof v === "string" ? v : null;
    if (this.bool) this.bool = typeof v === "boolean" || (this.fromText && s !== null && /^(true|false)$/i.test(s));
    if (this.num) {
      const n = typeof v === "number" ? v : this.fromText && s !== null && /^-?\d+([.,]\d+)?$/.test(s) ? Number(s.replace(",", ".")) : NaN;
      this.num = Number.isFinite(n);
      if (this.num && this.int) this.int = Number.isInteger(n);
    }
    if (this.date) this.date = s !== null && DATE.test(s);
    if (this.datetime) this.datetime = s !== null && DATETIME.test(s);
    if (this.email) this.email = s !== null && EMAIL.test(s);
    if (this.url) this.url = s !== null && URL_RE.test(s);
    if (this.str) this.str = s !== null;
    if (!this.long && s !== null) this.long = s.length >= 200 || s.includes("\n");
  }

  /** The narrowest builder type every value fits; text read from CSV is matched by its look. */
  type(): FieldType {
    if (!this.n) return "string";
    if (this.bool) return "boolean";
    if (this.num) return this.int ? "integer" : "number";
    if (this.date) return "date";
    if (this.datetime) return "datetime";
    if (this.email) return "email";
    if (this.url) return "url";
    if (this.str && this.long) return "text";
    return "string";
  }
}

const isObject = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
type Kind = "object" | "array" | "string" | "number" | "boolean" | "bigint" | "symbol" | "undefined" | "function";

/**
 * Everything seen at one place of the records (a field, a subfield, a list's items),
 * folded one value at a time, so a file of any size is read once and kept as a summary.
 */
class Shape {
  private nulls = false;
  private readonly kinds = new Map<Kind, Scalars | Map<string, Shape> | { items: Shape | null }>();
  /** root: the records themselves, whose fields keep fromText (CSV cells); deeper JSON has native types. */
  constructor(private readonly fromText: boolean, private readonly root = false) {}

  add(v: unknown) {
    if (v === undefined || (this.fromText && v === "")) return;
    if (v === null) { this.nulls = true; return; }
    const kind = (Array.isArray(v) ? "array" : typeof v) as Kind;
    let acc = this.kinds.get(kind);
    if (kind === "object") {
      // JSON inside a CSV cell already has native types: its strings are not coerced.
      const fields = (acc ?? new Map<string, Shape>()) as Map<string, Shape>;
      for (const [k, x] of Object.entries(v as Record<string, unknown>)) {
        if (!k) continue;
        let f = fields.get(k);
        if (!f) fields.set(k, (f = new Shape(this.root && this.fromText)));
        f.add(x);
      }
      acc = fields;
    } else if (kind === "array") {
      const list = (acc ?? { items: null }) as { items: Shape | null };
      for (const x of v as unknown[]) (list.items ??= new Shape(false)).add(x);
      acc = list;
    } else {
      acc ??= new Scalars(this.fromText);
      (acc as Scalars).add(v);
    }
    this.kinds.set(kind, acc);
  }

  /** The JSON Schema property for what was seen; title names a top-level field. */
  property(title?: string): Record<string, unknown> {
    const label = title === undefined ? {} : { title };
    const parts = [...this.kinds.entries()].map(([kind, acc]) => this.part(kind, acc));
    let property: Record<string, unknown>;
    if (parts.length > 1) property = { ...label, anyOf: parts };
    else if (parts.length === 1 && "type" in parts[0] && (parts[0].type === "object" || parts[0].type === "array")) property = { ...label, ...parts[0] };
    else {
      const type = parts.length ? ([...this.kinds.values()][0] as Scalars).type() : "string";
      property = propOf({ ...blankField, name: title ?? "", type });
      if (title === undefined) delete property.title;
    }
    // A null inside an object or list is a value, not an absent CSV cell.
    if (this.nulls) {
      // Only nulls seen: text that also takes null, so later records can fill it.
      if (typeof property.type === "string") property.type = [property.type, "null"];
      else (property.anyOf as Record<string, unknown>[]).push({ type: "null" });
    }
    return property;
  }

  private part(kind: Kind, acc: Scalars | Map<string, Shape> | { items: Shape | null }): Record<string, unknown> {
    if (kind === "object") {
      const properties = Object.fromEntries([...(acc as Map<string, Shape>)].map(([k, f]) => [k, f.property(k)]));
      return { type: "object", ...(Object.keys(properties).length ? { properties } : {}) };
    }
    if (kind === "array") {
      const items = (acc as { items: Shape | null }).items;
      return { type: "array", ...(items ? { items: items.property() } : {}) };
    }
    const p = propOf({ ...blankField, name: "", type: (acc as Scalars).type() });
    delete p.title;
    return p;
  }
}

/** Fields guessed from records fed one at a time: keys stay as the file writes them. */
export class SchemaInferrer {
  private readonly root: Shape;
  constructor(fromText: boolean) { this.root = new Shape(fromText, true); }
  /** A record (CSV rows arrive as objects of their cells). */
  add(record: unknown) { if (isObject(record)) this.root.add(record); }
  /** Keys known without a value (a CSV header with no rows). */
  keys(keys: string[]) { this.root.add(Object.fromEntries(keys.map((k) => [k, undefined]))); }
  schema(): Record<string, unknown> | null {
    const p = this.root.property();
    const properties = (p.properties ?? {}) as Record<string, unknown>;
    return Object.keys(properties).length ? { type: "object", properties } : null;
  }
}

/** A CSV row as a record of its cells, for the inferrer. */
export const csvRow = (header: string[], row: string[]) => Object.fromEntries(header.map((h, i) => [h.trim(), structuredCell(row[i] ?? "")]));

/**
 * Fields guessed from a data file: JSON (one array or JSON Lines) or CSV with a
 * header line. Keys stay as the file writes them, so its records import as they are.
 * Null when nothing there reads as records.
 */
export function schemaFromFile(name: string, text: string): Record<string, unknown> | null {
  if (/\.csv$/i.test(name)) {
    const [header, ...rows] = parseCsv(text);
    if (!header?.length) return null;
    const inf = new SchemaInferrer(true);
    inf.keys(header.map((h) => h.trim()));
    for (const r of rows) inf.add(csvRow(header, r));
    return inf.schema();
  }
  const inf = new SchemaInferrer(false);
  for (const r of parseImport(text).records) inf.add(r.data);
  return inf.schema();
}

/** Ready-made field sets; names come from the caller's language, keys from the names. */
export function templateSchema(fields: Partial<Field>[]): Record<string, unknown> {
  return fields.reduce<Record<string, unknown>>((schema, f) => addField(schema, { ...blankField, ...f, key: keyOf(f.name ?? "") }), { type: "object" });
}

const TEXT_TYPES: FieldType[] = ["string", "text", "enum", "date", "datetime", "email", "url", "reference"];

/** A CSV cell as the field's type stores it; what does not read as the type stays text, for the server to report. */
function cellValue(cell: string, type: FieldType | "structured" | undefined, nullable = false): unknown {
  if (nullable && cell.trim() === "null") return null;
  // A union accepting plain text and JSON must keep plain text as written.
  if (type === "structured") return structuredCell(cell);
  // A spreadsheet date arrives as days since 1899-12-30.
  if ((type === "date" || type === "datetime") && /^\d+(\.\d+)?$/.test(cell.trim())) {
    const iso = new Date(Date.UTC(1899, 11, 30) + Math.round(Number(cell) * 86_400_000)).toISOString();
    return type === "date" ? iso.slice(0, 10) : iso.replace(".000Z", "Z");
  }
  if (!type || TEXT_TYPES.includes(type)) return cell;
  const v = cell.trim();
  if (type === "boolean") {
    const b = v.toLowerCase();
    return ["true", "sim", "yes", "1", "verdadeiro"].includes(b) ? true : ["false", "não", "nao", "no", "0", "falso"].includes(b) ? false : cell;
  }
  if (type === "number" || type === "integer") {
    const n = Number(/^-?\d+,\d+$/.test(v) ? v.replace(",", ".") : v);
    return v !== "" && Number.isFinite(n) ? n : cell;
  }
  // Lists, groups, files and JSON: the export writes them as JSON; a list may also be "a; b".
  try {
    return JSON.parse(v);
  } catch {
    return type === "strings" && !/^[\[{]/.test(v) ? v.split(";").map((x) => x.trim()).filter(Boolean) : cell;
  }
}

/**
 * The field each column of a table fills, by the header naming its key or
 * label; a header no field matches keeps its own text (the server reports it).
 */
export function guessColumns(header: string[], schema: Record<string, unknown> | null): string[] {
  const byName = new Map<string, string>();
  for (const r of fieldsOf(schema)) byName.set(r.field.name.trim().toLowerCase(), r.key);
  for (const r of fieldsOf(schema)) byName.set(r.key.toLowerCase(), r.key);
  return header.map((h) => byName.get(h.trim().toLowerCase()) ?? h.trim());
}

/** Storage types can be declared by nullable types, alternatives or local refs. */
function storageTypes(property: unknown, root: unknown, seen = new Set<unknown>()): string[] {
  if (!isObject(property) || seen.has(property)) return [];
  seen.add(property);
  const types: string[] = typeof property.type === "string" ? [property.type] : Array.isArray(property.type) ? property.type.filter((t): t is string => typeof t === "string") : [];
  if (typeof property.$ref === "string" && property.$ref.startsWith("#")) {
    const pointer = decodeURIComponent(property.$ref.slice(1));
    const target = pointer === "" ? root : pointer.startsWith("/") ? pointer.slice(1).split("/").reduce<unknown>((v, k) => isObject(v) ? v[k.replace(/~1/g, "/").replace(/~0/g, "~")] : undefined, root) : undefined;
    types.push(...storageTypes(target, root, seen));
  }
  for (const keyword of ["anyOf", "oneOf", "allOf"]) {
    if (Array.isArray(property[keyword])) for (const sub of property[keyword]) types.push(...storageTypes(sub, root, seen));
  }
  return types;
}

/**
 * Turns table rows (CSV or a sheet) into records: columns maps each column to a
 * field key ("" skips it), guessed from the header when absent. Empty cells are left out.
 */
export function rowReader(header: string[], schema: Record<string, unknown> | null, columns?: string[]): (row: string[], line: number) => ParsedRecord {
  const keys = columns ?? guessColumns(header, schema);
  const props = propsOf(schema ?? {});
  const types = new Map(fieldsOf(schema).map((r) => {
    // The builder falls back to text for advanced schemas. The stored JSON
    // type still tells the importer to decode an object or array.
    const storage = storageTypes(props[r.key], schema);
    let type: FieldType | "structured" = r.field.type;
    if (r.advanced) {
      if (storage.includes("object") || storage.includes("array")) type = storage.includes("string") ? "structured" : "json";
      else if (storage.includes("integer")) type = "integer";
      else if (storage.includes("number")) type = "number";
      else if (storage.includes("boolean")) type = "boolean";
      else if (storage.includes("null")) type = "json";
    }
    return [r.key, { type, nullable: storage.includes("null") }];
  }));
  return (row, line) => ({
    line,
    data: Object.fromEntries(keys.flatMap((k, j) => (row[j] === undefined || row[j] === "" || !k ? [] : [[k, cellValue(row[j], types.get(k)?.type, types.get(k)?.nullable)]]))),
  });
}

/** Records from table rows whose first row is the header; see rowReader. */
export function tableRecords(rows: string[][], schema: Record<string, unknown> | null, columns?: string[]): ParsedFile {
  const [header = [], ...body] = rows;
  const read = rowReader(header, schema, columns);
  return { errors: [], records: body.map((row, i) => read(row, i + 2)) };
}

/** Records from a CSV with a header line, as the export writes it. */
export const csvRecords = (text: string, schema: Record<string, unknown> | null): ParsedFile => tableRecords(parseCsv(text), schema);
