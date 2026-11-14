"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Bell, ChevronRight, Menu } from "lucide-react";
import { AppRail, useWideRail } from "@/components/app-rail";
import { initials } from "@/components/bits";
import { CommandPalette } from "@/components/command-palette";
import { KeyboardShortcutsDialog } from "@/components/keyboard-shortcuts-dialog";
import { LanguageSwitcher } from "@/components/language-switcher";
import { QueryErrorState } from "@/components/query-error-state";
import { SessionKeepalive } from "@/components/session-keepalive";
import { ThemeToggle } from "@/components/theme-toggle";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { redirectToLogin } from "@/lib/api/client";
import { account, notifications } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { can } from "@/lib/permissions";
import { useI18n } from "@/lib/i18n/provider";
import { ADMIN_NAV, MAIN_NAV, isActive } from "@/lib/navigation";
import { useCrumbStore } from "@/lib/store/crumb";
import { useUIStore } from "@/lib/store/ui";

export const UNREAD_KEY = ["notifications", "unread"] as const;

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { t } = useI18n();
  const { user, failed, retry } = useMe();
  const collapsed = useUIStore((s) => s.sidebarCollapsed);
  const toggleSidebar = useUIStore((s) => s.toggleSidebar);
  const wide = useWideRail();
  const [mobileOpen, setMobileOpen] = useState(false);
  const unread = useQuery({ queryKey: UNREAD_KEY, queryFn: notifications.unread, refetchInterval: 60_000 });

  // layout.tsx marks <html> before paint; the store takes over from here.
  useEffect(() => document.documentElement.classList.remove("sidebar-collapsed"), []);

  useEffect(() => {
    document.getElementById("main-content")?.scrollTo({ top: 0 });
    // eslint-disable-next-line react-hooks/set-state-in-effect -- close the drawer after navigation
    setMobileOpen(false);
  }, [pathname]);

  const section = [...MAIN_NAV, ...ADMIN_NAV].find((item) => isActive(pathname, item.href));
  const crumb = useCrumbStore((s) => s.label);
  const parent = useCrumbStore((s) => s.parent);
  const count = unread.data?.unread ?? 0;
  const admin = ADMIN_NAV.filter((item) => !item.needs || can(user, ...item.needs));

  return (
    <div className="app-frame">
      <div className="app-shell flex h-dvh w-full overflow-hidden bg-content">
        <a
          href="#main-content"
          className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[100] focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-foreground focus:shadow-lg focus:outline-none focus:ring-2 focus:ring-ring"
        >
          {t("a11y.skip_to_content")}
        </a>

        <AppRail collapsed={collapsed || !wide} onToggle={toggleSidebar} />

        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetContent
            data-mobile-drawer
            side="left"
            showCloseButton
            className="flex flex-col gap-0 p-0 data-[side=left]:w-[min(18rem,calc(100vw-2rem))] data-[side=left]:sm:max-w-72 md:hidden"
          >
            <SheetHeader className="sr-only">
              <SheetTitle>{t("a11y.nav_menu")}</SheetTitle>
              <SheetDescription>{t("a11y.open_menu")}</SheetDescription>
            </SheetHeader>
            <AppRail collapsed={false} onToggle={toggleSidebar} mobile onNavigate={() => setMobileOpen(false)} />
          </SheetContent>
        </Sheet>

        <div className="flex min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-20 flex h-[var(--header-h)] shrink-0 items-center border-b bg-background px-[var(--space-page)]">
            <div className="flex w-full items-center justify-between">
            <div className="flex min-w-0 flex-1 items-center gap-2 text-sm">
              <button
                type="button"
                onClick={() => setMobileOpen(true)}
                aria-label={t("a11y.open_menu")}
                aria-expanded={mobileOpen}
                className="flex size-11 shrink-0 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground md:hidden"
              >
                <Menu className="size-5" />
              </button>
              {/* Top-level pages already name themselves in the h1; the trail starts one level down. */}
              {crumb && parent ? (
                <nav aria-label="breadcrumb" className="flex min-w-0 items-center gap-1.5">
                  <Link href={parent.href} className="min-w-0 truncate font-medium hover:underline">{parent.label}</Link>
                  {/* On a phone the h1 names the page; the trail keeps only its parent. */}
                  <ChevronRight className="hidden size-3.5 shrink-0 text-muted-foreground sm:block" />
                  <span aria-current="page" className="hidden truncate text-muted-foreground sm:inline">{crumb}</span>
                </nav>
              ) : !crumb && section && ADMIN_NAV.includes(section) ? (
                <span className="truncate text-muted-foreground">{t("nav.admin_area")}</span>
              ) : crumb && section && (
                <nav aria-label="breadcrumb" className="flex min-w-0 items-center gap-1.5">
                  <Link href={section.href} className="shrink-0 text-muted-foreground hover:text-foreground">{t(section.key)}</Link>
                  <ChevronRight className="hidden size-3.5 shrink-0 text-muted-foreground sm:block" />
                  <span aria-current="page" className="hidden truncate font-medium sm:inline">{crumb}</span>
                </nav>
              )}
            </div>
            <div className="ml-2 flex shrink-0 items-center gap-0.5 sm:gap-1">
              <CommandPalette />
              <KeyboardShortcutsDialog />
              <Link
                href="/notifications"
                aria-label={count ? `${t("nav.notifications")}: ${count}` : t("nav.notifications")}
                className="relative flex size-11 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground md:size-8"
              >
                <Bell className="size-4" />
                {count > 0 && (
                  <span className="absolute -right-0.5 -top-0.5 min-w-4 rounded-full bg-primary px-1 text-center text-[10px] font-semibold leading-4 text-primary-foreground">
                    {count > 99 ? "99+" : count}
                  </span>
                )}
              </Link>
              <ThemeToggle />
              <LanguageSwitcher />
              <DropdownMenu>
                <DropdownMenuTrigger aria-label={t("a11y.account_menu")} className="ml-1 flex size-11 items-center justify-center rounded-md md:size-8">
                  <Avatar className="size-8">
                    {user?.avatar_at && <AvatarImage src={account.avatarUrl(user.avatar_at)} alt="" />}
                    <AvatarFallback className="text-xs">{initials(user?.name ?? "?")}</AvatarFallback>
                  </Avatar>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-auto min-w-56 max-w-80">
                  <DropdownMenuGroup>
                    <DropdownMenuLabel>
                      <div className="truncate">{user?.name}</div>
                      <div className="truncate text-xs font-normal text-muted-foreground">{user?.email}</div>
                    </DropdownMenuLabel>
                  </DropdownMenuGroup>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem render={<Link href="/profile" />}>{t("nav.profile")}</DropdownMenuItem>
                  {admin.length > 0 && (
                    <>
                      <DropdownMenuSeparator />
                      <DropdownMenuGroup>
                        <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">{t("nav.admin_area")}</DropdownMenuLabel>
                        {admin.map((item) => (
                          <DropdownMenuItem key={item.href} render={<Link href={item.href} />}>
                            <item.icon className="size-4" aria-hidden />
                            {t(item.key)}
                          </DropdownMenuItem>
                        ))}
                      </DropdownMenuGroup>
                      <DropdownMenuSeparator />
                    </>
                  )}
                  <DropdownMenuItem onClick={() => void redirectToLogin()}>{t("common.logout")}</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
            </div>
          </header>

          <main id="main-content" tabIndex={-1} className="min-h-0 flex-1 overflow-auto bg-content outline-none">
            <div className="app-page">
              {failed ? <div className="p-[var(--space-page)]"><QueryErrorState onRetry={retry} /></div> : children}
            </div>
          </main>
        </div>
      </div>
      <SessionKeepalive />
    </div>
  );
}
