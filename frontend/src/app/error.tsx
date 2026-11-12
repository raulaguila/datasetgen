"use client";

import Link from "next/link";
import { AlertTriangle } from "lucide-react";
import { EmptyState } from "@/components/empty-state";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n/provider";

/** A screen that crashed while rendering: say so and offer a way out instead of a blank page. */
export default function ErrorPage({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  const { t } = useI18n();
  return (
    <main className="flex flex-1 items-center justify-center p-6">
      <EmptyState
        icon={AlertTriangle}
        title={t("error.title")}
        description={t("error.load_desc")}
        className="w-full max-w-md"
        action={
          <div className="flex flex-wrap justify-center gap-2">
            <Button variant="outline" onClick={reset}>{t("error.retry")}</Button>
            <Button nativeButton={false} render={<Link href="/" />}>{t("common.go_home")}</Button>
          </div>
        }
      />
    </main>
  );
}
