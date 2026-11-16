import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

/** Canonical empty list: icon + title + one sentence + optional CTA. compact is
 * the one-line form for a list inside a form section. */
export function EmptyState({
  icon: Icon,
  title,
  description,
  action,
  compact = false,
  className,
}: {
  icon?: LucideIcon;
  title: string;
  description?: string;
  action?: React.ReactNode;
  compact?: boolean;
  className?: string;
}) {
  if (compact) {
    return <p role="status" className={cn("rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground", className)}>{title}</p>;
  }
  return (
    <div
      role="status"
      className={cn(
        "flex flex-col items-center justify-center rounded-xl border border-dashed px-6 py-14 text-center",
        className,
      )}
    >
      {Icon && (
        <div className="mb-4 flex size-12 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <Icon className="size-5" />
        </div>
      )}
      <p className="font-medium">{title}</p>
      {description && (
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">{description}</p>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  );
}
