import { expect, it } from "vitest";
import { plural } from "./plural";

it("settles (s) endings by count", () => {
  expect(plural("{n} registro(s) criado(s)", 1)).toBe("1 registro criado");
  expect(plural("{n} registro(s) criado(s)", 3)).toBe("3 registros criados");
  expect(plural("{n} item(ns) excluído(s).", 2)).toBe("2 itens excluídos.");
  expect(plural("{n} item(ns) excluído(s).", 1)).toBe("1 item excluído.");
  expect(plural("{n} record(s)", 0, "0")).toBe("0 records");
  expect(plural("{n} condição(ões)", 2)).toBe("2 condições");
  expect(plural("{n} condição(ões)", 1)).toBe("1 condição");
});
