"use client";

import { AlertTriangle } from "lucide-react";
import { ApiError } from "@/lib/api/client";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n/provider";

/** Distinct from EmptyState: failed fetch, not “no rows yet”. A 403 says so and offers no retry. */
export function QueryErrorState({
  title,
  description,
  error,
  onRetry,
  className,
}: {
  title?: string;
  description?: string;
  error?: unknown;
  onRetry?: () => void;
  className?: string;
}) {
  const { t } = useI18n();
  if (error instanceof ApiError && error.code === "FORBIDDEN") {
    title ??= t("error.forbidden");
    description ??= "";
    onRetry = undefined;
  }
  return (
    <div
      role="alert"
      className={cn(
        "flex flex-col items-center justify-center rounded-xl border border-dashed border-destructive/30 px-6 py-14 text-center",
        className,
      )}
    >
      <div className="mb-4 flex size-12 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
        <AlertTriangle className="size-5" />
      </div>
      <p className="font-medium">{title ?? t("error.title")}</p>
      {description !== "" && (
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          {description ?? t("error.load_desc")}
        </p>
      )}
      {onRetry && (
        <Button className="mt-4" variant="outline" onClick={onRetry}>
          {t("error.retry")}
        </Button>
      )}
    </div>
  );
}
