"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Bell, KeyRound, Package, Search, User as UserIcon, UserRound, type LucideIcon } from "lucide-react";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { products, roles, users } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useI18n } from "@/lib/i18n/provider";
import { ADMIN_NAV, MAIN_NAV, type NavItem } from "@/lib/navigation";
import { can } from "@/lib/permissions";
import { cn } from "@/lib/utils";

interface Hit {
  href: string;
  label: string;
  hint?: string;
  icon: LucideIcon;
}

const EXTRA: NavItem[] = [
  { href: "/profile", key: "nav.profile", icon: UserRound },
  { href: "/notifications", key: "nav.notifications", icon: Bell },
];

function useHits(key: string, q: string, enabled: boolean, fn: (signal: AbortSignal) => Promise<Hit[]>) {
  return useQuery({ queryKey: ["palette", key, q], queryFn: ({ signal }) => fn(signal), enabled, staleTime: 10_000 });
}

/** Ctrl/Cmd+K: jump to a screen, or to a user, role or product by name. */
export function CommandPalette() {
  const { t } = useI18n();
  const { user } = useMe();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [active, setActive] = useState(0);
  const q = useDebounced(text.trim());
  const search = open && q.length >= 2;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key.toLowerCase() === "k" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const userHits = useHits("users", q, search && can(user, "users", "read"), (signal) =>
    users.list({ page: 1, limit: 5, search: q }, signal).then((r) => r.items.map((u) => ({ href: `/admin/users/${u.id}`, label: u.name, hint: u.email, icon: UserIcon }))));
  const roleHits = useHits("roles", q, search && can(user, "roles", "read"), (signal) =>
    roles.list({ page: 1, limit: 5, search: q }, signal).then((r) => r.items.map((x) => ({ href: `/admin/roles/${x.id}`, label: x.name, hint: t("nav.roles"), icon: KeyRound }))));
  const productHits = useHits("products", q, search && can(user, "products", "read"), (signal) =>
    products.list({ page: 1, limit: 5, search: q }, signal).then((r) => r.items.map((p) => ({ href: `/products/${p.id}`, label: p.name, hint: t("nav.products"), icon: Package }))));

  const needle = text.trim().toLowerCase();
  const pages: Hit[] = [...MAIN_NAV, ...ADMIN_NAV, ...EXTRA]
    .filter((item) => !item.needs || can(user, ...item.needs))
    .map((item) => ({ href: item.href, label: t(item.key), icon: item.icon }))
    .filter((hit) => hit.label.toLowerCase().includes(needle));
  const hits = [...pages, ...(userHits.data ?? []), ...(roleHits.data ?? []), ...(productHits.data ?? [])];
  const current = Math.min(active, hits.length - 1);
  // While the typed text waits for the debounce or a search is in flight, "nothing found" would be a guess.
  const searching = text.trim().length >= 2 && (text.trim() !== q || userHits.isFetching || roleHits.isFetching || productHits.isFetching);

  const changeOpen = (next: boolean) => {
    setOpen(next);
    setText("");
    setActive(0);
  };
  const go = (hit: Hit | undefined) => {
    if (!hit) return;
    changeOpen(false);
    router.push(hit.href);
  };

  return (
    <>
      <button
        type="button"
        onClick={() => changeOpen(true)}
        aria-label={t("palette.open")}
        aria-keyshortcuts="Control+K Meta+K"
        title={`${t("palette.open")} (Ctrl K)`}
        className="flex size-11 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground md:size-8"
      >
        <Search className="size-4" />
      </button>
      <Dialog open={open} onOpenChange={changeOpen}>
        <DialogContent showCloseButton={false} className="top-[20%] translate-y-0 gap-0 p-0 sm:max-w-lg">
          <DialogTitle className="sr-only">{t("palette.open")}</DialogTitle>
          <DialogDescription className="sr-only">{t("palette.placeholder")}</DialogDescription>
          <div className="flex items-center gap-2 border-b px-3">
            <Search className="size-4 shrink-0 text-muted-foreground" />
            <input
              autoFocus
              role="combobox"
              aria-expanded
              aria-controls="palette-list"
              aria-label={t("palette.placeholder")}
              aria-activedescendant={hits.length ? `palette-${current}` : undefined}
              placeholder={t("palette.placeholder")}
              className="h-12 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
              value={text}
              onChange={(e) => { setText(e.target.value); setActive(0); }}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") { e.preventDefault(); setActive((i) => Math.min(i + 1, hits.length - 1)); }
                else if (e.key === "ArrowUp") { e.preventDefault(); setActive((i) => Math.max(i - 1, 0)); }
                else if (e.key === "Enter") { e.preventDefault(); go(hits[current]); }
              }}
            />
          </div>
          <ul id="palette-list" role="listbox" className="max-h-80 overflow-y-auto p-1.5">
            {hits.length === 0 ? (
              <li className="px-3 py-6 text-center text-sm text-muted-foreground" aria-live="polite">{t(searching ? "palette.searching" : "palette.empty")}</li>
            ) : hits.map((hit, i) => (
              <li
                key={hit.href}
                id={`palette-${i}`}
                role="option"
                aria-selected={i === current}
                className={cn("flex cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm", i === current && "bg-accent")}
                onMouseMove={() => setActive(i)}
                onClick={() => go(hit)}
              >
                <hit.icon className="size-4 shrink-0 text-muted-foreground" />
                <span className="truncate">{hit.label}</span>
                {hit.hint && <span className="ml-auto truncate text-xs text-muted-foreground">{hit.hint}</span>}
              </li>
            ))}
          </ul>
        </DialogContent>
      </Dialog>
    </>
  );
}
