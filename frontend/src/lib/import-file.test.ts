import { describe, expect, it } from "vitest";
import { BATCH_BYTES, batches, parseImport } from "./import-file";

describe("parseImport", () => {
  it("reads JSON Lines and reports broken lines", () => {
    const p = parseImport('{"a":1}\n\nnot json\r\n{"a":2}\n');
    expect(p.records).toEqual([{ line: 1, data: { a: 1 } }, { line: 4, data: { a: 2 } }]);
    expect(p.errors).toEqual([{ line: 3, message: "parse" }]);
  });
  it("reads one JSON array", () => {
    expect(parseImport('﻿[{"a":1},{"a":2}]').records.map((r) => r.line)).toEqual([1, 2]);
  });
});

describe("batches", () => {
  it("splits by size and sets aside records too large to send", () => {
    const big = "x".repeat(BATCH_BYTES / 2);
    const recs = [{ s: big }, { s: big }, { s: "x".repeat(BATCH_BYTES) }, { s: "small" }].map((data, i) => ({ line: i + 1, data }));
    const b = batches(recs);
    expect(b.batches.map((x) => x.map((r) => r.line))).toEqual([[1], [2, 4]]);
    expect(b.tooLarge).toEqual([3]);
  });
});
