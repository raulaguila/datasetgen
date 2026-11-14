"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ShieldAlert } from "lucide-react";
import { PageBody } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { ADMIN_NAV, isActive } from "@/lib/navigation";
import { can } from "@/lib/permissions";

/**
 * An admin page opened by its address without the permission says so at once,
 * instead of drawing its filters over an error. The server still decides.
 */
export default function AdminLayout({ children }: { children: React.ReactNode }) {
  const { t } = useI18n();
  const { user } = useMe();
  const pathname = usePathname();
  const need = ADMIN_NAV.find((n) => isActive(pathname, n.href))?.needs;
  if (user && need && !can(user, ...need)) {
    return (
      <PageBody>
        <EmptyState icon={ShieldAlert} title={t("error.forbidden")} description={t("error.admin_only")}
          action={<Button variant="outline" nativeButton={false} render={<Link href="/collections" />}>{t("nav.collections")}</Button>} />
      </PageBody>
    );
  }
  return children;
}
