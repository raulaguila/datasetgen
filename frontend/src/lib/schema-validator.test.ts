import { afterEach, describe, expect, it, vi } from "vitest";
import type { RJSFSchema } from "@rjsf/utils";
import { draftOf, schemaValidator } from "./schema-validator";

const wired: RJSFSchema = {
  $schema: "https://json-schema.org/draft/2020-12/schema",
  type: "object",
  required: ["nome", "categoria"],
  properties: {
    nome: { type: "string", minLength: 2 },
    categoria: { enum: ["gatilho", "efeito"] },
    dia: { type: "string", format: "date" },
    parametros: { type: "array", items: { type: "object", required: ["tipo"] } },
  },
};
const pt = schemaValidator((k) => ({ required: "Campo obrigatório." })[k]);

afterEach(() => vi.unstubAllGlobals());

describe("schemaValidator", () => {
  it("puts each broken rule under its field, translated when it can", () => {
    const { errors, errorSchema } = pt.validateFormData({ nome: "x", dia: "ontem", parametros: [{}] }, wired);
    expect(errors.map((e) => [e.property, e.name])).toEqual([
      [".categoria", "required"],
      [".nome", "minLength"],
      [".dia", "format"],
      [".parametros.0.tipo", "required"],
    ]);
    expect(errorSchema).toMatchObject({ categoria: { __errors: ["Campo obrigatório."] }, parametros: { 0: { tipo: { __errors: ["Campo obrigatório."] } } } });
  });

  // The production CSP has no 'unsafe-eval'; ajv broke there, this must not.
  it("never compiles code", () => {
    vi.stubGlobal("Function", () => { throw new EvalError("blocked by CSP"); });
    expect(pt.isValid(wired, { nome: "Ao clicar", categoria: "gatilho" }, wired)).toBe(true);
    expect(pt.validateFormData({}, wired).errors).toHaveLength(2);
  });

  it("reads the draft from $schema like the server", () => {
    expect(draftOf({})).toBe("2020-12");
    expect(draftOf({ $schema: "http://json-schema.org/draft-07/schema#" })).toBe("7");
  });
});
