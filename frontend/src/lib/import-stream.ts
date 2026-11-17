import { fieldsOf } from "@/components/field-builder";
import { BATCH_BYTES, BATCH_RECORDS, type ParsedRecord } from "@/lib/import-file";
import { CsvReader, SchemaInferrer, csvRow, rowReader } from "@/lib/infer-schema";
import { xlsxRows } from "@/lib/xlsx";

/**
 * Reading an import file of any size without holding it whole: CSV and JSON Lines
 * are read in pieces, and only one batch of records exists at a time. These run
 * in the import worker (lib/import.worker.ts), or inline where workers are missing.
 */

/** A problem with one line of the file, before the server sees it. */
export type LineProblem = { line: number; code: "parse" | "too_large" | "duplicate"; field?: string };
export type Batch = { records: ParsedRecord[]; problems: LineProblem[] };
type Progress = (bytesRead: number) => void;

const isCsv = (f: File) => /\.csv$/i.test(f.name);
const isXlsx = (f: File) => /\.xlsx$/i.test(f.name);
export const isTable = (f: File) => isCsv(f) || isXlsx(f);

/** The file's text in pieces, reporting the bytes read so far. */
async function* textPieces(file: File, progress?: Progress): AsyncGenerator<string> {
  if (typeof file.stream !== "function") { // test doubles and old browsers
    yield await file.text();
    progress?.(file.size);
    return;
  }
  const reader = file.stream().getReader();
  const decoder = new TextDecoder(); // drops a leading BOM
  let read = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    read += value.byteLength;
    yield decoder.decode(value, { stream: true });
    progress?.(read);
  }
  const rest = decoder.decode();
  if (rest) yield rest;
}

/** Rows of a CSV or the first sheet of an .xlsx, header first; both read in pieces. */
async function* tableRows(file: File, progress?: Progress): AsyncGenerator<string[]> {
  if (isXlsx(file)) { // the sheet streams; progress counts rows, not bytes, so it shows at the end
    yield* xlsxRows(file);
    progress?.(file.size);
    return;
  }
  const csv = new CsvReader();
  for await (const piece of textPieces(file, progress)) yield* csv.push(piece);
  yield* csv.end();
}

/**
 * Splits one JSON array, fed in pieces of any size, into the text of its items,
 * so a file of any size never has to be one string. Only brackets, braces and
 * strings are tracked; each item is then read with JSON.parse.
 */
export class JsonArrayReader {
  private depth = 0; // 0 before the [, 1 between items
  private inString = false;
  private escaped = false;
  private item = "";
  private ended = false;

  push(text: string): string[] {
    const out: string[] = [];
    let start = 0;
    const keep = (i: number) => { if (i > start) this.item += text.slice(start, i); };
    for (let i = 0; i < text.length && !this.ended; i++) {
      const c = text[i];
      if (this.inString) {
        if (this.escaped) this.escaped = false;
        else if (c === "\\") this.escaped = true;
        else if (c === '"') this.inString = false;
        continue;
      }
      if (this.depth === 0) {
        if (c === "[") { this.depth = 1; start = i + 1; }
        else start = i + 1; // BOM, spaces
        continue;
      }
      if (c === '"') this.inString = true;
      else if (c === "{" || c === "[") this.depth++;
      else if (c === "}" || c === "]") {
        if (this.depth === 1) { keep(i); this.emit(out); this.ended = true; start = i + 1; break; } // the array's ]
        this.depth--;
      } else if (c === "," && this.depth === 1) { keep(i); this.emit(out); start = i + 1; }
    }
    if (!this.ended) keep(text.length);
    return out;
  }

  /** What is left once the text ends: an unclosed array's last item. */
  end(): string[] {
    const out: string[] = [];
    if (!this.ended) this.emit(out);
    return out;
  }

  private emit(out: string[]) {
    const t = this.item.trim();
    this.item = "";
    if (t) out.push(t);
  }
}

/** Records of a JSON Lines file or of one JSON array, both read in pieces. */
async function* jsonRecords(file: File, progress?: Progress): AsyncGenerator<ParsedRecord | LineProblem> {
  let line = 0;
  let rest = "";
  let array: JsonArrayReader | null = null;
  let decided = false;
  const parse = function* (raw: string): Generator<ParsedRecord | LineProblem> {
    line++;
    if (!raw.trim()) return;
    try { yield { line, data: JSON.parse(raw) }; } catch { yield { line, code: "parse" }; }
  };
  for await (const piece of textPieces(file, progress)) {
    if (!decided) {
      const head = (rest + piece).replace(/^\uFEFF/, "").trimStart();
      if (!head) { rest += piece; continue; }
      decided = true;
      if (head.startsWith("[")) {
        array = new JsonArrayReader();
        // In an array the "line" is the item's position.
        for (const item of array.push(rest + piece)) yield* parse(item);
        rest = "";
        continue;
      }
    }
    if (array) { for (const item of array.push(piece)) yield* parse(item); continue; }
    const lines = (rest + piece).split(/\r?\n/);
    rest = lines.pop() ?? "";
    for (const raw of lines) yield* parse(raw);
  }
  if (array) { for (const item of array.end()) yield* parse(item); return; }
  if (rest) yield* parse(rest);
}

/** The header row of a CSV or sheet. */
export async function headerOf(file: File): Promise<string[]> {
  for await (const row of tableRows(file)) return row;
  return [];
}

/** Fields guessed from the whole file, read once. */
export async function inferFile(file: File, progress?: Progress): Promise<Record<string, unknown> | null> {
  if (isTable(file)) {
    const inf = new SchemaInferrer(true);
    let header: string[] | null = null;
    for await (const row of tableRows(file, progress)) {
      if (!header) { header = row; inf.keys(row.map((h) => h.trim())); continue; }
      inf.add(csvRow(header, row));
    }
    return header?.length ? inf.schema() : null;
  }
  const inf = new SchemaInferrer(false);
  for await (const r of jsonRecords(file, progress)) if ("data" in r) inf.add(r.data);
  return inf.schema();
}

/**
 * The file's records in request-sized batches, each with the problems found on the
 * way: lines that are not JSON, records too large to send, and a unique field's value
 * an earlier line already holds (the server would take the first and refuse the
 * others, but it sees one batch at a time). upsertKey's repeats update one record, so they pass.
 */
export async function* recordBatches(
  file: File,
  opts: { schema: Record<string, unknown> | null; columns?: string[]; upsertKey?: string },
  progress?: Progress,
): AsyncGenerator<Batch> {
  const uniques = fieldsOf(opts.schema).filter((r) => r.field.unique && r.key !== opts.upsertKey).map((r) => r.key);
  const seen = new Map(uniques.map((k) => [k, new Set<string>()]));
  let records: ParsedRecord[] = [];
  let problems: LineProblem[] = [];
  let size = 0;
  const encoder = new TextEncoder();

  async function* source(): AsyncGenerator<ParsedRecord | LineProblem> {
    if (!isTable(file)) return yield* jsonRecords(file, progress);
    let read: ((row: string[], line: number) => ParsedRecord) | null = null;
    let line = 1;
    for await (const row of tableRows(file, progress)) {
      if (!read) { read = rowReader(row, opts.schema, opts.columns); continue; }
      yield read(row, ++line);
    }
  }

  for await (const r of source()) {
    if (!("data" in r)) { problems.push(r); continue; }
    const bytes = encoder.encode(JSON.stringify(r.data)).length + 1;
    if (bytes > BATCH_BYTES) { problems.push({ line: r.line, code: "too_large" }); continue; }
    const dup = uniques.find((k) => {
      const v = (r.data as Record<string, unknown> | null)?.[k];
      if (v === undefined || v === null) return false;
      const set = seen.get(k)!;
      const key = JSON.stringify(v);
      if (set.has(key)) return true;
      set.add(key);
      return false;
    });
    if (dup) { problems.push({ line: r.line, code: "duplicate", field: dup }); continue; }
    if (records.length && (size + bytes > BATCH_BYTES || records.length >= BATCH_RECORDS)) {
      yield { records, problems };
      records = [];
      problems = [];
      size = 0;
    }
    records.push(r);
    size += bytes;
  }
  if (records.length || problems.length) yield { records, problems };
}
