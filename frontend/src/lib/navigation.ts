import { Database, Home, KeyRound, Package, ScrollText, Users, type LucideIcon } from "lucide-react";
import type { Action } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";

export interface NavItem {
  href: string;
  key: MessageKey;
  icon: LucideIcon;
  /** Shown only when the user may do this; absent means every signed in user. */
  needs?: [resource: string, action: Action];
}

export const MAIN_NAV: NavItem[] = [
  { href: "/", key: "nav.home", icon: Home },
  { href: "/products", key: "nav.products", icon: Package, needs: ["products", "read"] },
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
