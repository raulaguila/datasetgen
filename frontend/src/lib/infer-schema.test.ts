import { describe, expect, it } from "vitest";
import { csvRecords, parseCsv, schemaFromFile, templateSchema } from "./infer-schema";

describe("parseCsv", () => {
  it("reads quoted cells, either separator and blank lines", () => {
    expect(parseCsv('a,b\n"x, y","say ""hi"""\n\n1,2')).toEqual([["a", "b"], ["x, y", 'say "hi"'], ["1", "2"]]);
    expect(parseCsv("a;b\r\n1,5;2")).toEqual([["a", "b"], ["1,5", "2"]]);
  });
});

describe("schemaFromFile", () => {
  it("guesses types from JSON Lines and keeps the file's keys", () => {
    const text = [
      '{"Nome":"Ana","idade":30,"peso":61.5,"ativo":true,"email":"a@x.io","site":"https://x.io","nasc":"1990-01-02","tags":["a"],"extra":{"k":1}}',
      '{"Nome":"Bia","idade":25,"peso":55,"ativo":false,"email":"b@x.io","site":"https://y.io","nasc":"1999-03-04","tags":[],"extra":{}}',
    ].join("\n");
    const s = schemaFromFile("pessoas.jsonl", text)!;
    const props = s.properties as Record<string, Record<string, unknown>>;
    expect(Object.keys(props)).toEqual(["Nome", "idade", "peso", "ativo", "email", "site", "nasc", "tags", "extra"]);
    expect([props.Nome.type, props.idade.type, props.peso.type, props.ativo.type]).toEqual(["string", "integer", "number", "boolean"]);
    expect([props.email.format, props.site.format, props.nasc.format]).toEqual(["email", "uri", "date"]);
    expect(props.tags).toMatchObject({ type: "array" });
    expect(props.extra).toEqual({ title: "extra", type: "object" });
  });

  it("reads CSV text by its look", () => {
    const s = schemaFromFile("x.csv", "sku;qtd;preco;ok\nA1;3;9,90;true\nB2;;1,5;false")!;
    const props = s.properties as Record<string, Record<string, unknown>>;
    expect([props.sku.type, props.qtd.type, props.preco.type, props.ok.type]).toEqual(["string", "integer", "number", "boolean"]);
  });

  it("finds nothing in a file without records", () => {
    expect(schemaFromFile("x.json", "[1, 2]")).toBeNull();
    expect(schemaFromFile("x.csv", "")).toBeNull();
  });
});

it("builds a template from labels", () => {
  expect(templateSchema([{ name: "Código", unique: true, required: true }, { name: "Qtd", type: "integer" }])).toEqual({
    type: "object",
    required: ["codigo"],
    properties: { codigo: { title: "Código", type: "string", "x-unique": true }, qtd: { title: "Qtd", type: "integer" } },
  });
});

describe("csvRecords", () => {
  it("reads columns by key or label and converts cells by the field type", () => {
    const schema = {
      type: "object",
      properties: {
        nome: { type: "string", title: "Nome" },
        idade: { type: "integer" },
        peso: { type: "number" },
        ativo: { type: "boolean" },
        tags: { type: "array", items: { type: "string" } },
      },
    };
    const out = csvRecords('Nome;idade;peso;ativo;tags;outro\nAna;30;61,5;sim;"a; b";\nBia;x;;0;"[""c""]";1\n', schema);
    expect(out.records).toEqual([
      { line: 2, data: { nome: "Ana", idade: 30, peso: 61.5, ativo: true, tags: ["a", "b"] } },
      { line: 3, data: { nome: "Bia", idade: "x", ativo: false, tags: ["c"], outro: "1" } },
    ]);
  });
});
