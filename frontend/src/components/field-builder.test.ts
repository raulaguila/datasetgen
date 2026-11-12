import { expect, it } from "vitest";
import { addField, fieldsOf, moveField, removeField, updateField } from "./field-builder";

it("adds a field and keeps the rest of the schema", () => {
  const start = { type: "object", title: "Wired", required: ["nome"], properties: { nome: { type: "string" } } };
  const next = addField(start, { name: " categoria ", type: "enum", required: true, options: "gatilho, efeito,," });
  expect(next).toEqual({
    type: "object",
    title: "Wired",
    required: ["nome", "categoria"],
    properties: { nome: { type: "string" }, categoria: { type: "string", title: "categoria", enum: ["gatilho", "efeito"] } },
  });
  expect(addField({}, { name: "tags", type: "strings", required: false, options: "" })).toEqual({
    type: "object",
    properties: { tags: { type: "array", title: "tags", items: { type: "string" } } },
  });
});

const wired = {
  type: "object",
  required: ["nome"],
  properties: {
    nome: { type: "string", title: "Nome", minLength: 1 },
    descricao: { type: "string", title: "Descrição", maxLength: 5000 },
    categoria: { type: "string", enum: ["gatilho", "efeito"] },
    params: { type: "object", properties: {} },
  },
};

it("lists fields with the type the builder would draw", () => {
  expect(fieldsOf(wired).map((r) => [r.key, r.field.name, r.field.type, r.field.required, r.advanced])).toEqual([
    ["nome", "Nome", "string", true, false],
    ["descricao", "Descrição", "text", false, false],
    ["categoria", "categoria", "enum", false, false],
    ["params", "params", "string", false, true],
  ]);
  expect(fieldsOf(wired)[2].field.options).toBe("gatilho, efeito");
});

it("edits a field in place, keeping its key and other rules", () => {
  const next = updateField(wired, "nome", { name: "Nome do wired", type: "string", required: false, options: "" });
  expect(Object.keys(next.properties as object)).toEqual(["nome", "descricao", "categoria", "params"]);
  expect((next.properties as Record<string, unknown>).nome).toEqual({ type: "string", title: "Nome do wired", minLength: 1 });
  expect(next.required).toBeUndefined();

  const asText = updateField(wired, "categoria", { name: "Categoria", type: "text", required: true, options: "" });
  expect((asText.properties as Record<string, unknown>).categoria).toEqual({ type: "string", title: "Categoria", maxLength: 5000 });
  expect(asText.required).toEqual(["nome", "categoria"]);
});

it("removes and reorders fields", () => {
  const gone = removeField(wired, "nome");
  expect(Object.keys(gone.properties as object)).toEqual(["descricao", "categoria", "params"]);
  expect(gone.required).toBeUndefined();
  expect(Object.keys(moveField(wired, "descricao", -1).properties as object)).toEqual(["descricao", "nome", "categoria", "params"]);
  expect(moveField(wired, "nome", -1)).toBe(wired);
});
