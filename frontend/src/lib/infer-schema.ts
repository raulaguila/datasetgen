import { addField, blankField, keyOf, type Field, type FieldType } from "@/components/field-builder";
import { parseImport } from "@/lib/import-file";

/** How many records are read to guess each field's type. */
const SAMPLE = 200;

/** Splits CSV text into rows of cells: quoted cells may hold commas, quotes ("") and line breaks. */
export function parseCsv(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let cell = "";
  let quoted = false;
  const body = text.replace(/^﻿/, "");
  // The separator is whichever of , and ; the header line uses more.
  const header = body.split(/\r?\n/, 1)[0] ?? "";
  const sep = (header.match(/;/g)?.length ?? 0) > (header.match(/,/g)?.length ?? 0) ? ";" : ",";
  for (let i = 0; i < body.length; i++) {
    const c = body[i];
    if (quoted) {
      if (c === '"' && body[i + 1] === '"') { cell += '"'; i++; }
      else if (c === '"') quoted = false;
      else cell += c;
    } else if (c === '"') quoted = true;
    else if (c === sep) { row.push(cell); cell = ""; }
    else if (c === "\n" || c === "\r") {
      if (c === "\r" && body[i + 1] === "\n") i++;
      row.push(cell); cell = "";
      if (row.some((v) => v !== "")) rows.push(row);
      row = [];
    } else cell += c;
  }
  row.push(cell);
  if (row.some((v) => v !== "")) rows.push(row);
  return rows;
}

const DATE = /^\d{4}-\d{2}-\d{2}$/;
const DATETIME = /^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})$/;
const EMAIL = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const URL_RE = /^https?:\/\/\S+$/;

/** The narrowest builder type every sample fits; text read from CSV is matched by its look. */
function typeOfValues(values: unknown[], fromText: boolean): FieldType {
  const present = values.filter((v) => v !== null && v !== undefined && v !== "");
  if (!present.length) return "string";
  const all = (test: (v: unknown) => boolean) => present.every(test);
  const str = (re: RegExp) => (v: unknown) => typeof v === "string" && re.test(v);
  if (all((v) => typeof v === "boolean" || (fromText && typeof v === "string" && /^(true|false)$/i.test(v)))) return "boolean";
  const num = (v: unknown) => (typeof v === "number" ? v : fromText && typeof v === "string" && /^-?\d+([.,]\d+)?$/.test(v) ? Number(v.replace(",", ".")) : NaN);
  if (all((v) => Number.isFinite(num(v)))) return all((v) => Number.isInteger(num(v))) ? "integer" : "number";
  if (all((v) => Array.isArray(v) && v.every((x) => typeof x === "string"))) return "strings";
  if (all((v) => !!v && typeof v === "object")) return "json";
  if (all(str(DATE))) return "date";
  if (all(str(DATETIME))) return "datetime";
  if (all(str(EMAIL))) return "email";
  if (all(str(URL_RE))) return "url";
  if (all((v) => typeof v === "string") && present.some((v) => (v as string).length >= 200 || (v as string).includes("\n"))) return "text";
  return "string";
}

/**
 * Fields guessed from a data file: JSON (one array or JSON Lines) or CSV with a
 * header line. Keys stay as the file writes them, so its records import as they are.
 * Null when nothing there reads as records.
 */
export function schemaFromFile(name: string, text: string): Record<string, unknown> | null {
  let records: Record<string, unknown>[];
  let fromText = false;
  if (/\.csv$/i.test(name)) {
    const [header, ...rows] = parseCsv(text);
    if (!header?.length) return null;
    records = rows.slice(0, SAMPLE).map((r) => Object.fromEntries(header.map((h, i) => [h.trim(), r[i] ?? ""])));
    if (!records.length) records = [Object.fromEntries(header.map((h) => [h.trim(), ""]))];
    fromText = true;
  } else {
    records = parseImport(text).records.slice(0, SAMPLE).map((r) => r.data).filter((d): d is Record<string, unknown> => !!d && typeof d === "object" && !Array.isArray(d));
  }
  const keys: string[] = [];
  for (const r of records) for (const k of Object.keys(r)) if (k && !keys.includes(k)) keys.push(k);
  if (!keys.length) return null;
  return keys.reduce<Record<string, unknown>>((schema, key) => {
    const type = typeOfValues(records.map((r) => r[key]), fromText);
    const f: Field = { ...blankField, name: key, key, type };
    return addField(schema, f);
  }, { type: "object" });
}

/** Ready-made field sets; names come from the caller's language, keys from the names. */
export function templateSchema(fields: Partial<Field>[]): Record<string, unknown> {
  return fields.reduce<Record<string, unknown>>((schema, f) => addField(schema, { ...blankField, ...f, key: keyOf(f.name ?? "") }), { type: "object" });
}
