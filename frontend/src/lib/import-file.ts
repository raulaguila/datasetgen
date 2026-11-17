/** One record read from an import file, with the line (or array position) it came from. */
export interface ParsedRecord {
  line: number;
  data: unknown;
}

export interface ParsedFile {
  records: ParsedRecord[];
  /** Lines that are not JSON. */
  errors: { line: number; message: "parse" }[];
}

// The API takes up to 5000 records in an 8 MB body: batches stay well under both.
// Fewer, larger batches keep a big file under the per-minute request limit.
export const BATCH_BYTES = 4_000_000;
export const BATCH_RECORDS = 5000;
/** One record's data cap on the server (domain.RecordMaxBytes); the server measures it exactly. */
export const RECORD_BYTES = 1 << 20;

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
