"use client";

import { useId, useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { ChevronDown, ChevronRight, ShieldCheck } from "lucide-react";
import { BrandMark } from "@/components/brand-mark";
import { BrandWordmark } from "@/components/brand-wordmark";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { ADMIN_NAV, MAIN_NAV, isActive, type NavItem } from "@/lib/navigation";
import { can } from "@/lib/permissions";

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
  const visible = (items: NavItem[]) => items.filter((item) => !item.needs || can(user, ...item.needs));
  const admin = visible(ADMIN_NAV);

  const tooltip = (label: string, child: React.ReactElement) =>
    compact ? <Tooltip><TooltipTrigger render={child} /><TooltipContent side="right">{label}</TooltipContent></Tooltip> : child;

  const link = (item: NavItem, className: string, iconClass: string) => {
    const label = t(item.key);
    const current = isActive(pathname, item.href);
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
