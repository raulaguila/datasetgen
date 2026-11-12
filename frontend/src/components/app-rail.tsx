"use client";

import { useEffect, useId, useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Activity, ChevronDown, ChevronRight, Plus, Settings, Star } from "lucide-react";
import { BrandMark } from "@/components/brand-mark";
import { CollectionAvatar } from "@/components/collection-avatar";
import { BrandWordmark } from "@/components/brand-wordmark";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { collections } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { COLLECTION_MONITORING_NAV, COLLECTION_NAV, COLLECTION_SECTIONS, COLLECTION_SETTINGS_NAV, type CollectionSection } from "@/lib/navigation";
import { can, roleAllows } from "@/lib/permissions";
import { useUIStore } from "@/lib/store/ui";

type Group = "collection" | "monitoring" | "settings";

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
 * listed at the top, the selected one opening its work sections (records,
 * fields) below it; its monitoring and settings form groups of their own under
 * "New collection", each shown when the caller's role reaches one of its items. One group is open at a time, the one the page is in.
 * Platform administration lives in the account menu.
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
  // Pinned first, each part in the API's order (by name).
  const all = list.data?.items ?? [];
  const items = [...all.filter((c) => pinned.includes(c.id)), ...all.filter((c) => !pinned.includes(c.id))];
  const storedId = useUIStore((s) => s.collectionId);
  // A collection page names its collection; a record page leaves it to the store.
  const activeId = collectionInPath(pathname) ?? storedId;
  const active = items.find((c) => c.id === activeId);
  const base = active ? `/collections/${active.id}` : "";
  const allowed = (s: CollectionSection) => !s.need || roleAllows(active?.role, s.need);
  const sections = COLLECTION_NAV.filter(allowed);
  const groups = ([
    { group: "monitoring", label: t("collections.tab.monitoring"), icon: Activity, items: COLLECTION_MONITORING_NAV.filter(allowed) },
    { group: "settings", label: t("collections.tab.settings"), icon: Settings, items: COLLECTION_SETTINGS_NAV.filter(allowed) },
  ] as const).filter((g) => g.items.length > 0);
  const pageGroup = base ? groups.find((g) => g.items.some((s) => pathname === base + s.seg))?.group : undefined;
  const inCollection = !!base && !pageGroup && (pathname === base || pathname.startsWith(base + "/") || pathname.startsWith("/records/"));
  const routeGroup: Group | null = pageGroup ?? (inCollection ? "collection" : null);
  // Each navigation opens the page's group and closes the other; the settings toggle works in between.
  const context = `${pathname}|${active?.id}|${active?.role}|${routeGroup}`;
  const [disclosure, setDisclosure] = useState<{ context: string; open: Group[] }>({ context, open: routeGroup ? [routeGroup] : [] });
  let open = disclosure.open;
  if (disclosure.context !== context) {
    open = routeGroup ? [routeGroup] : [];
    setDisclosure({ context, open });
  }
  const toggle = (group: Group) => setDisclosure({ context, open: open.includes(group) ? open.filter((g) => g !== group) : [...open, group] });
  const enterGroup = (group: Group) => {
    setDisclosure({ context, open: [group] });
    onNavigate?.();
  };
  const groupId = useId();

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
  const link = (s: CollectionSection, group: Group) => {
    const href = base + s.seg;
    // Records is the collection's root; its record pages count as Records too.
    const current = s.seg ? pathname === href : pathname === base || pathname.startsWith("/records/");
    const name = t(s.key);
    return (
      <span key={s.seg} className="contents">
        {tooltip(name, (
          <Link href={href} aria-label={name} aria-current={current ? "page" : undefined} className="rail-item" onClick={() => enterGroup(group)}>
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
                      <Link href={`/collections/${c.id}/overview`} className="rail-item rail-workspace" aria-label={c.name} title={c.name} aria-current={selected ? "true" : undefined} onClick={() => enterGroup("collection")}>
                        <CollectionAvatar collection={c} className="rail-avatar" />
                        {!compact && <span className="rail-workspace-copy"><strong>{c.name}</strong><span>{count(c.record_count)}</span></span>}
                        {!compact && pinned.includes(c.id) && <Star className="rail-pin" fill="currentColor" aria-label={t("collections.pinned")} />}
                      </Link>
                    ))}
                    {selected && open.includes("collection") && (
                      <nav className="rail-workspace-navigation" aria-label={c.name}>{sections.map((s) => link(s, "collection"))}</nav>
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
            {active && groups.length > 0 && (
              <section className="rail-admin">
                {groups.map(({ group, label, icon: Icon, items: entries }) => (
                  <div key={`${active.id}:${group}`} className="rail-admin-group" data-open={open.includes(group) ? "" : undefined}>
                    {tooltip(`${label} · ${active.name}`, (
                      <button type="button" className="rail-item rail-administration" aria-label={label} aria-expanded={open.includes(group)} aria-controls={`${groupId}-${group}`}
                        data-active={pageGroup === group ? "" : undefined} onClick={() => toggle(group)}>
                        <Icon className="size-5" aria-hidden />
                        {!compact && <><span>{label}</span><ChevronDown className="rail-admin-chevron size-4" aria-hidden /></>}
                      </button>
                    ))}
                    <nav id={`${groupId}-${group}`} aria-label={label} hidden={!open.includes(group)}>{entries.map((s) => link(s, group))}</nav>
                  </div>
                ))}
              </section>
            )}
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
