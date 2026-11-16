// Conditions written as text, as in Jira: status = pago e valor > 100 ou
// (cliente = "Ana Prado" e vencimento < hoje). Parsed into the same rules the
// visual editor holds, and written back from them, so either can edit a filter.
// Portuguese and English words are both read; text is written in the reader's language.
import { opsFor, todayOf, type EditOp, type Entry, type Kind, type Rule, type Where } from "@/components/rule-model";

export type Lang = "pt" | "en";

export type TextErrorKey =
  | "unexpected" | "field" | "op" | "value" | "number" | "date" | "boolean" | "option"
  | "list" | "paren" | "nested" | "join" | "days" | "too_many" | "ref_none" | "ref_many";
/** Where the text went wrong: a message key, its words, and the character it starts at. */
export type TextError = { key: TextErrorKey; params: Record<string, string>; at: number };

/** A reference written by name: the caller looks it up and puts the id in its place. */
export type RefName = { name: string; target: string; field: string };
const REF_NAME = "$name";
export const refNameOf = (v: unknown): string | undefined =>
  v && typeof v === "object" && !Array.isArray(v) && typeof (v as Record<string, unknown>)[REF_NAME] === "string" ? (v as Record<string, string>)[REF_NAME] : undefined;

type Tok = { k: "str" | "num" | "word" | "sym" | "date" | "today"; v: string; at: number; end: number };

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const MAX_RULES = 20;
const MAX_GROUPS = 10;

class Fail extends Error {
  constructor(public e: TextError) { super(e.key); }
}
const fail = (key: TextErrorKey, at: number, params: Record<string, string> = {}): never => { throw new Fail({ key, params, at }); };

/** Splits the text into words, quoted texts, numbers, dates, today ± n and symbols. */
function tokenize(src: string, lenient = false): Tok[] {
  const out: Tok[] = [];
  let i = 0;
  while (i < src.length) {
    const rest = src.slice(i);
    const ws = /^\s+/.exec(rest);
    if (ws) { i += ws[0].length; continue; }
    if (rest[0] === '"') {
      let j = 1, v = "";
      while (j < rest.length && rest[j] !== '"') { if (rest[j] === "\\" && j + 1 < rest.length) j++; v += rest[j]; j++; }
      if (j >= rest.length) { if (lenient) { out.push({ k: "str", v, at: i, end: src.length }); break; } fail("unexpected", i, { x: '"' }); }
      out.push({ k: "str", v, at: i, end: i + j + 1 }); i += j + 1; continue;
    }
    const sym = /^(!=|>=|<=|!~|[=<>~(),])/.exec(rest);
    if (sym) { out.push({ k: "sym", v: sym[1], at: i, end: i + sym[1].length }); i += sym[1].length; continue; }
    const date = /^\d{4}-\d{2}-\d{2}(?![\p{L}\p{N}])/u.exec(rest);
    if (date) { out.push({ k: "date", v: date[0], at: i, end: i + 10 }); i += 10; continue; }
    const today = /^(hoje|today)(?:\s*([+-])\s*(\d+))?(?![\p{L}\p{N}_])/iu.exec(rest);
    if (today) { out.push({ k: "today", v: today[2] ? `${today[2] === "-" ? "-" : ""}${today[3]}` : "0", at: i, end: i + today[0].length }); i += today[0].length; continue; }
    const near = /^(ontem|yesterday|amanh[ãa]|tomorrow)(?![\p{L}\p{N}_])/iu.exec(rest);
    if (near) { out.push({ k: "today", v: /^(ontem|yesterday)$/i.test(near[1]) ? "-1" : "1", at: i, end: i + near[0].length }); i += near[0].length; continue; }
    const num = /^-?\d+(?:\.\d+)?(?![\p{L}_])/u.exec(rest);
    if (num) { out.push({ k: "num", v: num[0], at: i, end: i + num[0].length }); i += num[0].length; continue; }
    const word = /^[\p{L}\p{N}_$.\-@]+/u.exec(rest);
    if (word) { out.push({ k: "word", v: word[0], at: i, end: i + word[0].length }); i += word[0].length; continue; }
    if (lenient) { i++; continue; }
    fail("unexpected", i, { x: rest[0] });
  }
  return out;
}

const low = (s: string) => s.toLocaleLowerCase();
const isWord = (t: Tok | undefined, ...words: string[]) => !!t && t.k === "word" && words.includes(low(t.v));
const AND = ["e", "and"], OR = ["ou", "or"];
const TRUE = ["sim", "yes", "true", "verdadeiro"], FALSE = ["não", "nao", "no", "false", "falso"];
const EMPTY = ["vazio", "vazia", "empty"];

/** The field a run of words names: the longest that matches a name or key. */
function matchField(toks: Tok[], p: number, entries: Entry[]): { entry: Entry; next: number } | null {
  const t = toks[p];
  if (!t) return null;
  const byName = (name: string) => entries.find((e) => low(e.name) === low(name)) ?? entries.find((e) => e.key === name);
  if (t.k === "str") { const e = byName(t.v); return e ? { entry: e, next: p + 1 } : null; }
  if (t.k !== "word") return null;
  for (let n = Math.min(6, toks.length - p); n >= 1; n--) {
    const run = toks.slice(p, p + n);
    if (run.some((x) => x.k !== "word")) continue;
    const e = byName(run.map((x) => x.v).join(" "));
    if (e) return { entry: e, next: p + n };
  }
  return null;
}

/** Reads the operator words after a field; null when they are not one. */
function matchOp(toks: Tok[], p: number): { op: EditOp; next: number } | null {
  const t = toks[p], u = toks[p + 1], w = toks[p + 2];
  if (!t) return null;
  if (t.k === "sym") {
    const op = ({ "=": "eq", "!=": "neq", ">": "gt", ">=": "gte", "<": "lt", "<=": "lte", "~": "contains", "!~": "not_contains" } as Record<string, EditOp>)[t.v];
    if (!op) return null;
    // "= vazio" and "!= vazio" read as empty and not empty.
    if ((op === "eq" || op === "neq") && isWord(u, ...EMPTY)) return { op: op === "eq" ? "empty" : "not_empty", next: p + 2 };
    return { op, next: p + 1 };
  }
  if (t.k !== "word") return null;
  const a = low(t.v);
  if (["contém", "contem", "contains"].includes(a)) return { op: "contains", next: p + 1 };
  if (["não", "nao", "not"].includes(a)) {
    if (isWord(u, "contém", "contem", "contains")) return { op: "not_contains", next: p + 2 };
    if (isWord(u, "em", "in")) return { op: "not_in", next: p + 2 };
    if (isWord(u, "é", "eh", "is") && isWord(w, ...EMPTY)) return { op: "not_empty", next: p + 3 };
    if (isWord(u, "é", "eh", "is")) return { op: "neq", next: p + 2 };
    return null;
  }
  if (["começa", "comeca"].includes(a) && isWord(u, "com")) return { op: "starts_with", next: p + 2 };
  if (a === "starts" && isWord(u, "with")) return { op: "starts_with", next: p + 2 };
  if (["em", "in"].includes(a)) {
    if (a === "in" && isWord(u, "last", "next")) return { op: low(u!.v) === "last" ? "last_days" : "next_days", next: p + 2 };
    if (a === "in" && isWord(u, "the") && isWord(w, "last", "next")) return { op: low(w!.v) === "last" ? "last_days" : "next_days", next: p + 3 };
    return { op: "in", next: p + 1 };
  }
  if (["tem", "has", "inclui", "includes"].includes(a)) {
    if (isWord(u, "algum", "any")) return { op: "has_any", next: isWord(w, "de", "of") ? p + 3 : p + 2 };
    return { op: "has", next: p + 1 };
  }
  if (["entre", "between"].includes(a)) return { op: "between", next: p + 1 };
  if (["é", "eh", "is"].includes(a)) {
    if (isWord(u, ...EMPTY)) return { op: "empty", next: p + 2 };
    if (isWord(u, "not") && isWord(w, ...EMPTY)) return { op: "not_empty", next: p + 3 };
    if (isWord(u, "not")) return { op: "neq", next: p + 2 };
    return { op: "eq", next: p + 1 };
  }
  if (a === "nos" && isWord(u, "últimos", "ultimos")) return { op: "last_days", next: p + 2 };
  if (a === "nos" && isWord(u, "próximos", "proximos")) return { op: "next_days", next: p + 2 };
  if (["últimos", "ultimos", "last"].includes(a)) return { op: "last_days", next: p + 1 };
  if (["próximos", "proximos", "next"].includes(a)) return { op: "next_days", next: p + 1 };
  return null;
}

/** One value as the field's kind holds it. */
function typed(t: Tok, entry: Entry, src: string): unknown {
  const kind: Kind = entry.kind;
  const text = t.v;
  if (t.k === "sym") fail("value", t.at, { field: entry.name });
  if (kind === "number") {
    const n = Number(text);
    if (t.k === "str" || t.k === "date" || t.k === "today" || !Number.isFinite(n)) fail("number", t.at, { field: entry.name, x: text });
    return n;
  }
  if (kind === "date") {
    if (t.k === "today") return { today: Number(text) };
    if (t.k === "date" || (t.k === "str" && /^\d{4}-\d{2}-\d{2}/.test(text))) return text;
    fail("date", t.at, { field: entry.name, x: text });
  }
  if (kind === "boolean") {
    if (TRUE.includes(low(text))) return true;
    if (FALSE.includes(low(text))) return false;
    fail("boolean", t.at, { field: entry.name, x: text });
  }
  if (kind === "enum") {
    const option = entry.options.find((o) => low(o) === low(text));
    if (!option) fail("option", t.at, { field: entry.name, x: text, options: entry.options.join(", ") });
    return option;
  }
  if ((kind === "ref" || kind === "refs") && !UUID.test(text)) return { [REF_NAME]: text };
  return t.k === "today" ? src.slice(t.at, t.end) : text;
}

type Node = { t: "cond"; rule: Rule; at: number } | { t: "and" | "or"; items: Node[]; paren: boolean; at: number };

/**
 * Reads the text into rules. References written by name come back as
 * {"$name": "..."} for the caller to look up (refNames lists them).
 */
export function parseRuleText(src: string, entries: Entry[]): { where: Where; refNames: RefName[] } | { error: TextError } {
  try {
    const toks = tokenize(src);
    let p = 0;
    const refNames: RefName[] = [];
    const value = (entry: Entry) => {
      const t = toks[p];
      if (!t || (t.k === "word" && [...AND, ...OR].includes(low(t.v))) || t.k === "sym") fail("value", t?.at ?? src.length, { field: entry.name });
      p++;
      const v = typed(t, entry, src);
      const name = refNameOf(v);
      if (name !== undefined) refNames.push({ name, target: entry.target, field: entry.key });
      return v;
    };
    const cond = (): Node => {
      const at = toks[p]?.at ?? src.length;
      const f = matchField(toks, p, entries);
      if (!f) fail(toks[p] ? "field" : "value", at, { x: toks[p]?.v ?? "" });
      const { entry } = f!;
      p = f!.next;
      const o = matchOp(toks, p);
      if (!o) fail("op", toks[p]?.at ?? src.length, { field: entry.name, x: toks[p]?.v ?? "" });
      let op = o!.op;
      p = o!.next;
      // Lists read "=" as includes and "em" as includes any.
      if (entry.kind === "strings" || entry.kind === "refs") op = op === "eq" ? "has" : op === "in" ? "has_any" : op;
      if (!opsFor(entry.kind).includes(op)) fail("op", toks[o!.next - 1].at, { field: entry.name, x: toks.slice(o!.next - 1, o!.next).map((x) => x.v).join(" ") });
      const field = entry.key;
      if (op === "empty" || op === "not_empty") return { t: "cond", rule: { field, op }, at };
      if (op === "last_days" || op === "next_days") {
        const t = toks[p];
        if (!t || t.k !== "num" || !Number.isInteger(Number(t.v)) || Number(t.v) < 0) fail("days", t?.at ?? src.length, { field: entry.name });
        p++;
        if (isWord(toks[p], "dias", "dia", "days", "day")) p++;
        const n = Number(t!.v);
        return { t: "cond", rule: { field, op: "between", value: op === "last_days" ? [{ today: -n }, { today: 0 }] : [{ today: 0 }, { today: n }] }, at };
      }
      if (op === "between") {
        const a = value(entry);
        if (!isWord(toks[p], ...AND)) fail("value", toks[p]?.at ?? src.length, { field: entry.name });
        p++;
        return { t: "cond", rule: { field, op, value: [a, value(entry)] }, at };
      }
      if (op === "in" || op === "not_in" || op === "has_any") {
        if (toks[p]?.v !== "(") return { t: "cond", rule: { field, op, value: [value(entry)] }, at };
        const open = toks[p];
        p++;
        const list: unknown[] = [];
        while (toks[p] && toks[p].v !== ")") {
          list.push(value(entry));
          if (toks[p]?.v === ",") p++;
          else if (toks[p]?.v !== ")") fail("list", toks[p]?.at ?? src.length, { field: entry.name });
        }
        if (!toks[p]) fail("paren", open.at);
        p++;
        if (!list.length) fail("list", open.at, { field: entry.name });
        return { t: "cond", rule: { field, op, value: list }, at };
      }
      return { t: "cond", rule: { field, op: op as Rule["op"], value: value(entry) }, at };
    };
    const unit = (): Node => {
      const t = toks[p];
      if (t?.v === "(") {
        p++;
        const inner = expr();
        if (toks[p]?.v !== ")") fail("paren", t.at);
        p++;
        return inner.t === "cond" ? inner : { ...inner, paren: true };
      }
      return cond();
    };
    const chain = (kind: "and" | "or", next: () => Node, words: string[]): Node => {
      const at = toks[p]?.at ?? 0;
      const items = [next()];
      while (isWord(toks[p], ...words)) { p++; items.push(next()); }
      return items.length === 1 ? items[0] : { t: kind, items, paren: false, at };
    };
    const and = () => chain("and", unit, AND);
    const expr = (): Node => chain("or", and, OR);
    if (!toks.length) return { where: { any: false, rules: [] }, refNames };
    const tree = expr();
    if (p < toks.length) fail(toks[p].v === ")" ? "paren" : "join", toks[p].at, { x: toks[p].v });
    return { where: toWhere(tree), refNames };
  } catch (e) {
    if (e instanceof Fail) return { error: e.e };
    throw e;
  }
}

/** The tree as rules, one level of groups deep; (a e b) ou c becomes a group and a rule. */
function toWhere(node: Node): Where {
  // A bracket holding the same join as around it adds nothing.
  const flat = (n: Node): Node => (n.t === "cond" ? n : { ...n, items: n.items.map(flat).flatMap((c) => (c.t === n.t ? (c as { items: Node[] }).items : [c])) });
  const tree = flat(node);
  const rulesOf = (n: Node): Rule[] => {
    if (n.t === "cond") return [n.rule];
    return n.items.map((c) => { if (c.t !== "cond") fail("nested", c.at); return (c as { rule: Rule }).rule; });
  };
  let w: Where;
  if (tree.t === "cond") w = { any: false, rules: [tree.rule] };
  else {
    const rules: Rule[] = [];
    const groups: { any: boolean; rules: Rule[] }[] = [];
    for (const c of tree.items) {
      if (c.t === "cond") rules.push(c.rule);
      else groups.push({ any: c.t === "or", rules: rulesOf(c) });
    }
    w = groups.length ? { any: tree.t === "or", rules, groups } : { any: tree.t === "or", rules };
  }
  const count = w.rules.length + (w.groups ?? []).reduce((n, g) => n + g.rules.length, 0);
  if (count > MAX_RULES || (w.groups ?? []).length > MAX_GROUPS) fail("too_many", 0);
  return w;
}

const WORDS: Record<Lang, Record<string, string>> = {
  pt: { and: "e", or: "ou", contains: "contém", not_contains: "não contém", starts_with: "começa com", in: "em", not_in: "não em", has: "tem", has_any: "tem algum", between: "entre", empty: "é vazio", not_empty: "não é vazio", last_days: "nos últimos", next_days: "nos próximos", days: "dias", today: "hoje", yes: "sim", no: "não" },
  en: { and: "and", or: "or", contains: "contains", not_contains: "not contains", starts_with: "starts with", in: "in", not_in: "not in", has: "has", has_any: "has any", between: "between", empty: "is empty", not_empty: "is not empty", last_days: "in last", next_days: "in next", days: "days", today: "today", yes: "yes", no: "no" },
};
const SYMBOLS: Partial<Record<EditOp, string>> = { eq: "=", neq: "!=", gt: ">", gte: ">=", lt: "<", lte: "<=" };
const KEYWORDS = new Set(["e", "and", "ou", "or", "não", "nao", "not", "em", "in", "é", "is", "tem", "has", "entre", "between", "hoje", "today"]);

const quote = (s: string) => `"${s.replace(/["\\]/g, (c) => `\\${c}`)}"`;
/** A field's name as text: bare when it is one plain word. */
export const fieldWord = (name: string) => (/^[\p{L}_][\p{L}\p{N}_]*$/u.test(name) && !KEYWORDS.has(low(name)) ? name : quote(name));
/** The words an operator is written with. */
export const opWord = (op: EditOp, lang: Lang) => SYMBOLS[op] ?? WORDS[lang][op];

/** Rules as text; refNames gives the names of referenced records (else the id). */
export function ruleText(w: Where, entries: Entry[], lang: Lang, refNames: Record<string, string> = {}): string {
  const L = WORDS[lang];
  const one = (v: unknown): string => {
    const n = todayOf(v);
    if (n !== undefined) return n === 0 ? L.today : `${L.today}${n > 0 ? "+" : ""}${n}`;
    if (typeof v === "number") return String(v);
    if (typeof v === "boolean") return v ? L.yes : L.no;
    const s = String(v);
    if (/^\d{4}-\d{2}-\d{2}$/.test(s)) return s;
    return quote(refNames[s] ?? s);
  };
  const cond = (r: Rule) => {
    const entry = entries.find((e) => e.key === r.field);
    const field = fieldWord(entry?.name ?? r.field);
    if (r.op === "empty" || r.op === "not_empty") return `${field} ${L[r.op]}`;
    if (r.op === "between" && Array.isArray(r.value)) {
      const [a, b] = r.value.map(todayOf);
      if (a !== undefined && a < 0 && b === 0) return `${field} ${L.last_days} ${-a} ${L.days}`;
      if (a === 0 && b !== undefined && b > 0) return `${field} ${L.next_days} ${b} ${L.days}`;
      return `${field} ${L.between} ${one(r.value[0])} ${L.and} ${one(r.value[1])}`;
    }
    const op = opWord(r.op, lang);
    return Array.isArray(r.value) ? `${field} ${op} (${r.value.map(one).join(", ")})` : `${field} ${op} ${one(r.value)}`;
  };
  const join = (any: boolean) => ` ${any ? L.or : L.and} `;
  const groups = (w.groups ?? []).filter((g) => g.rules.length);
  const parts = [...w.rules.map(cond), ...groups.map((g) => (g.rules.length > 1 && (g.any || !w.any) ? `(${g.rules.map(cond).join(join(g.any))})` : g.rules.map(cond).join(join(g.any))))];
  return parts.join(join(w.any));
}

/** What fits at the cursor: field names, operators, values or a join word. */
export function suggestAt(src: string, cursor: number, entries: Entry[], lang: Lang): { from: number; items: string[] } {
  const before = src.slice(0, cursor);
  const partial = /("[^"]*|[\p{L}\p{N}_$.\-@]*)$/u.exec(before)?.[0] ?? "";
  const from = cursor - partial.length;
  let toks: Tok[];
  try { toks = tokenize(before.slice(0, from), true); } catch { return { from, items: [] }; }
  const L = WORDS[lang];
  let state: "field" | "op" | "value" | "join" = "field";
  let entry: Entry | undefined;
  let op: EditOp | undefined;
  let need = 1; // values left (between takes two; a list until ")")
  let list = false;
  for (let p = 0; p < toks.length;) {
    const t = toks[p];
    if (state === "field") {
      if (t.v === "(") { p++; continue; }
      const f = matchField(toks, p, entries);
      if (!f) return { from, items: [] };
      entry = f.entry; p = f.next; state = "op"; continue;
    }
    if (state === "op") {
      const o = matchOp(toks, p);
      if (!o) return { from, items: [] };
      op = o.op; p = o.next;
      if (op === "empty" || op === "not_empty") { state = "join"; continue; }
      state = "value"; need = op === "between" ? 2 : 1; list = false; continue;
    }
    if (state === "value") {
      if (t.v === "(") { list = true; p++; continue; }
      if (list) { if (t.v === ")") { list = false; state = "join"; } p++; continue; }
      if (op === "between" && need === 1 && isWord(t, ...AND)) { p++; continue; }
      p++; need--;
      if (need === 0) state = "join";
      continue;
    }
    if (t.v === ")" || isWord(t, "dias", "dia", "days", "day")) { p++; continue; }
    if (isWord(t, ...AND, ...OR)) { state = "field"; p++; continue; }
    return { from, items: [] };
  }
  const q = low(partial.replace(/^"/, ""));
  const pick = (items: string[]) => items.filter((x) => low(x.replace(/^"/, "")).startsWith(q) && low(x) !== low(partial)).slice(0, 12);
  if (state === "field") return { from, items: pick(entries.map((e) => fieldWord(e.name))) };
  if (state === "op" && entry) {
    return { from, items: pick(opsFor(entry.kind).map((o) => (o === "last_days" || o === "next_days" ? `${L[o]} 7 ${L.days}` : opWord(o, lang)))) };
  }
  if (state === "value" && entry) {
    if (op === "between" && need === 1 && partial === "" && toks.length && !isWord(toks[toks.length - 1], ...AND)) return { from, items: [L.and] };
    if (entry.kind === "enum") return { from, items: pick(entry.options.map(quote)) };
    if (entry.kind === "boolean") return { from, items: pick([L.yes, L.no]) };
    if (entry.kind === "date") return { from, items: pick([L.today, `${L.today}-7`, `${L.today}+7`]) };
    return { from, items: [] };
  }
  if (state === "join") return { from, items: pick([L.and, L.or]) };
  return { from, items: [] };
}
