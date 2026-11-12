import { Database, FileJson, Package, Home, KeyRound, ListTree, ScrollText, Settings2, Users, type LucideIcon } from "lucide-react";
import type { Action } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";

export interface NavItem {
  href: string;
  key: MessageKey;
  icon: LucideIcon;
  /** Shown only when the user may do this; absent means every signed in user. */
  needs?: [resource: string, action: Action];
  /** The example module `make module` copies; never shown. Copies drop the flag. */
  template?: true;
}

export const MAIN_NAV: NavItem[] = [
  { href: "/", key: "nav.home", icon: Home },
  { href: "/products", key: "nav.products", icon: Package, needs: ["products", "read"], template: true },
  { href: "/collections", key: "nav.collections", icon: Database, needs: ["collections", "read"] },
];

export const ADMIN_NAV: NavItem[] = [
  { href: "/admin/users", key: "nav.users", icon: Users, needs: ["users", "read"] },
  { href: "/admin/roles", key: "nav.roles", icon: KeyRound, needs: ["roles", "read"] },
  { href: "/admin/audit", key: "nav.audit", icon: ScrollText, needs: ["audit", "read"] },
];

export function isActive(pathname: string, href: string): boolean {
  return href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(href + "/");
}

/** The sections of one collection, under the collection picked in the sidebar. */
export const COLLECTION_NAV: { seg: string; key: MessageKey; icon: LucideIcon; needs?: [resource: string, action: Action] }[] = [
  { seg: "", key: "collections.tab.records", icon: FileJson, needs: ["records", "read"] },
  { seg: "/fields", key: "collections.tab.fields", icon: ListTree },
  { seg: "/settings", key: "collections.tab.settings", icon: Settings2 },
];
