import { describe, expect, it } from "vitest";
import { parseRuleText, ruleText, suggestAt } from "@/components/rule-text";
import type { Entry, Kind, Where } from "@/components/rule-model";

const e = (key: string, name: string, kind: Kind, extra: Partial<Entry> = {}): Entry =>
  ({ key, name, kind, type: "string", options: [], colors: {}, target: "", stored: true, ...extra });
const entries: Entry[] = [
  e("nome", "Nome", "text"),
  e("idade", "Idade", "number"),
  e("venc", "Data de vencimento", "date"),
  e("status", "Status", "enum", { options: ["Aberto", "Pago"] }),
  e("pago", "Pago", "boolean"),
  e("tags", "Tags", "strings"),
  e("cli", "Cliente", "ref", { target: "c1" }),
];
const where = (src: string) => {
  const r = parseRuleText(src, entries);
  if ("error" in r) throw new Error(`${r.error.key} at ${r.error.at}`);
  return r.where;
};
const error = (src: string) => {
  const r = parseRuleText(src, entries);
  return "error" in r ? r.error : null;
};

describe("parseRuleText", () => {
  it("reads AND tighter than OR", () => {
    expect(where('nome = "Raul" e idade > 18 ou status = pago')).toEqual({
      any: true, rules: [{ field: "status", op: "eq", value: "Pago" }],
      groups: [{ any: false, rules: [{ field: "nome", op: "eq", value: "Raul" }, { field: "idade", op: "gt", value: 18 }] }],
    });
  });
  it("reads English words and brackets", () => {
    expect(where("idade >= 1 and (nome contains ana or nome starts with bia)")).toEqual({
      any: false, rules: [{ field: "idade", op: "gte", value: 1 }],
      groups: [{ any: true, rules: [{ field: "nome", op: "contains", value: "ana" }, { field: "nome", op: "starts_with", value: "bia" }] }],
    });
  });
  it("reads dates, ranges, lists and empty", () => {
    expect(where('"Data de vencimento" nos últimos 7 dias').rules[0]).toEqual({ field: "venc", op: "between", value: [{ today: -7 }, { today: 0 }] });
    expect(where("data de vencimento < hoje-3").rules[0]).toEqual({ field: "venc", op: "lt", value: { today: -3 } });
    expect(where("idade entre 1 e 5").rules[0]).toEqual({ field: "idade", op: "between", value: [1, 5] });
    expect(where("status não em (aberto, pago)").rules[0]).toEqual({ field: "status", op: "not_in", value: ["Aberto", "Pago"] });
    expect(where("tags = x").rules[0]).toEqual({ field: "tags", op: "has", value: "x" });
    expect(where("tags em (x, y)").rules[0]).toEqual({ field: "tags", op: "has_any", value: ["x", "y"] });
    expect(where("nome é vazio e pago = sim").rules).toEqual([{ field: "nome", op: "empty" }, { field: "pago", op: "eq", value: true }]);
  });
  it("hands back references written by name", () => {
    const r = parseRuleText('cliente = "Ana Prado"', entries);
    expect(r).toEqual({ where: { any: false, rules: [{ field: "cli", op: "eq", value: { $name: "Ana Prado" } }] }, refNames: [{ name: "Ana Prado", target: "c1", field: "cli" }] });
  });
  it("points at what is wrong", () => {
    expect(error("nada = 1")).toMatchObject({ key: "field", at: 0 });
    expect(error("idade = abc")).toMatchObject({ key: "number", at: 8 });
    expect(error("status = fechado")).toMatchObject({ key: "option" });
    expect(error("idade > 1 idade < 3")).toMatchObject({ key: "join", at: 10 });
    expect(error("(idade > 1")).toMatchObject({ key: "paren", at: 0 });
    expect(error("idade contém 1")).toMatchObject({ key: "op" });
    expect(error("(idade > 1 ou (idade < 0 e nome = a)) e pago = sim")).toMatchObject({ key: "nested" });
  });
});

describe("ruleText", () => {
  it("writes what it reads", () => {
    const cases = [
      'Status = "Pago" ou Nome = "Raul" e Idade > 18',
      '"Data de vencimento" nos últimos 7 dias e Tags tem algum ("a", "b")',
      'Idade entre 1 e 5 e (Nome contém "x" ou Pago = não)',
      '"Data de vencimento" < hoje-3 e Nome não é vazio',
    ];
    for (const src of cases) expect(ruleText(where(src), entries, "pt")).toBe(src);
  });
  it("names referenced records", () => {
    const w: Where = { any: false, rules: [{ field: "cli", op: "eq", value: "id1" }] };
    expect(ruleText(w, entries, "en", { id1: "Ana" })).toBe('Cliente = "Ana"');
  });
});

describe("suggestAt", () => {
  const at = (src: string) => suggestAt(src, src.length, entries, "pt").items;
  it("offers fields, then operators, values and joins", () => {
    expect(at("St")).toEqual(["Status"]);
    expect(at("Status ")).toContain("em");
    expect(at("Status = ")).toEqual(['"Aberto"', '"Pago"']);
    expect(at("Status = Pago ")).toEqual(["e", "ou"]);
    expect(at("Idade entre 1 ")).toEqual(["e"]);
  });
});
