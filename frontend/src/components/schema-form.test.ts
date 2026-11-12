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
});

describe("issueErrors", () => {
  it("puts each server issue under its field", () => {
    expect(issueErrors([{ path: "/parametros/0", message: "falta nome" }, { path: "", message: "raiz" }])).toEqual({
      __errors: ["raiz"],
      parametros: { 0: { __errors: ["falta nome"] } },
    });
  });
});
