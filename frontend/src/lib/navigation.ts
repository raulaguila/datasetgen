import { ChartColumn, Database, FileJson, LayoutDashboard, ListChecks, ListTree, Plug, ScrollText, Trash2, UserCog, Users, Zap, type LucideIcon } from "lucide-react";
import type { Action, MemberRole } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";
import { collectionInPath } from "@/lib/paths";

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
  { href: "/admin/audit", key: "nav.audit", icon: ScrollText, needs: ["audit", "read"] },
];

/** The pages of the person's own account, reached from the avatar menu. */
export const ACCOUNT_PAGES = ["/profile", "/api-access", "/notifications"];

export function isActive(pathname: string, href: string): boolean {
  if (href === "/collections" && collectionInPath(pathname)) return true; // a collection lives at /<owner>/<slug>
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

/** A collection's pages in the sidebar, in three labelled sections that are always open. */
export const COLLECTION_GROUPS: { key: MessageKey; items: CollectionSection[] }[] = [
  {
    key: "nav.section.data",
    items: [
      { seg: "/overview", key: "collections.tab.overview", icon: LayoutDashboard, shortcut: "o" },
      { seg: "", key: "collections.tab.records", icon: FileJson, shortcut: "r" },
      { seg: "/fields", key: "collections.tab.fields", icon: ListTree, shortcut: "f" },
      { seg: "/analysis", key: "collections.tab.analysis", icon: ChartColumn, shortcut: "n" },
      { seg: "/quality", key: "collections.tab.quality", icon: ListChecks, need: "editor", shortcut: "q" },
    ],
  },
  {
    key: "nav.section.flows",
    items: [
      { seg: "/automations", key: "collections.tab.automations", icon: Zap, need: "owner", shortcut: "u" },
      { seg: "/integrations", key: "collections.tab.integrations", icon: Plug, need: "owner", shortcut: "i" },
    ],
  },
  {
    key: "nav.section.manage",
    items: [
      { seg: "/members", key: "collections.tab.members", icon: UserCog, need: "owner", shortcut: "m" },
      { seg: "/audit", key: "collections.tab.audit", icon: ScrollText, need: "owner", shortcut: "a" },
      { seg: "/trash", key: "collections.tab.trash", icon: Trash2, need: "editor", shortcut: "t" },
    ],
  },
];

/** Every section of a collection, for shortcuts and the palette. */
export const COLLECTION_SECTIONS: CollectionSection[] = COLLECTION_GROUPS.flatMap((g) => g.items);
