import { describe, expect, it } from "vitest";
import type { Me } from "@/lib/api/types";
import { can, permissionsText } from "./permissions";

const user = (over: Partial<Me>): Me => ({
  id: "1", name: "A", email: "a@x", status: "active", created_at: "", is_superuser: false,
  permissions: {}, preferences: { theme: "system", language: "pt-BR" }, roles: [], ...over,
});

describe("can", () => {
  it("allows only the granted action", () => {
    const u = user({ permissions: { roles: ["read"] } });
    expect(can(u, "roles", "read")).toBe(true);
    expect(can(u, "roles", "delete")).toBe(false);
    expect(can(u, "users", "read")).toBe(false);
  });

  it("allows everything for the * sentinel and for superusers", () => {
    expect(can(user({ permissions: { "*": ["create", "read", "update", "delete"] } }), "users", "delete")).toBe(true);
    expect(can(user({ is_superuser: true }), "roles", "update")).toBe(true);
  });

  it("denies when nobody is signed in", () => expect(can(null, "users", "read")).toBe(false));
});

describe("permissionsText", () => {
  const t = (key: string) => ({ "resource.users": "Usuários", "action.read": "Ler", "action.update": "Editar", "roles.all_permissions": "Todas", "audit.no_permissions": "Nenhuma" })[key] ?? key;
  it("names each resource and its actions in bit order", () => {
    expect(permissionsText(t as never, { users: 6, audit: 0 })).toBe("Usuários: Ler, Editar");
  });
  it("keeps unknown codes and says when nothing is granted", () => {
    expect(permissionsText(t as never, { reports: 2 })).toBe("reports: Ler");
    expect(permissionsText(t as never, {})).toBe("Nenhuma");
  });
});
