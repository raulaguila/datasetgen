import { ChartColumn, Database, FileJson, KeyRound, LayoutDashboard, ListChecks, ListTree, Plug, ScrollText, Settings2, Trash2, UserCog, Users, Zap, type LucideIcon } from "lucide-react";
import type { Action, MemberRole } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";

export interface NavItem {
  href: string;
  key: MessageKey;
  icon: LucideIcon;
  /** Shown only when the user may do this; absent means every signed in user. */
  needs?: [resource: string, action: Action];
}

export const MAIN_NAV: NavItem[] = [
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
  /** "g" then this key opens it (keyboard-shortcuts-dialog lists them). */
  shortcut?: string;
}

/** The sections of one collection, under the collection picked in the sidebar. */
export const COLLECTION_NAV: CollectionSection[] = [
  { seg: "/overview", key: "collections.tab.overview", icon: LayoutDashboard, shortcut: "o" },
  { seg: "", key: "collections.tab.records", icon: FileJson, shortcut: "r" },
  { seg: "/fields", key: "collections.tab.fields", icon: ListTree, shortcut: "f" },
  { seg: "/analysis", key: "collections.tab.analysis", icon: ChartColumn, shortcut: "n" },
];

/** How the collection is doing, its "Monitoring" group in the sidebar. */
export const COLLECTION_MONITORING_NAV: CollectionSection[] = [
  { seg: "/quality", key: "collections.tab.quality", icon: ListChecks, need: "editor", shortcut: "q" },
  { seg: "/audit", key: "collections.tab.audit", icon: ScrollText, need: "owner", shortcut: "a" },
  { seg: "/trash", key: "collections.tab.trash", icon: Trash2, need: "editor", shortcut: "t" },
];

/** The collection's own administration, its "Settings" group in the sidebar. */
export const COLLECTION_SETTINGS_NAV: CollectionSection[] = [
  { seg: "/settings", key: "collections.tab.general", icon: Settings2, need: "owner", shortcut: "," },
  { seg: "/members", key: "collections.tab.members", icon: UserCog, need: "owner", shortcut: "m" },
  { seg: "/automations", key: "collections.tab.automations", icon: Zap, need: "owner", shortcut: "u" },
  { seg: "/integrations", key: "collections.tab.integrations", icon: Plug, need: "owner", shortcut: "i" },
];

/** Every section of a collection, for shortcuts and the palette. */
export const COLLECTION_SECTIONS: CollectionSection[] = [...COLLECTION_NAV, ...COLLECTION_MONITORING_NAV, ...COLLECTION_SETTINGS_NAV];
