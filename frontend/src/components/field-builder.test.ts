import { expect, it } from "vitest";
import { addField } from "./field-builder";

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
