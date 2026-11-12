"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { Lock, UserRoundPlus, type LucideIcon } from "lucide-react";
import { PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { Card } from "@/components/ui/card";
import { audit, products, roles, users } from "@/lib/api/endpoints";
import { targetHref } from "@/lib/audit";
import type { UserCounts } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { ADMIN_NAV, MAIN_NAV } from "@/lib/navigation";
import { can } from "@/lib/permissions";

// eslint-disable-next-line @typescript-eslint/no-explicit-any -- each total picks its number from its own answer
type Total = { key: readonly unknown[]; fn: () => Promise<any>; pick: (data: any) => number };

const listTotal = (data: { pagination: { total_items: number } }) => data.pagination.total_items;
// The three user cards share one query: one count pass instead of three.
const userCounts = (pick: (c: UserCounts) => number): Total => ({ key: ["users", "counts"], fn: users.counts, pick });

// One row of each list is enough: the total comes back in the pagination.
const totals: Record<string, Total> = {
  "/products": { key: ["products", "total"], fn: () => products.list({ page: 1, limit: 1 }), pick: listTotal },
  "/admin/users": userCounts((c) => c.total),
  "/admin/roles": { key: ["roles", "list", "total"], fn: () => roles.list({ page: 1, limit: 1 }), pick: listTotal },
};

function StatCard({ href, label, icon: Icon, total }: { href: string; label: string; icon: LucideIcon; total: Total }) {
  const count = useQuery({ queryKey: total.key, queryFn: total.fn, select: total.pick });
  return (
    <Link href={href} className="group rounded-lg focus-visible:outline-2 focus-visible:outline-ring">
      <Card className="h-full gap-2 p-4 transition-colors group-hover:bg-muted">
        <div className="flex items-center justify-between gap-2">
          <span className="text-sm text-muted-foreground">{label}</span>
          <Icon className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        </div>
        <span className="text-2xl font-semibold tabular-nums">{count.data ?? "—"}</span>
      </Card>
    </Link>
  );
}

/** The latest audit records, so a returning admin sees what changed. */
function RecentActivity() {
  const { t } = useI18n();
  const format = useFormatDate();
  const recent = useQuery({ queryKey: ["audit", "recent"], queryFn: ({ signal }) => audit.list({ page: 1, limit: 5 }, signal) });
  const label = (a: string) => {
    const text = t(`audit.action.${a}` as MessageKey);
    return text.startsWith("audit.action.") ? a : text;
  };
  return (
    <Card className="gap-3 p-4">
      <div className="flex items-center justify-between gap-2">
        <h2 className="text-sm font-medium">{t("home.recent")}</h2>
        <Link href="/admin/audit" className="text-sm text-muted-foreground hover:underline">{t("home.see_all")}</Link>
      </div>
      {recent.data?.items.length === 0 && <p className="text-sm text-muted-foreground">{t("home.no_activity")}</p>}
      <ul className="divide-y">
        {recent.data?.items.map((r) => (
          <li key={r.id} className="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-0.5 py-2 text-sm">
            <span className="min-w-0 truncate">
              <span className="font-medium">{label(r.action)}</span>
              {r.target_label && <> · {targetHref(r) ? <Link href={targetHref(r)!} className="hover:underline">{r.target_label}</Link> : r.target_label}</>}{" "}
              <span className="text-muted-foreground">{t("home.by")} {r.actor_name || r.actor_email || t("audit.system")}</span>
            </span>
            <span className="text-xs text-muted-foreground tabular-nums">{format(r.created_at)}</span>
          </li>
        ))}
      </ul>
    </Card>
  );
}

export default function HomePage() {
  const { t } = useI18n();
  const { user } = useMe();
  const items = [...MAIN_NAV, ...ADMIN_NAV].filter((item) => totals[item.href] && (!item.needs || can(user, ...item.needs)));
  const pending = userCounts((c) => c.pending);
  const locked = userCounts((c) => c.locked);
  return (
    <>
      <PageHeader title={user ? `${t("home.greeting")}, ${user.name.split(" ")[0]}` : t("nav.home")} description={t("home.subtitle")} />
      <PageBody>
        <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-3">
          {items.map((item) => <StatCard key={item.href} href={item.href} label={t(item.key)} icon={item.icon} total={totals[item.href]} />)}
          {can(user, "users", "read") && <StatCard href="/admin/users?status=pending" label={t("home.pending")} icon={UserRoundPlus} total={pending} />}
          {can(user, "users", "read") && <StatCard href="/admin/users?status=locked" label={t("home.locked")} icon={Lock} total={locked} />}
        </div>
        {can(user, "audit", "read") && <div className="mt-6"><RecentActivity /></div>}
      </PageBody>
    </>
  );
}
