"use client";

import Link from "next/link";
import { Compass } from "lucide-react";
import { useDocumentTitle } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n/provider";

/** Any address no route matches. */
export default function NotFound() {
  const { t } = useI18n();
  useDocumentTitle(t("error.page_not_found"));
  return (
    <main className="flex flex-1 items-center justify-center p-6">
      <EmptyState
        icon={Compass}
        title={t("error.page_not_found")}
        description={t("error.page_not_found_desc")}
        className="w-full max-w-md"
        action={<Button nativeButton={false} render={<Link href="/" />}>{t("common.go_home")}</Button>}
      />
    </main>
  );
}
