import { describe, expect, it, vi } from "vitest";
import { CsvReader, parseCsv, schemaFromFile } from "./infer-schema";
import { JsonArrayReader, inferFile, recordBatches } from "./import-stream";
import { withRetry } from "./import-client";
import { ApiError } from "./api/client";
import { BATCH_RECORDS, RECORD_BYTES } from "./import-file";

const csv = '﻿nome;obs;n\r\n"Ana; ""A""";"linha 1\nlinha 2";-1\r\nBia;;\'-5\r\n\r\n;;\r\nCid;x;3';

/** A File whose stream hands out the text in pieces of `size` bytes. */
function chunked(text: string, name: string, size: number): File {
  const bytes = new TextEncoder().encode(text);
  const file = new File([bytes], name);
  Object.defineProperty(file, "stream", {
    value: () => new ReadableStream<Uint8Array>({
      start(c) {
        for (let i = 0; i < bytes.length; i += size) c.enqueue(bytes.slice(i, i + size));
        c.close();
      },
    }),
  });
  return file;
}

describe("CsvReader", () => {
  it("reads the same rows whatever the pieces' size", () => {
    const whole = parseCsv(csv);
    expect(whole).toEqual([["nome", "obs", "n"], ['Ana; "A"', "linha 1\nlinha 2", "-1"], ["Bia", "", "-5"], ["Cid", "x", "3"]]);
    for (const size of [1, 2, 3, 7]) {
      const r = new CsvReader();
      const rows: string[][] = [];
      for (let i = 0; i < csv.length; i += size) rows.push(...r.push(csv.slice(i, i + size)));
      rows.push(...r.end());
      expect(rows).toEqual(whole);
    }
  });
});

describe("import stream", () => {
  it("infers from a streamed file what the whole text gives", async () => {
    for (const size of [1, 5, 64]) expect(await inferFile(chunked(csv, "x.csv", size))).toEqual(schemaFromFile("x.csv", csv));
    const jsonl = '{"a":1,"b":{"c":"x"}}\n\n{"a":2.5,"b":null}\nnot json\n';
    expect(await inferFile(chunked(jsonl, "x.jsonl", 3))).toEqual(schemaFromFile("x.jsonl", jsonl));
    const array = JSON.stringify([{ a: 1 }, { a: "x" }]);
    expect(await inferFile(chunked(array, "x.json", 4))).toEqual(schemaFromFile("x.json", array));
  });

  it("batches records, flags unreadable lines and repeated unique values across batches", async () => {
    const schema = { type: "object", properties: { cod: { type: "string", "x-unique": true }, n: { type: "integer" } } };
    const n = BATCH_RECORDS + 10;
    const lines = Array.from({ length: n }, (_, i) => JSON.stringify({ cod: `c${i % (n - 3)}`, n: i }));
    lines.splice(5, 0, "{oops");
    const out = [];
    for await (const b of recordBatches(chunked(lines.join("\n"), "x.jsonl", 997), { schema })) out.push(b);
    expect(out.map((b) => b.records.length)).toEqual([BATCH_RECORDS, n - 3 - BATCH_RECORDS]);
    expect(out.flatMap((b) => b.problems)).toEqual([
      { line: 6, code: "parse" },
      ...[n - 2, n - 1, n].map((i) => ({ line: i + 1, code: "duplicate", field: "cod" })),
    ]);
    // An upsert key's repeats update one record: they pass.
    let passed = 0;
    for await (const b of recordBatches(chunked(lines.join("\n"), "x.jsonl", 997), { schema, upsertKey: "cod" })) passed += b.records.length;
    expect(passed).toBe(n);
  });

  it("sets aside a record over the server's size cap", async () => {
    const text = [JSON.stringify({ s: "x".repeat(RECORD_BYTES) }), JSON.stringify({ s: "ok" })].join("\n");
    const out = [];
    for await (const b of recordBatches(chunked(text, "x.jsonl", 65536), { schema: null })) out.push(b);
    expect(out.flatMap((b) => b.records.map((r) => r.line))).toEqual([2]);
    expect(out.flatMap((b) => b.problems)).toEqual([{ line: 1, code: "too_large" }]);
  });

  it("reads CSV rows with the export's formula guard undone", async () => {
    const schema = { type: "object", properties: { nome: { type: "string" }, obs: { type: "string" }, n: { type: "integer" } } };
    const out = [];
    for await (const b of recordBatches(chunked(csv, "x.csv", 4), { schema })) out.push(...b.records);
    expect(out).toEqual([
      { line: 2, data: { nome: 'Ana; "A"', obs: "linha 1\nlinha 2", n: -1 } },
      { line: 3, data: { nome: "Bia", n: -5 } },
      { line: 4, data: { nome: "Cid", obs: "x", n: 3 } },
    ]);
  });
});

describe("JsonArrayReader", () => {
  it("splits an array into its items whatever the pieces' size", () => {
    const items = [{ a: "x, ]\"}" }, [1, [2, { b: null }]], "texto [com] {chaves}", 3.5, true, null, { c: "\\" }];
    const text = "\uFEFF  [\n" + items.map((i) => JSON.stringify(i)).join(" ,\n") + "\n]  \n";
    for (const size of [1, 2, 5, 1000]) {
      const r = new JsonArrayReader();
      const out: string[] = [];
      for (let i = 0; i < text.length; i += size) out.push(...r.push(text.slice(i, i + size)));
      out.push(...r.end());
      expect(out.map((x) => JSON.parse(x))).toEqual(items);
    }
  });

  it("reads a streamed array file as records, a broken item as a problem", async () => {
    const text = '[{"a":1},{"a":},{"a":3}]';
    const out = [];
    for await (const b of recordBatches(chunked(text, "x.json", 3), { schema: null })) out.push(b);
    expect(out[0].records).toEqual([{ line: 1, data: { a: 1 } }, { line: 3, data: { a: 3 } }]);
    expect(out[0].problems).toEqual([{ line: 2, code: "parse" }]);
  });
});

describe("withRetry", () => {
  it("waits out the request limit and failures that may pass, then gives up on the rest", async () => {
    vi.useFakeTimers();
    try {
      const errs: unknown[] = [new ApiError(429, {}, 1), new TypeError("network"), new ApiError(503, {})];
      const fn = vi.fn(async () => { const e = errs.shift(); if (e) throw e; return "ok"; });
      const waits: unknown[] = [];
      const p = withRetry(fn, (w) => waits.push(w?.reason ?? null));
      await vi.runAllTimersAsync();
      expect(await p).toBe("ok");
      expect(fn).toHaveBeenCalledTimes(4);
      expect(waits.filter(Boolean)).toEqual(["limit", "failure", "failure", "failure", "failure", "failure", "failure"]);
      const refused = withRetry(async () => { throw new ApiError(422, {}); }, () => {});
      await expect(refused).rejects.toBeInstanceOf(ApiError);
    } finally {
      vi.useRealTimers();
    }
  });
});
