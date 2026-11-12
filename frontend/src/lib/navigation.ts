import { Database, FileJson, Package, KeyRound, ListChecks, ListTree, ScrollText, Settings2, UserCog, Users, type LucideIcon } from "lucide-react";
import type { Action, MemberRole } from "@/lib/api/types";
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
  { href: "/products", key: "nav.products", icon: Package, needs: ["products", "read"], template: true },
  { href: "/collections", key: "nav.collections", icon: Database },
];

export const ADMIN_NAV: NavItem[] = [
  { href: "/admin/users", key: "nav.users", icon: Users, needs: ["users", "read"] },
  { href: "/admin/roles", key: "nav.roles", icon: KeyRound, needs: ["roles", "read"] },
  { href: "/admin/audit", key: "nav.audit", icon: ScrollText, needs: ["audit", "read"] },
];

export function isActive(pathname: string, href: string): boolean {
  return href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(href + "/");
}

export interface CollectionSection {
  seg: string;
  key: MessageKey;
  icon: LucideIcon;
  /** The least role in the collection that sees it; absent means every member. */
  need?: MemberRole;
}

/** The sections of one collection, under the collection picked in the sidebar. */
export const COLLECTION_NAV: CollectionSection[] = [
  { seg: "", key: "collections.tab.records", icon: FileJson },
  { seg: "/fields", key: "collections.tab.fields", icon: ListTree },
];

/** How the collection is doing, its "Monitoring" group in the sidebar. */
export const COLLECTION_MONITORING_NAV: CollectionSection[] = [
  { seg: "/quality", key: "collections.tab.quality", icon: ListChecks, need: "editor" },
  { seg: "/audit", key: "collections.tab.audit", icon: ScrollText, need: "owner" },
];

/** The collection's own administration, its "Settings" group in the sidebar. */
export const COLLECTION_SETTINGS_NAV: CollectionSection[] = [
  { seg: "/settings", key: "collections.tab.general", icon: Settings2, need: "owner" },
  { seg: "/members", key: "collections.tab.members", icon: UserCog, need: "owner" },
];
