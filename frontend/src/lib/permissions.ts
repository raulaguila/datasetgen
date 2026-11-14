import type { Action, Me, MemberRole } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";

/** Mirrors the backend rule for UX only: the API still decides every request. */
export function can(user: Me | null | undefined, resource: string, action: Action): boolean {
  if (!user) return false;
  if (user.is_superuser || "*" in user.permissions) return true;
  return user.permissions[resource]?.includes(action) ?? false;
}

const RANK: Record<MemberRole, number> = { viewer: 1, editor: 2, owner: 3 };

/** Whether a collection role covers need (owner > editor > viewer). Mirrors the backend for UX only. */
export function roleAllows(role: MemberRole | undefined, need: MemberRole): boolean {
  return !!role && RANK[role] >= RANK[need];
}

/** Translated label and description of an RBAC resource; unknown codes fall back to the code and the server's text. */
export function resourceText(t: (key: MessageKey) => string, code: string, description = "") {
  const label = t(`resource.${code}` as MessageKey);
  const desc = t(`resource.${code}_desc` as MessageKey);
  return { label: label.startsWith("resource.") ? code : label, description: desc.startsWith("resource.") ? description : desc };
}

const BITS: [Action, number][] = [["create", 1], ["read", 2], ["update", 4], ["delete", 8]];

/** A role's grants as the audit log stores them ({"users": 6}), in words: "Usuários: Ler, Editar". */
export function permissionsText(t: (key: MessageKey) => string, grants: Record<string, number>): string {
  const parts = Object.entries(grants)
    .filter(([, bits]) => bits > 0)
    .map(([code, bits]) => {
      const label = code === "*" ? t("roles.all_permissions") : resourceText(t, code).label;
      const actions = BITS.filter(([, bit]) => (bits & bit) === bit).map(([a]) => t(`action.${a}` as MessageKey));
      return `${label}: ${actions.join(", ")}`;
    });
  return parts.length ? parts.join("; ") : t("audit.no_permissions");
}
