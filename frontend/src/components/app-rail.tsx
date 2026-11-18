"use client";

import { useEffect, useSyncExternalStore } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ChevronRight, KeyRound, Plus, Star } from "lucide-react";
import { BrandMark } from "@/components/brand-mark";
import { CollectionAvatar } from "@/components/collection-avatar";
import { BrandWordmark } from "@/components/brand-wordmark";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { collections } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { ADMIN_NAV, COLLECTION_GROUPS, COLLECTION_SECTIONS, isActive, type CollectionSection } from "@/lib/navigation";
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
 * Floating sidebar: the collections listed at the top, the selected one opening
 * all of its pages below it in three labelled sections (data, flows, manage),
 * always open, each page shown when the caller's role reaches it. The pages of
 * the account and the platform close the rail, so every destination lives here.
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
  const pinned = useUIStore((s) => s.pinned);
  const storedId = useUIStore((s) => s.collectionId);
  // A collection page names its collection; a record page leaves it to the store.
  const activeId = collectionInPath(pathname) ?? storedId;
  // The open collection keeps its menu even when it is past the first 100.
  const missing = !!activeId && list.isSuccess && !list.data.items.some((c) => c.id === activeId);
  const extra = useQuery({ queryKey: ["collections", "detail", activeId], queryFn: () => collections.get(activeId!), enabled: missing, retry: false });
  // Pinned first, each part in the API's order (by name).
  const all = [...(list.data?.items ?? []), ...(missing && extra.data ? [extra.data] : [])];
  const items = [...all.filter((c) => pinned.includes(c.id)), ...all.filter((c) => !pinned.includes(c.id))];
  const active = items.find((c) => c.id === activeId);
  const base = active ? `/collections/${active.id}` : "";
  const allowed = (s: CollectionSection) => !s.need || roleAllows(active?.role, s.need);
  const groups = COLLECTION_GROUPS.map((g) => ({ ...g, items: g.items.filter(allowed) })).filter((g) => g.items.length > 0);
  const admin = ADMIN_NAV.filter((item) => !item.needs || can(user, ...item.needs));
  const globals = [{ href: "/api-access", key: "nav.api_access" as const, icon: KeyRound }, ...admin];

  // "g" then a section's key opens it in the selected collection, as in the reference app.
  const router = useRouter();
  useEffect(() => {
    if (mobile || !active) return;
    const keys = Object.fromEntries(COLLECTION_SECTIONS.filter((s) => s.shortcut && (!s.need || roleAllows(active.role, s.need))).map((s) => [s.shortcut!, s.seg]));
    let lastG = 0;
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const el = document.activeElement as HTMLElement | null;
      if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable)) return;
      if (e.key === "g") {
        lastG = e.timeStamp;
        return;
      }
      if (e.timeStamp - lastG < 1000 && e.key in keys) {
        lastG = 0;
        router.push(`/collections/${active.id}${keys[e.key]}`);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [mobile, active, router]);

  const tooltip = (label: string, child: React.ReactElement) =>
    compact ? <Tooltip><TooltipTrigger render={child} /><TooltipContent side="right">{label}</TooltipContent></Tooltip> : child;
  const link = (s: CollectionSection) => {
    const href = base + s.seg;
    // Records is the collection's root; its record pages count as Records too.
    const current = s.seg ? pathname === href : pathname === base || pathname.startsWith("/records/");
    const name = t(s.key);
    return (
      <span key={s.seg} className="contents">
        {tooltip(s.shortcut ? `${name} (g ${s.shortcut})` : name, (
          <Link href={href} aria-label={name} aria-keyshortcuts={s.shortcut && `g ${s.shortcut}`} title={s.shortcut && !compact ? `${name} (g ${s.shortcut})` : undefined} aria-current={current ? "page" : undefined} className="rail-item" onClick={onNavigate}>
            <s.icon className="size-4" aria-hidden />
            {!compact && <span>{name}</span>}
          </Link>
        ))}
      </span>
    );
  };
  const globalNav = (
    <nav className="rail-global" aria-label={t("nav.section.global")}>
      {globals.map((item) => (
        <span key={item.href} className="contents">
          {tooltip(t(item.key), (
            <Link href={item.href} aria-label={t(item.key)} aria-current={isActive(pathname, item.href) ? "page" : undefined} className="rail-item" onClick={onNavigate}>
              <item.icon className="size-4" aria-hidden />
              {!compact && <span>{t(item.key)}</span>}
            </Link>
          ))}
        </span>
      ))}
    </nav>
  );
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
                      <Link href={`/collections/${c.id}/overview`} className="rail-item rail-workspace" aria-label={c.name} title={c.name} aria-current={selected ? "true" : undefined} onClick={onNavigate}>
                        <CollectionAvatar collection={c} className="rail-avatar" />
                        {!compact && <span className="rail-workspace-copy"><strong>{c.name}</strong><span>{count(c.record_count)}</span></span>}
                        {!compact && pinned.includes(c.id) && <Star className="rail-pin" fill="currentColor" aria-label={t("collections.pinned")} />}
                      </Link>
                    ))}
                    {selected && (
                      <nav className="rail-workspace-navigation" aria-label={c.name}>
                        {groups.map((g) => (
                          <div key={g.key} role="group" aria-label={t(g.key)} className="rail-nav-section">
                            {!compact && <span className="rail-nav-label" aria-hidden>{t(g.key)}</span>}
                            {g.items.map(link)}
                          </div>
                        ))}
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
            {mobile && globalNav}
          </>
        )}
      </div>
      {!mobile && (
        <div className="rail-footer">
          {user && globalNav}
          <button type="button" className="rail-item rail-toggle" aria-label={t("layout.toggle_sidebar")} title={t(compact ? "layout.expand" : "layout.collapse")} aria-expanded={!compact} onClick={onToggle}>
            <ChevronRight className="size-5" style={compact ? undefined : { transform: "rotate(180deg)" }} aria-hidden />
            {!compact && <span>{t("layout.collapse")}</span>}
          </button>
        </div>
      )}
    </aside>
  );
}
