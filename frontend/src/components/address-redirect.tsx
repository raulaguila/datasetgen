"use client";

import Link from "next/link";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { PageBody } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n/provider";

/**
 * An old link that names only an id: look its address up and move there, keeping
 * the query and hash. A link to something gone or hidden shows "not found".
 */
export function AddressRedirect({ queryKey, find }: { queryKey: unknown[]; find: () => Promise<string> }) {
  const { t } = useI18n();
  const router = useRouter();
  const target = useQuery({ queryKey: ["address", ...queryKey], queryFn: find, retry: false });
  useEffect(() => {
    if (target.data) router.replace(target.data + window.location.search + window.location.hash);
  }, [target.data, router]);
  return (
    <PageBody>
      {target.isError ? (
        <EmptyState title={t("error.not_found")} description={t("error.collection_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/collections" />}>{t("common.back_to_list")}</Button>} />
      ) : (
        <ListSkeleton rows={3} />
      )}
    </PageBody>
  );
}
