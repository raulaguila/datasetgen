import { describe, expect, it } from "vitest";
import { issueErrors, uiFor } from "./schema-form";

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

  it("shows the first example as the placeholder", () => {
    expect(uiFor({ properties: { nome: { type: "string", examples: ["Ao clicar"] }, n: { type: "number", examples: [3] } } })).toEqual({ nome: { "ui:placeholder": "Ao clicar" } });
  });

  it("edits free objects below the root as JSON, groups as their fields", () => {
    expect(uiFor({ type: "object" })).toEqual({});
    expect(uiFor({ type: "object", properties: { meta: { type: "object" }, end: { type: "object", properties: { rua: { type: "string" } } } } })).toEqual({ meta: { "ui:field": "json" } });
  });
});

describe("issueErrors", () => {
  it("translates by keyword and keeps the server text otherwise", () => {
    const pt = (k: string) => ({ required: "Campo obrigatório." })[k];
    expect(issueErrors([{ path: "/nome", keyword: "required", message: "missing property" }, { path: "/dia", keyword: "x-unknown", message: "raw" }], pt)).toEqual({
      nome: { __errors: ["Campo obrigatório."] },
      dia: { __errors: ["raw"] },
    });
  });

  it("puts each server issue under its field", () => {
    expect(issueErrors([{ path: "/parametros/0", message: "falta nome" }, { path: "", message: "raiz" }])).toEqual({
      __errors: ["raiz"],
      parametros: { 0: { __errors: ["falta nome"] } },
    });
  });
});
