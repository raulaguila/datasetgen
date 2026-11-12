import { describe, expect, it } from "vitest";
import { forDraft, issueErrors, uiFor } from "./schema-form";

const props = { properties: { a: { type: "string", format: "date" } }, required: ["a"], type: "object" };

describe("forDraft", () => {
  it("validates schemas that declare draft 2020-12", () => {
    const { schema, validator } = forDraft({ $schema: "https://json-schema.org/draft/2020-12/schema", ...props });
    expect(schema).not.toHaveProperty("$schema");
    expect(validator.isValid(schema, { a: "2020-01-01" }, schema)).toBe(true);
    expect(validator.isValid(schema, {}, schema)).toBe(false);
    expect(validator.isValid(schema, { a: "ontem" }, schema)).toBe(false);
  });

  it("keeps the draft-07 validator for draft-07 schemas", () => {
    const { schema, validator } = forDraft({ $schema: "http://json-schema.org/draft-07/schema#", ...props, items: [{ type: "string" }] });
    expect(validator.isValid(schema, { a: "2020-01-01" }, schema)).toBe(true);
  });
});

describe("uiFor", () => {
  it("draws text areas for long strings, nested too", () => {
    const schema = {
      type: "object",
      properties: {
        nome: { type: "string" },
        descricao: { type: "string", maxLength: 5000 },
        exemplos: { type: "array", items: { type: "object", properties: { explicacao: { type: "string", maxLength: 5000 } } } },
      },
    };
    expect(uiFor(schema)).toEqual({ descricao: { "ui:widget": "textarea" }, exemplos: { items: { explicacao: { "ui:widget": "textarea" } } } });
  });
});

describe("issueErrors", () => {
  it("puts each server issue under its field", () => {
    expect(issueErrors([{ path: "/parametros/0", message: "falta nome" }, { path: "", message: "raiz" }])).toEqual({
      __errors: ["raiz"],
      parametros: { 0: { __errors: ["falta nome"] } },
    });
  });
});
