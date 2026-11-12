/** One record read from an import file, with the line (or array position) it came from. */
export interface ParsedRecord {
  line: number;
  data: unknown;
}

export interface ParsedFile {
  records: ParsedRecord[];
  /** Lines that are not JSON, or records too large to send. */
  errors: { line: number; message: "parse" | "too_large" }[];
}

// The API body limit is about 100 KB: batches stay under it with room for the envelope.
export const BATCH_BYTES = 80_000;
export const BATCH_RECORDS = 1000;

/** Reads JSON Lines (one record per line) or one JSON array of records. */
export function parseImport(text: string): ParsedFile {
  const out: ParsedFile = { records: [], errors: [] };
  const body = text.replace(/^﻿/, "");
  if (body.trimStart().startsWith("[")) {
    try {
      const items: unknown = JSON.parse(body);
      if (Array.isArray(items)) {
        items.forEach((data, i) => out.records.push({ line: i + 1, data }));
        return out;
      }
    } catch {
      // not one array: read it as JSON Lines below
    }
  }
  body.split(/\r?\n/).forEach((raw, i) => {
    if (!raw.trim()) return;
    try {
      out.records.push({ line: i + 1, data: JSON.parse(raw) });
    } catch {
      out.errors.push({ line: i + 1, message: "parse" });
    }
  });
  return out;
}

/** Splits records into request-sized batches; a record larger than one batch is reported instead. */
export function batches(records: ParsedRecord[]): { batches: ParsedRecord[][]; tooLarge: number[] } {
  const out: ParsedRecord[][] = [];
  const tooLarge: number[] = [];
  let current: ParsedRecord[] = [];
  let size = 0;
  for (const r of records) {
    const bytes = new TextEncoder().encode(JSON.stringify(r.data)).length + 1;
    if (bytes > BATCH_BYTES) {
      tooLarge.push(r.line);
      continue;
    }
    if (current.length && (size + bytes > BATCH_BYTES || current.length >= BATCH_RECORDS)) {
      out.push(current);
      current = [];
      size = 0;
    }
    current.push(r);
    size += bytes;
  }
  if (current.length) out.push(current);
  return { batches: out, tooLarge };
}
