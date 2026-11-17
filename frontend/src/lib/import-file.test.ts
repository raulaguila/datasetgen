import { describe, expect, it } from "vitest";
import { parseImport } from "./import-file";

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
