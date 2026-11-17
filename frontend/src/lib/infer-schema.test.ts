import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { Validator } from "@cfworker/json-schema";
import { fieldsOf } from "@/components/field-builder";
import { csvRecords, parseCsv, schemaFromFile, templateSchema } from "./infer-schema";

describe("parseCsv", () => {
  it("reads quoted cells, either separator and blank lines", () => {
    expect(parseCsv('a,b\n"x, y","say ""hi"""\n\n1,2')).toEqual([["a", "b"], ["x, y", 'say "hi"'], ["1", "2"]]);
    expect(parseCsv("a;b\r\n1,5;2")).toEqual([["a", "b"], ["1,5", "2"]]);
  });
});

describe("schemaFromFile", () => {
  it("discovers subfields even when the first 200 records contain empty objects and lists", () => {
    const records = [...Array.from({ length: 200 }, () => ({ grupo: {}, itens: [] })), { grupo: { codigo: "001" }, itens: [{ qtd: 0 }] }];
    const schema = schemaFromFile("x.json", JSON.stringify(records))!;
    expect(schema.properties).toMatchObject({ grupo: { properties: { codigo: { type: "string" } } }, itens: { items: { properties: { qtd: { type: "integer" } } } } });
  });

  it("preserves nested objects, mixed lists, nulls and native string values", () => {
    const data = { grupo: { interno: { codigo: "001", vazio: "" } }, lista: [{ flag: false, valor: null }, { flag: true, valor: { qtd: 0 } }], numeros: [0, 2.5], bools: [false, true], matriz: [[1], [2, 3]], misto: [1, "001", false, null, { nome: "a" }, ["x"]], vazio: [] };
    const text = Object.keys(data).join(",") + "\n" + Object.values(data).map((v) => '"' + JSON.stringify(v).replaceAll('"', '""') + '"').join(",");
    const schema = schemaFromFile("x.csv", text)!;
    expect(csvRecords(text, schema).records[0].data).toEqual(data);
    expect(new Validator(schema, "2020-12", false).validate(data).valid).toBe(true);
    expect(schema.properties).toMatchObject({ grupo: { properties: { interno: { properties: { codigo: { type: "string" } } } } }, numeros: { items: { type: "number" } }, bools: { items: { type: "boolean" } } });
  });

  it("keeps a field that is null in every record open to values", () => {
    const records = [{ nome: "a", obs: null, grupo: { x: null } }, { nome: "b", obs: null, grupo: { x: null } }];
    const schema = schemaFromFile("x.json", JSON.stringify(records))!;
    const props = schema.properties as Record<string, Record<string, unknown>>;
    expect(props.obs.type).toEqual(["string", "null"]);
    const validator = new Validator(schema, "2020-12", false);
    expect(records.every((r) => validator.validate(r).valid)).toBe(true);
    expect(validator.validate({ nome: "c", obs: "texto", grupo: { x: "y" } }).valid).toBe(true);
  });

  it.each(["csv", "json", "jsonl"])("discovers object fields and list item fields in %s", (format) => {
    const records = [
      { grupo: { nome: "Ana", codigo: "001", ativo: false, notas: ["a", "b"] }, itens: [] },
      { grupo: { nome: "Bia", codigo: "002", ativo: true }, itens: [{ chave: "delay", rotulo: "Atraso", qtd: 2, opcoes: ["a"] }] },
      { grupo: {}, itens: [{ chave: "message", descricao: "Olá", qtd: 3, opcoes: [] }] },
    ];
    const text = format === "json" ? JSON.stringify(records) : format === "jsonl" ? records.map((r) => JSON.stringify(r)).join("\n") :
      "grupo,itens\n" + records.map((r) => [r.grupo, r.itens].map((v) => '"' + JSON.stringify(v).replaceAll('"', '""') + '"').join(",")).join("\n");
    const schema = schemaFromFile(`x.${format}`, text)!;
    const props = schema.properties as Record<string, Record<string, unknown>>;
    expect(props.grupo).toMatchObject({ type: "object", properties: {
      nome: { type: "string" }, codigo: { type: "string" }, ativo: { type: "boolean" }, notas: { type: "array", items: { type: "string" } },
    } });
    expect(props.itens).toMatchObject({ type: "array", items: { type: "object", properties: {
      chave: { type: "string" }, rotulo: { type: "string" }, qtd: { type: "integer" }, opcoes: { type: "array", items: { type: "string" } }, descricao: { type: "string" },
    } } });
    expect(fieldsOf(schema).map((r) => [r.key, r.field.type, r.advanced])).toEqual([["grupo", "group", false], ["itens", "objects", false]]);
    const validator = new Validator(schema, "2020-12", false);
    expect(records.every((r) => validator.validate(r).valid)).toBe(true);
    if (format === "csv") expect(csvRecords(text, schema).records.map((r) => r.data)).toEqual(records);
  });

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
    expect(props.extra).toEqual({ title: "extra", type: "object", properties: { k: { title: "k", type: "integer" } } });
  });

  it("reads CSV text by its look", () => {
    const s = schemaFromFile("x.csv", "sku;qtd;preco;ok\nA1;3;9,90;true\nB2;;1,5;false")!;
    const props = s.properties as Record<string, Record<string, unknown>>;
    expect([props.sku.type, props.qtd.type, props.preco.type, props.ok.type]).toEqual(["string", "integer", "number", "boolean"]);
  });

  it("recognizes JSON cells without interpreting ordinary text as JSON", () => {
    const text = 'tags;extra;numeros;texto\n"[""a""]";"{""k"":1}";"[1,2]";[not JSON]';
    const schema = schemaFromFile("x.csv", text)!;
    const props = schema.properties as Record<string, Record<string, unknown>>;
    expect([props.tags.type, props.extra.type, props.numeros.type, props.texto.type]).toEqual(["array", "object", "array", "string"]);
    expect(csvRecords(text, schema).records[0].data).toEqual({ tags: ["a"], extra: { k: 1 }, numeros: [1, 2], texto: "[not JSON]" });
  });

  it("infers long text without a length limit in both CSV and JSON", () => {
    const values = [...Array<string>(200).fill("x".repeat(201)), "y".repeat(6000)];
    for (const [name, text] of [
      ["x.csv", `conteudo\n${values.join("\n")}`],
      ["x.json", JSON.stringify(values.map((conteudo) => ({ conteudo })))],
    ]) {
      const schema = schemaFromFile(name, text)!;
      expect(schema.properties).toEqual({ conteudo: { title: "conteudo", type: "string", "x-multiline": true } });
      expect(new Validator(schema, "2020-12").validate({ conteudo: "x".repeat(200_000) }).valid).toBe(true);
    }
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
  it("reads back the formula guard our export puts before = + - @", () => {
    const schema = { type: "object", properties: { n: { type: "integer" }, tel: { type: "string" }, f: { type: "string" } } };
    const parsed = csvRecords("\uFEFFn,tel,f\n'-5,'+55 11 9999,'=1+1\n", schema);
    expect(parsed.records[0].data).toEqual({ n: -5, tel: "+55 11 9999", f: "=1+1" });
  });

  it.each([
    { type: ["object", "null"], properties: { qtd: { type: "integer" } } },
    { anyOf: [{ type: "object" }, { type: "null" }] },
    { $ref: "#/$defs/grupo" },
  ])("reads objects declared through nullable types, alternatives and refs: %j", (property) => {
    const schema: Record<string, unknown> = { type: "object", $defs: { grupo: { type: ["object", "null"], properties: { qtd: { type: "integer" } } } }, properties: { grupo: property } };
    const parsed = csvRecords('grupo\n"{""qtd"":0}"\nnull', schema);
    expect(parsed.records.map((r) => r.data)).toEqual([{ grupo: { qtd: 0 } }, { grupo: null }]);
    const validator = new Validator(schema, "2020-12", false);
    expect(parsed.records.every((r) => validator.validate(r.data).valid)).toBe(true);
  });

  it("reads lists with typed, nested and object items against an existing schema", () => {
    const schema: Record<string, unknown> = { type: "object", properties: {
      grupos: { type: "array", items: { type: "object", properties: { qtd: { type: "integer" }, opcoes: { type: "array", items: { type: "string" } } } } },
      numeros: { type: "array", items: { type: "number" } },
      bools: { type: "array", items: { type: "boolean" } },
      matriz: { type: "array", items: { type: "array", items: { type: "integer" } } },
    } };
    const data = { grupos: [{ qtd: 0, opcoes: ["a", "b"] }], numeros: [0, 2.5], bools: [false, true], matriz: [[1], [2, 3]] };
    const text = Object.keys(data).join(",") + "\n" + Object.values(data).map((v) => '"' + JSON.stringify(v).replaceAll('"', '""') + '"').join(",");
    expect(csvRecords(text, schema).records[0].data).toEqual(data);
    expect(new Validator(schema, "2020-12").validate(csvRecords(text, schema).records[0].data).valid).toBe(true);
  });

  it("leaves malformed object and list JSON for validation to reject", () => {
    const schema: Record<string, unknown> = { type: "object", properties: { grupo: { type: "object" }, tags: { type: "array", items: { type: "string" } } } };
    const parsed = csvRecords('grupo;tags\n{broken;[broken', schema);
    expect(parsed.records[0].data).toEqual({ grupo: "{broken", tags: "[broken" });
    expect(new Validator(schema, "2020-12").validate(parsed.records[0].data).valid).toBe(false);
  });

  it("decodes arrays even when their advanced schema has no builder type", () => {
    const schema = { type: "object", properties: {
      fontes: { type: "array", items: { type: "object", properties: {
        opcoes: { type: "array", items: { type: "string", enum: ["Mobi"] } },
      } } },
    } };
    expect(csvRecords('fontes\n"[{""opcoes"":[""Mobi""]}]"', schema).records).toEqual([
      { line: 2, data: { fontes: [{ opcoes: ["Mobi"] }] } },
    ]);
  });

  it("keeps JSON-looking text when the destination field is a string", () => {
    expect(csvRecords('texto\n"{""k"":1}"', { properties: { texto: { type: "string" } } }).records[0].data)
      .toEqual({ texto: '{"k":1}' });
  });

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

describe("Wired Club CSV imports", () => {
  it.each([
    ["Wired_Club___refer_ncia-20261117.csv", "wireds", 221],
    ["Wired_Club___guias-20261117.csv", "guias", 33],
  ])("preserves every record in %s with existing and inferred fields", (name, schemaName, count) => {
    const text = readFileSync(resolve("../dataset", name), "utf8");
    const existing = JSON.parse(readFileSync(resolve("../examples/wiredclub", `${schemaName}.schema.json`), "utf8"));
    const inferred = schemaFromFile(name, text)!;
    if (schemaName === "wireds") expect(inferred.properties).toMatchObject({
      versao_negativa: { properties: { nome: { type: "string" }, classname: { type: "string" }, revisao: { type: "integer" } } },
      parametros: { items: { properties: { chave: { type: "string" }, rotulo: { type: "string" }, opcoes: { type: "array" } } } },
      fontes: { items: { properties: { bloco: { type: "string" }, titulo: { type: "string" }, opcoes: { type: "array", items: { type: "string" } } } } },
    });
    for (const schema of [existing, inferred]) {
      const parsed = csvRecords(text, schema);
      expect(parsed.errors).toEqual([]);
      expect(parsed.records).toHaveLength(count);
      const validator = new Validator(schema, "2020-12", false);
      const invalid = parsed.records.flatMap(({ line, data }) => {
        const result = validator.validate(data);
        return result.valid ? [] : [{ line, errors: result.errors }];
      });
      expect(invalid).toEqual([]);
    }
    expect(csvRecords(text, inferred).records).toEqual(csvRecords(text, existing).records);
  });
});
