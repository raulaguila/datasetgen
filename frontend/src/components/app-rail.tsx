"use client";

import { useId, useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Check, ChevronDown, ChevronRight, ChevronsUpDown, Database, List, Plus, ShieldCheck } from "lucide-react";
import { BrandMark } from "@/components/brand-mark";
import { BrandWordmark } from "@/components/brand-wordmark";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { collections } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { ADMIN_NAV, COLLECTION_NAV, MAIN_NAV, isActive, type NavItem } from "@/lib/navigation";
import { can } from "@/lib/permissions";
import { useUIStore } from "@/lib/store/ui";

const WIDE = "(min-width: 1024px)";
const subscribeWide = (listener: () => void) => {
  const media = window.matchMedia(WIDE);
  media.addEventListener("change", listener);
  return () => media.removeEventListener("change", listener);
};

export const useWideRail = () => useSyncExternalStore(subscribeWide, () => window.matchMedia(WIDE).matches, () => true);

/** Floating sidebar: main links, then an administration group shown only to who may use it. */
export function AppRail({ collapsed, onToggle, mobile = false, onNavigate }: { collapsed: boolean; onToggle: () => void; mobile?: boolean; onNavigate?: () => void }) {
  const { t } = useI18n();
  const { user } = useMe();
  const pathname = usePathname();
  const compact = collapsed && !mobile;
  const visible = (items: NavItem[]) => items.filter((item) => !item.template && (!item.needs || can(user, ...item.needs)));
  const admin = visible(ADMIN_NAV);

  const tooltip = (label: string, child: React.ReactElement) =>
    compact ? <Tooltip><TooltipTrigger render={child} /><TooltipContent side="right">{label}</TooltipContent></Tooltip> : child;

  const link = (item: NavItem, className: string, iconClass: string) => {
    const label = t(item.key);
    // Inside one collection the group above is the current place, not the list of all.
    const current = item.href === "/collections" ? pathname === item.href : isActive(pathname, item.href);
    return (
      <span key={item.href} className="contents">
        {tooltip(label, (
          <Link href={item.href} aria-label={label} aria-current={current ? "page" : undefined} className={className} onClick={onNavigate}>
            <item.icon className={iconClass} aria-hidden />
            {!compact && <span>{label}</span>}
          </Link>
        ))}
      </span>
    );
  };
  const adminActive = admin.some((item) => isActive(pathname, item.href));
  // Each navigation opens the group on admin pages and closes it elsewhere; the toggle still works in between.
  // adminActive also flips once the permissions load after a direct visit to an admin page.
  const seen = `${pathname}|${adminActive}`;
  const [toggled, setToggled] = useState<boolean | null>(null);
  const [seenKey, setSeenKey] = useState(seen);
  if (seen !== seenKey) {
    setSeenKey(seen);
    setToggled(null);
  }
  const adminOpen = toggled ?? adminActive;
  const toggleAdmin = () => setToggled(!adminOpen);
  const adminNavId = useId();
  const adminLabel = t("nav.administration");

  return (
    <aside data-ui-workspace-rail data-collapsed={compact ? "" : undefined} data-mobile={mobile ? "" : undefined} aria-label={t("layout.rail")}>
      {tooltip(t("nav.home"), (
        <Link href="/" className="rail-brand" aria-label={t("nav.home")} onClick={onNavigate}>
          <span className="rail-brand-mark"><BrandMark size={36} className="text-primary" /></span>
          <span className="rail-brand-wordmark"><BrandWordmark height={36} /></span>
        </Link>
      ))}
      <div className="rail-body">
        {can(user, "collections", "read") && <CollectionGroup compact={compact} tooltip={tooltip} onNavigate={onNavigate} />}
        <nav className="rail-main" aria-label={t("layout.navigation")}>{visible(MAIN_NAV).map((item) => link(item, "rail-item rail-create", "size-6"))}</nav>
        {admin.length > 0 && (
          <section className="rail-admin">
            <div className="rail-admin-group" data-open={adminOpen ? "" : undefined}>
              {tooltip(adminLabel, (
                <button type="button" className="rail-item rail-administration" aria-label={adminLabel} aria-expanded={adminOpen} aria-controls={adminNavId}
                  data-active={adminActive ? "" : undefined} onClick={toggleAdmin}>
                  <ShieldCheck className="size-5" aria-hidden />
                  {!compact && <><span>{adminLabel}</span><ChevronDown className="rail-admin-chevron size-4" aria-hidden /></>}
                </button>
              ))}
              <nav id={adminNavId} aria-label={adminLabel} hidden={!adminOpen}>{admin.map((item) => link(item, "rail-item", "size-4"))}</nav>
            </div>
          </section>
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

/**
 * The collection being worked on, picked at the top of the rail, with its sections
 * below: the app's main context, as a workspace is elsewhere.
 */
function CollectionGroup({ compact, tooltip, onNavigate }: { compact: boolean; tooltip: (label: string, child: React.ReactElement) => React.ReactNode; onNavigate?: () => void }) {
  const { t } = useI18n();
  const { user } = useMe();
  const pathname = usePathname();
  const currentId = useUIStore((s) => s.collectionId);
  // shortcut: the picker lists the first 100 by name; past that, "All collections" has search.
  const list = useQuery({ queryKey: ["collections", "picker"], queryFn: ({ signal }) => collections.list({ page: 1, limit: 100 }, signal), staleTime: 30_000 });
  const items = list.data?.items ?? [];
  const current = items.find((c) => c.id === currentId);
  const base = current ? `/collections/${current.id}` : "";
  const label = current?.name ?? t("nav.pick_collection");
  const sections = COLLECTION_NAV.filter((s) => !s.needs || can(user, ...s.needs));

  return (
    <section className="rail-admin" aria-label={t("nav.collection")}>
      <div className="rail-admin-group" data-open={current ? "" : undefined}>
        <DropdownMenu>
          {tooltip(label, (
            <DropdownMenuTrigger className="rail-item rail-administration" aria-label={`${t("nav.collection")}: ${label}`} data-active={current && pathname.startsWith(base) ? "" : undefined}>
              <Database className="size-5" aria-hidden />
              {!compact && <><span className="truncate">{label}</span><ChevronsUpDown className="rail-admin-chevron size-4" aria-hidden /></>}
            </DropdownMenuTrigger>
          ))}
          <DropdownMenuContent align="start" side={compact ? "right" : "bottom"} className="w-60">
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t("nav.collections")}</DropdownMenuLabel>
              {items.map((c) => (
                <DropdownMenuItem key={c.id} render={<Link href={`/collections/${c.id}`} onClick={onNavigate} />}>
                  <span className="min-w-0 flex-1 truncate">{c.name}</span>
                  {c.id === current?.id && <Check className="size-4" aria-hidden />}
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            {can(user, "collections", "create") && <DropdownMenuItem render={<Link href="/collections/new" onClick={onNavigate} />}><Plus className="size-4" />{t("collections.new")}</DropdownMenuItem>}
            <DropdownMenuItem render={<Link href="/collections" onClick={onNavigate} />}><List className="size-4" />{t("nav.all_collections")}</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        {current && (
          <nav aria-label={current.name}>
            {sections.map((s) => {
              const href = base + s.seg;
              // Records is the collection's root; its record pages count as Records too.
              const active = s.seg ? pathname === href : pathname === base || pathname.startsWith("/records/");
              const name = t(s.key);
              return (
                <span key={s.seg} className="contents">
                  {tooltip(name, (
                    <Link href={href} aria-label={name} aria-current={active ? "page" : undefined} className="rail-item" onClick={onNavigate}>
                      <s.icon className="size-4" aria-hidden />
                      {!compact && <span>{name}</span>}
                    </Link>
                  ))}
                </span>
              );
            })}
          </nav>
        )}
      </div>
    </section>
  );
}
