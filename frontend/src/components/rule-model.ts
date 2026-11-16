// The record conditions' model: what a rule holds, how each kind of field
// compares, and the editor's drafts. Shared by the visual and the text editor.
import { VIRTUAL_TYPES, type Field, type FieldRow, type FieldType } from "@/components/field-builder";

export type RuleOp = "eq" | "neq" | "contains" | "not_contains" | "starts_with" | "gt" | "gte" | "lt" | "lte" | "between" | "empty" | "not_empty" | "in" | "not_in" | "has" | "has_any";
export interface Rule { field: string; op: RuleOp; value?: unknown }
/** Rules joined by AND (any: false) or OR; groups bracket rules of their own, one level deep. */
export interface Where { any: boolean; rules: Rule[]; groups?: { any: boolean; rules: Rule[] }[] }

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

export const needsValue = (op: EditOp) => op !== "empty" && op !== "not_empty";
export const LIST_OPS: EditOp[] = ["in", "not_in", "has_any"];

/** A rule as the editor holds it: text while typed, "@n" for today plus n days, picked items in list. */
export type DraftRule = { field: string; op: EditOp; raw: string; raw2: string; list: string[] };

export const todayOf = (v: unknown): number | undefined => {
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
export type Entry = {
  key: string; name: string; kind: Kind; type: FieldType;
  /** An option list's choices and their colors. */
  options: string[]; colors: Record<string, string>;
  /** A reference's collection. */
  target: string;
  /** Stored in the record (not a lookup, rollup or the record's own date). */
  stored: boolean;
};

// Groups and free JSON have no single value to compare.
export function entriesOf(rows: FieldRow[], t: (k: "rules.created_at" | "rules.updated_at") => string): Entry[] {
  const fields = rows.filter((r) => !r.advanced && r.field.type !== "group" && r.field.type !== "json").map((r): Entry => ({
    key: r.key, name: r.field.name, kind: kindOf(r.field), type: r.field.type,
    options: r.field.type === "enum" ? r.field.options.split(",").map((o) => o.trim()).filter(Boolean) : [], colors: r.field.colors ?? {},
    target: r.field.type === "reference" || r.field.type === "references" ? r.field.ref : "",
    stored: !VIRTUAL_TYPES.includes(r.field.type),
  }));
  return [...fields,
    { key: "$created_at", name: t("rules.created_at"), kind: "date", type: "datetime", options: [], colors: {}, target: "", stored: false },
    { key: "$updated_at", name: t("rules.updated_at"), kind: "date", type: "datetime", options: [], colors: {}, target: "", stored: false }];
}

