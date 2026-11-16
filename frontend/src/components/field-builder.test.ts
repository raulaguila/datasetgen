import { describe, expect, it } from "vitest";
import { addField, blankField, duplicateField, fieldProblem, fieldsOf, keyOf, moveField, moveFieldTo, removeField, restoreField, schemaChanges, setProp, shows, typeOf, updateField, type Field } from "./field-builder";

const field = (patch: Partial<Field>): Field => ({ ...blankField, ...patch });

it("makes storage keys from labels", () => {
  expect(keyOf(" Nome do Cliente ")).toBe("nome_do_cliente");
  expect(keyOf("Ação/Reação")).toBe("acao_reacao");
  expect(keyOf("2 lugar")).toBe("f_2_lugar");
  expect(keyOf("—")).toBe("");
});

it("adds a field under its key and keeps the rest of the schema", () => {
  const start = { type: "object", title: "Wired", required: ["nome"], properties: { nome: { type: "string" } } };
  const next = addField(start, field({ name: " Categoria ", key: "categoria", type: "enum", required: true, options: "gatilho, efeito,,", default: "efeito" }));
  expect(next).toEqual({
    type: "object",
    title: "Wired",
    required: ["nome", "categoria"],
    properties: { nome: { type: "string" }, categoria: { title: "Categoria", type: "string", enum: ["gatilho", "efeito"], default: "efeito" } },
  });
  // Without a key typed, the label gives one.
  expect(addField({}, field({ name: "Tags", type: "strings", min: "1", default: "a, b" }))).toEqual({
    type: "object",
    properties: { tags: { title: "Tags", type: "array", items: { type: "string" }, minItems: 1, default: ["a", "b"] } },
  });
});

it("writes formats, help, placeholder and limits", () => {
  const s = addField({}, field({ name: "Quando", key: "quando", type: "datetime" }));
  expect((s.properties as Record<string, unknown>).quando).toEqual({ title: "Quando", type: "string", format: "date-time" });
  const t = addField({}, field({ name: "Resumo", key: "resumo", type: "text", description: "Uma frase", placeholder: "Ex.: dano em área", min: "10" }));
  expect((t.properties as Record<string, unknown>).resumo).toEqual({ title: "Resumo", type: "string", description: "Uma frase", examples: ["Ex.: dano em área"], minLength: 10, maxLength: 5000 });
  const n = addField({}, field({ name: "Nota", key: "nota", type: "integer", min: "0", max: "10", default: "5" }));
  expect((n.properties as Record<string, unknown>).nota).toEqual({ title: "Nota", type: "integer", default: 5, minimum: 0, maximum: 10 });
});

const wired = {
  type: "object",
  required: ["nome"],
  properties: {
    nome: { type: "string", title: "Nome", minLength: 1, pattern: "^[A-Z]" },
    descricao: { type: "string", title: "Descrição", maxLength: 5000, description: "Ajuda" },
    categoria: { type: "string", enum: ["gatilho", "efeito"] },
    site: { type: "string", format: "uri" },
    params: { type: "object", properties: { a: { type: "object", properties: {} } } },
  },
};

describe("fieldsOf", () => {
  it("lists fields with the type the builder would draw and their rules", () => {
    expect(fieldsOf(wired).map((r) => [r.key, r.field.name, r.field.type, r.field.required, r.advanced])).toEqual([
      ["nome", "Nome", "string", true, false],
      ["descricao", "Descrição", "text", false, false],
      ["categoria", "categoria", "enum", false, false],
      ["site", "site", "url", false, false],
      ["params", "params", "string", false, true],
    ]);
    const [nome, descricao, categoria] = fieldsOf(wired).map((r) => r.field);
    expect([nome.min, nome.max, descricao.max, descricao.description, categoria.options]).toEqual(["1", "", "", "Ajuda", "gatilho, efeito"]);
  });
});

describe("updateField", () => {
  it("edits in place, keeping keywords the builder does not know", () => {
    const [nome] = fieldsOf(wired).map((r) => r.field);
    const next = updateField(wired, "nome", { ...nome, name: "Nome do wired", required: false, min: "" });
    expect(Object.keys(next.properties as object)).toEqual(["nome", "descricao", "categoria", "site", "params"]);
    expect((next.properties as Record<string, unknown>).nome).toEqual({ title: "Nome do wired", type: "string", pattern: "^[A-Z]" });
    expect(next.required).toBeUndefined();
  });

  it("starts a new type fresh and can move the field to a new key", () => {
    const categoria = fieldsOf(wired)[2].field;
    const next = updateField(wired, "categoria", { ...categoria, name: "Categoria", key: "tipo", type: "text", required: true, options: "" });
    expect(Object.keys(next.properties as object)).toEqual(["nome", "descricao", "tipo", "site", "params"]);
    expect((next.properties as Record<string, unknown>).tipo).toEqual({ title: "Categoria", type: "string", maxLength: 5000 });
    expect(next.required).toEqual(["nome", "tipo"]);
  });
});

describe("fieldProblem", () => {
  const free = () => false;
  it("names what blocks saving", () => {
    expect(fieldProblem(field({ key: "x" }), free)).toBe("collections.field_name_needed");
    expect(fieldProblem(field({ name: "A", key: "Nome Errado" }), free)).toBe("collections.field_key_invalid");
    expect(fieldProblem(field({ name: "A", key: "a" }), (k) => k === "a")).toBe("collections.field_exists");
    expect(fieldProblem(field({ name: "A", key: "a", type: "enum" }), free)).toBe("collections.field_options_needed");
    expect(fieldProblem(field({ name: "A", key: "a", min: "5", max: "2" }), free)).toBe("collections.field_limits_invalid");
    expect(fieldProblem(field({ name: "A", key: "a", min: "1.5" }), free)).toBe("collections.field_limits_invalid");
    expect(fieldProblem(field({ name: "A", key: "a", type: "text", max: "100" }), free)).toBe("collections.field_text_max");
    expect(fieldProblem(field({ name: "A", key: "a", max: "800" }), free)).toBe("collections.field_string_max");
    expect(fieldProblem(field({ name: "A", key: "a", type: "integer", default: "2.5" }), free)).toBe("collections.field_default_invalid");
    expect(fieldProblem(field({ name: "A", key: "a", type: "enum", options: "x, y", default: "z" }), free)).toBe("collections.field_default_invalid");
    expect(fieldProblem(field({ name: "A", key: "a", max: "3", default: "longo" }), free)).toBe("collections.field_default_invalid");
    expect(fieldProblem(field({ name: "A", key: "a", type: "number", min: "-1.5", default: "-1" }), free)).toBeNull();
  });

  it("lets an existing field keep a key the format would refuse", () => {
    expect(fieldProblem(field({ name: "A", key: "Nome antigo" }), (k) => k === "Nome antigo", "Nome antigo")).toBeNull();
  });
});

it("removes, reorders and restores fields", () => {
  const gone = removeField(wired, "nome");
  expect(Object.keys(gone.properties as object)).toEqual(["descricao", "categoria", "site", "params"]);
  expect(gone.required).toBeUndefined();
  expect(Object.keys(moveField(wired, "descricao", -1).properties as object)).toEqual(["descricao", "nome", "categoria", "site", "params"]);
  expect(moveField(wired, "nome", -1)).toBe(wired);
  expect(Object.keys(moveFieldTo(wired, "params", 0).properties as object)).toEqual(["params", "nome", "descricao", "categoria", "site"]);
  const back = restoreField(gone, wired, "nome");
  expect(Object.keys(back.properties as object)).toEqual(["nome", "descricao", "categoria", "site", "params"]);
  expect(back.required).toEqual(["nome"]);
});

it("summarizes what a save changes", () => {
  const descricao = fieldsOf(wired)[1].field;
  let next = removeField(wired, "site");
  next = updateField(next, "descricao", { ...descricao, required: true });
  next = addField(next, field({ name: "Peso", key: "peso", type: "number" }));
  expect(schemaChanges(wired, next)).toEqual([
    { kind: "required", name: "Descrição", risky: true },
    { kind: "added", name: "Peso", risky: false },
    { kind: "removed", name: "site", risky: true },
  ]);
  expect(schemaChanges(wired, wired)).toEqual([]);
});

describe("groups and free objects", () => {
  it("knows a group of plain subfields and a free object", () => {
    expect(typeOf({ type: "object", properties: { rua: { type: "string" } } })).toBe("group");
    expect(typeOf({ type: "object" })).toBe("json");
    // A group inside a group, or closed objects, stay in JSON.
    expect(typeOf({ type: "object", properties: { a: { type: "object", properties: {} } } })).toBeUndefined();
    expect(typeOf({ type: "object", additionalProperties: false })).toBeUndefined();
    expect(typeOf({ type: "object", properties: { extra: { type: "object" } } })).toBe("group");
  });

  it("adds a group, fills it and keeps its subfields when renamed", () => {
    let s = addField({}, field({ name: "Endereço", key: "endereco", type: "group", required: true }));
    s = addField(s, field({ name: "Meta", key: "meta", type: "json" }));
    expect(s.properties).toEqual({ endereco: { title: "Endereço", type: "object", properties: {} }, meta: { title: "Meta", type: "object" } });
    const group = (s.properties as Record<string, Record<string, unknown>>).endereco;
    s = setProp(s, "endereco", addField(group, field({ name: "Cidade", key: "cidade", required: true })));
    s = updateField(s, "endereco", { ...fieldsOf(s)[0].field, name: "Endereço de entrega" });
    expect((s.properties as Record<string, unknown>).endereco).toEqual({
      title: "Endereço de entrega", type: "object", properties: { cidade: { title: "Cidade", type: "string" } }, required: ["cidade"],
    });
    expect(s.required).toEqual(["endereco"]);
    expect(fieldsOf(s).map((r) => [r.field.type, r.advanced])).toEqual([["group", false], ["json", false]]);
  });

  it("names subfield changes through their group", () => {
    const before = { type: "object", properties: { end: { title: "Endereço", type: "object", properties: { rua: { title: "Rua", type: "string" } } } } };
    const after = setProp(before, "end", addField(removeField(before.properties.end, "rua"), field({ name: "CEP", key: "cep" })));
    expect(schemaChanges(before, after)).toEqual([
      { kind: "added", name: "Endereço › CEP", risky: false },
      { kind: "removed", name: "Endereço › Rua", risky: true },
    ]);
  });
});

it("marks top-level plain fields unique and reports turning it on", () => {
  const s = addField({}, field({ name: "Código", key: "codigo", unique: true }));
  expect((s.properties as Record<string, unknown>).codigo).toEqual({ title: "Código", type: "string", "x-unique": true });
  // Lists and objects cannot be compared across records: the mark is dropped.
  expect((addField({}, field({ name: "Tags", key: "tags", type: "strings", unique: true })).properties as Record<string, unknown>).tags).not.toHaveProperty("x-unique");
  const before = addField({}, field({ name: "Nome", key: "nome" }));
  const after = updateField(before, "nome", { ...fieldsOf(before)[0].field, unique: true });
  expect(fieldsOf(after)[0].field.unique).toBe(true);
  expect(schemaChanges(before, after)).toEqual([{ kind: "unique", name: "Nome", risky: false }]);
});

describe("list of objects", () => {
  it("is built empty, takes subfields with a reference, and reads back", () => {
    let s = addField({ type: "object" }, field({ name: "Passos", type: "objects" }));
    expect(s.properties).toEqual({ passos: { title: "Passos", type: "array", items: { type: "object", properties: {} } } });
    const passos = (s.properties as Record<string, Record<string, unknown>>).passos;
    const item = addField(addField(passos.items as Record<string, unknown>, field({ name: "Wired", type: "reference", ref: "c1" })), field({ name: "Configuração", type: "text" }));
    s = setProp(s, "passos", { ...passos, items: item });
    expect(typeOf((s.properties as Record<string, unknown>).passos)).toBe("objects");
    // Editing the list's own name keeps its subfields.
    s = updateField(s, "passos", field({ name: "Etapas", key: "passos", type: "objects" }));
    const items = (s.properties as Record<string, { items: { properties: Record<string, unknown> } }>).passos.items;
    expect(Object.keys(items.properties)).toEqual(["wired", "configuracao"]);
    expect(schemaChanges({ type: "object", properties: { passos } }, s).map((c) => c.name)).toContain("Etapas › Wired");
  });

  it("is not offered for items holding a group", () => {
    expect(typeOf({ type: "array", items: { type: "object", properties: { g: { type: "object", properties: {} } } } })).toBeUndefined();
  });
});

describe("lookups and rollups", () => {
  it("write the server's keywords and read back the same settings", () => {
    let s = addField({ type: "object" }, field({ name: "Cliente", type: "reference", ref: "c1" }));
    s = addField(s, field({ name: "Nome do cliente", type: "lookup", via: "cliente", source: "nome", required: true }));
    s = addField(s, field({ name: "Gasto", type: "rollup", ref: "p1", via: "cliente", fn: "sum", source: "total" }));
    s = addField(s, field({ name: "Pedidos", type: "rollup", ref: "p1", via: "cliente", fn: "count", source: "total" }));
    const props = s.properties as Record<string, Record<string, unknown>>;
    expect(props.nome_do_cliente["x-lookup"]).toEqual({ ref: "cliente", field: "nome" });
    expect(props.gasto["x-rollup"]).toEqual({ collection: "p1", ref: "cliente", fn: "sum", field: "total" });
    expect(props.pedidos["x-rollup"]).toEqual({ collection: "p1", ref: "cliente", fn: "count" });
    expect(s.required).toBeUndefined(); // computed fields are never required
    const rows = fieldsOf(s);
    expect(rows.map((r) => r.field.type)).toEqual(["reference", "lookup", "rollup", "rollup"]);
    expect(rows[2].field).toMatchObject({ ref: "p1", via: "cliente", fn: "sum", source: "total" });
  });

  it("need every setting", () => {
    expect(fieldProblem(field({ name: "x", key: "x", type: "lookup", via: "cliente" }), () => false)).toBe("collections.field_lookup_needed");
    expect(fieldProblem(field({ name: "x", key: "x", type: "rollup", ref: "p1", via: "cliente", fn: "sum" }), () => false)).toBe("collections.field_rollup_needed");
    expect(fieldProblem(field({ name: "x", key: "x", type: "rollup", ref: "p1", via: "cliente", fn: "count" }), () => false)).toBeNull();
  });
});

describe("form layout and option colors", () => {
  it("round-trip colors, pattern, today, section, width and condition", () => {
    let s = addField({ type: "object" }, field({ name: "Status", type: "enum", options: "Aberto, Cancelado", colors: { Aberto: "green", Cancelado: "red", Sumiu: "blue" } }));
    s = addField(s, field({ name: "Motivo", pattern: "^\\w+$", section: "Encerramento", width: "half", showWhen: { field: "status", op: "eq", value: "Cancelado" } }));
    s = addField(s, field({ name: "Quando", type: "date", today: true }));
    const props = s.properties as Record<string, Record<string, unknown>>;
    expect(props.status["x-colors"]).toEqual({ Aberto: "green", Cancelado: "red" }); // no color for a missing option
    expect(props.motivo).toMatchObject({ pattern: "^\\w+$", "x-section": "Encerramento", "x-width": "half", "x-show-when": { field: "status", op: "eq", value: "Cancelado" } });
    expect(props.quando["x-default"]).toBe("today");
    const [st, mo, qu] = fieldsOf(s).map((r) => r.field);
    expect(st.colors).toEqual({ Aberto: "green", Cancelado: "red" });
    expect(mo).toMatchObject({ pattern: "^\\w+$", section: "Encerramento", width: "half", showWhen: { field: "status", op: "eq", value: "Cancelado" } });
    expect(qu.today).toBe(true);
  });

  it("check pattern and condition", () => {
    const ok = field({ name: "x", key: "x" });
    expect(fieldProblem({ ...ok, pattern: "(" }, () => false)).toBe("collections.field_pattern_invalid");
    expect(fieldProblem({ ...ok, showWhen: { field: "", op: "filled", value: "" } }, () => false)).toBe("collections.field_show_needed");
    expect(fieldProblem({ ...ok, showWhen: { field: "s", op: "eq", value: "" } }, () => false)).toBe("collections.field_show_needed");
    expect(fieldProblem({ ...ok, required: true, showWhen: { field: "s", op: "filled", value: "" } }, () => false)).toBe("collections.field_show_required");
    expect(fieldProblem({ ...ok, showWhen: { field: "s", op: "filled", value: "" } }, () => false)).toBeNull();
  });

  it("evaluate a condition", () => {
    expect(shows(null, {})).toBe(true);
    expect(shows({ field: "s", op: "eq", value: "Cancelado" }, { s: "Cancelado" })).toBe(true);
    expect(shows({ field: "s", op: "eq", value: "Cancelado" }, { s: "Aberto" })).toBe(false);
    expect(shows({ field: "s", op: "neq", value: "Cancelado" }, {})).toBe(true);
    expect(shows({ field: "s", op: "filled", value: "" }, { s: "" })).toBe(false);
    expect(shows({ field: "s", op: "empty", value: "" }, {})).toBe(true);
    expect(shows({ field: "b", op: "eq", value: "true" }, { b: true })).toBe(true);
    expect(shows({ field: "t", op: "eq", value: "a" }, { t: ["a", "b"] })).toBe(true);
  });

  it("duplicate a field right after itself, under a free key, not unique", () => {
    let s = addField({ type: "object" }, field({ name: "Código", unique: true }));
    s = addField(s, field({ name: "Fim" }));
    s = duplicateField(s, "codigo", "(cópia)");
    s = duplicateField(s, "codigo", "(cópia)");
    const rows = fieldsOf(s);
    expect(rows.map((r) => r.key)).toEqual(["codigo", "codigo_3", "codigo_2", "fim"]);
    expect(rows[2].field).toMatchObject({ name: "Código (cópia)", unique: false });
  });

  it("call a display-only change safe", () => {
    const before = addField({ type: "object" }, field({ name: "Status", type: "enum", options: "A, B" }));
    const after = updateField(before, "status", field({ name: "Status", key: "status", type: "enum", options: "A, B", colors: { A: "red" }, section: "Geral" }));
    expect(schemaChanges(before, after)).toEqual([{ kind: "display", name: "Status", risky: false }]);
    const tighter = updateField(before, "status", field({ name: "Status", key: "status", type: "enum", options: "A" }));
    expect(schemaChanges(before, tighter)[0]).toMatchObject({ kind: "rules", risky: true });
  });
});
