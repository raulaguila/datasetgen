import { describe, expect, it } from "vitest";
import { issueErrors, uiFor, withToday } from "./schema-form";

describe("uiFor", () => {
  it("keeps unlimited long-text fields as text areas, nested too", () => {
    const text = { type: "string", "x-multiline": true };
    expect(uiFor({ type: "object", properties: { conteudo: text, grupo: { type: "object", properties: { texto: text } } } }))
      .toEqual({ conteudo: { "ui:widget": "textarea" }, grupo: { texto: { "ui:widget": "textarea" } } });
  });

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

  it("draws several picks from a fixed list as checkboxes", () => {
    const pick = { type: "array", uniqueItems: true, items: { type: "string", enum: ["a", "b"] } };
    expect(uiFor({ properties: { fontes: { type: "array", items: { type: "object", properties: { opcoes: pick } } }, tags: { ...pick, uniqueItems: false } } }))
      .toEqual({ fontes: { items: { opcoes: { "ui:widget": "checkboxes" } } } });
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

describe("uiFor in a list of objects", () => {
  it("picks records for a reference subfield", () => {
    expect(uiFor({ type: "object", properties: { passos: { type: "array", items: { type: "object", properties: { wired: { type: "string", "x-ref": "c1" } } } } } }))
      .toEqual({ passos: { items: { wired: { "ui:field": "reference" } } } });
  });
});

describe("form layout", () => {
  it("gives a field its width class", () => {
    const ui = uiFor({ type: "object", properties: { a: { type: "string", "x-width": "half" }, b: { type: "string", "x-width": "full" }, c: { type: "string" } } });
    expect(ui.a).toEqual({ "ui:classNames": "rjsf-half" });
    expect(ui.b).toEqual({ "ui:classNames": "rjsf-full" });
    expect(ui.c).toBeUndefined();
  });

  it("starts today fields at the local date, or now", () => {
    const now = new Date(2026, 10, 16, 9, 30);
    const s = withToday({ type: "object", properties: { d: { type: "string", format: "date", "x-default": "today" }, t: { type: "string", format: "date-time", "x-default": "today" }, n: { type: "string" } } }, now);
    const props = s.properties as Record<string, Record<string, unknown>>;
    expect(props.d.default).toBe("2026-11-16");
    expect(props.t.default).toBe(now.toISOString());
    expect(props.n.default).toBeUndefined();
    const plain = { type: "object", properties: { n: { type: "string" } } };
    expect(withToday(plain)).toBe(plain);
  });
});
