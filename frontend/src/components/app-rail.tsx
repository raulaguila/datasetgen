"use client";

import { useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ChevronRight, Plus } from "lucide-react";
import { avatarStyle, initials } from "@/components/bits";
import { BrandMark } from "@/components/brand-mark";
import { BrandWordmark } from "@/components/brand-wordmark";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { collections } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { COLLECTION_NAV, COLLECTION_SETTINGS_NAV, type CollectionSection } from "@/lib/navigation";
import { can, roleAllows } from "@/lib/permissions";
import { useUIStore } from "@/lib/store/ui";

const WIDE = "(min-width: 1024px)";
const subscribeWide = (listener: () => void) => {
  const media = window.matchMedia(WIDE);
  media.addEventListener("change", listener);
  return () => media.removeEventListener("change", listener);
};

export const useWideRail = () => useSyncExternalStore(subscribeWide, () => window.matchMedia(WIDE).matches, () => true);

/** The collection a path belongs to: /collections/{id}/… (not "new"). */
function collectionInPath(pathname: string): string | undefined {
  const m = /^\/collections\/([^/]+)/.exec(pathname);
  return m && m[1] !== "new" ? m[1] : undefined;
}

/**
 * Floating sidebar, after the reference app's workspace rail: the collections
 * listed at the top, the selected one opening its sections and, for its owners,
 * its settings below it. Platform administration lives in the account menu.
 */
export function AppRail({ collapsed, onToggle, mobile = false, onNavigate }: { collapsed: boolean; onToggle: () => void; mobile?: boolean; onNavigate?: () => void }) {
  const { t } = useI18n();
  const { user } = useMe();
  const pathname = usePathname();
  const compact = collapsed && !mobile;
  const list = useQuery({
    queryKey: ["collections", "picker"],
    // shortcut: the rail lists the first 100 by name; past that, "Manage" has search.
    queryFn: ({ signal }) => collections.list({ page: 1, limit: 100 }, signal),
    enabled: !!user,
    staleTime: 30_000,
  });
  const items = list.data?.items ?? [];
  const storedId = useUIStore((s) => s.collectionId);
  // A collection page names its collection; a record page leaves it to the store.
  const activeId = collectionInPath(pathname) ?? storedId;
  const active = items.find((c) => c.id === activeId);
  const base = active ? `/collections/${active.id}` : "";
  const allowed = (s: CollectionSection) => !s.need || roleAllows(active?.role, s.need);
  const sections = COLLECTION_NAV.filter(allowed);
  const settings = COLLECTION_SETTINGS_NAV.filter(allowed);

  const inCollection = !!base && (pathname === base || pathname.startsWith(base + "/") || pathname.startsWith("/records/"));
  // Each navigation opens the selected collection's sections when the page is in it.
  const context = `${pathname}|${active?.id}`;
  const [disclosure, setDisclosure] = useState({ context, open: inCollection });
  let open = disclosure.open;
  if (disclosure.context !== context) {
    open = inCollection;
    setDisclosure({ context, open });
  }
  const enter = () => {
    setDisclosure({ context, open: true });
    onNavigate?.();
  };

  const tooltip = (label: string, child: React.ReactElement) =>
    compact ? <Tooltip><TooltipTrigger render={child} /><TooltipContent side="right">{label}</TooltipContent></Tooltip> : child;
  const link = (s: CollectionSection) => {
    const href = base + s.seg;
    // Records is the collection's root; its record pages count as Records too.
    const current = s.seg ? pathname === href : pathname === base || pathname.startsWith("/records/");
    const name = t(s.key);
    return (
      <span key={s.seg} className="contents">
        {tooltip(name, (
          <Link href={href} aria-label={name} aria-current={current ? "page" : undefined} className="rail-item" onClick={enter}>
            <s.icon className="size-4" aria-hidden />
            {!compact && <span>{name}</span>}
          </Link>
        ))}
      </span>
    );
  };
  const count = (n = 0) => (n === 1 ? t("nav.record_one") : t("nav.record_many").replace("{n}", String(n)));

  return (
    <aside data-ui-workspace-rail data-collapsed={compact ? "" : undefined} data-mobile={mobile ? "" : undefined} aria-label={t("layout.rail")}>
      {tooltip(t("nav.manage_collections"), (
        <Link href="/collections" className="rail-brand" aria-label={t("nav.manage_collections")} onClick={onNavigate}>
          <span className="rail-brand-mark"><BrandMark size={36} className="text-primary" /></span>
          <span className="rail-brand-wordmark"><BrandWordmark height={36} /></span>
        </Link>
      ))}
      <div className="rail-body">
        {user && (
          <>
            {!compact && (
              <div className="rail-group-heading">
                <span className="rail-label" aria-hidden>{t("nav.collections")}</span>
                <Link href="/collections" onClick={onNavigate}>{t("nav.manage")}</Link>
              </div>
            )}
            <ul aria-label={t("nav.collections")} className="rail-workspaces">
              {items.map((c) => {
                const selected = c.id === active?.id;
                return (
                  <li key={c.id} className="rail-workspace-group" data-selected={selected ? "" : undefined}>
                    {tooltip(c.name, (
                      <Link href={`/collections/${c.id}`} className="rail-item rail-workspace" aria-label={c.name} title={c.name} aria-current={selected ? "true" : undefined} onClick={enter}>
                        <span className="rail-avatar" style={avatarStyle(c.id)} aria-hidden>{initials(c.name)}</span>
                        {!compact && <span className="rail-workspace-copy"><strong>{c.name}</strong><span>{count(c.record_count)}</span></span>}
                      </Link>
                    ))}
                    {selected && open && (
                      <nav className="rail-workspace-navigation" aria-label={c.name}>
                        {sections.map(link)}
                        {settings.length > 0 && (
                          <>
                            {!compact && <span className="rail-label rail-sublabel" aria-hidden>{t("collections.tab.settings")}</span>}
                            <div role="group" aria-label={t("collections.tab.settings")} className="contents">{settings.map(link)}</div>
                          </>
                        )}
                      </nav>
                    )}
                  </li>
                );
              })}
            </ul>
            {can(user, "collections", "create") && tooltip(t("collections.new"), (
              <Link href="/collections/new" className="rail-item rail-create" aria-label={t("collections.new")} onClick={onNavigate}>
                <Plus className="size-5" aria-hidden />
                {!compact && <span>{t("collections.new")}</span>}
              </Link>
            ))}
          </>
        )}
      </div>
      {!mobile && (
        <div className="rail-footer">
          <button type="button" className="rail-item rail-toggle" aria-label={t("layout.toggle_sidebar")} title={t(compact ? "layout.expand" : "layout.collapse")} aria-expanded={!compact} onClick={onToggle}>
            <ChevronRight className="size-5" style={compact ? undefined : { transform: "rotate(180deg)" }} aria-hidden />
            {!compact && <span>{t("layout.collapse")}</span>}
          </button>
        </div>
      )}
    </aside>
  );
}
