"use client";

import { useEffect } from "react";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { APP_NAME } from "@/lib/brand";
import { useI18n } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";
import type { UserStatus } from "@/lib/api/types";

/**
 * Names the browser tab after the screen. An effect, not a <title> tag: Next keeps
 * the last screens mounted but hidden, and their hoisted tags would pile up.
 */
export function useDocumentTitle(title?: string) {
  useEffect(() => {
    if (title) document.title = `${title} - ${APP_NAME}`;
  }, [title]);
}

export function PageHeader({
  title,
  description,
  action,
}: {
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  useDocumentTitle(title);
  return (
    <div data-ui-page-header className="border-b px-[var(--space-page)] py-[var(--space-page-y)] sm:py-4">
      <div className="flex w-full flex-col gap-[var(--space-row)] sm:flex-row sm:flex-wrap sm:items-start sm:justify-between">
      <div className="min-w-0 max-w-full flex-1 sm:basis-64">
        <h1 className="font-heading text-xl font-semibold tracking-tight sm:text-2xl">{title}</h1>
        {description && <p className="mt-1 max-w-[72ch] text-sm text-muted-foreground">{description}</p>}
      </div>
      {action && <div className="flex max-w-full flex-wrap items-center gap-2 [&>*]:max-w-full [&>*]:flex-wrap">{action}</div>}
      </div>
    </div>
  );
}

/** Page body padding shared by every app screen. */
export function PageBody({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn("w-full space-y-6 px-[var(--space-page)] py-[var(--space-page-y)]", className)}>{children}</div>;
}

const STATUS_CLS: Record<UserStatus, string> = {
  active: "bg-emerald-500/15 text-emerald-800 dark:text-emerald-300",
  pending: "bg-amber-500/15 text-amber-800 dark:text-amber-300",
  locked: "bg-red-500/15 text-red-800 dark:text-red-300",
  disabled: "bg-zinc-500/15 text-zinc-700 dark:text-zinc-300",
};

export function StatusBadge({ status }: { status: UserStatus }) {
  const { t } = useI18n();
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium", STATUS_CLS[status])}>
      <span className="size-1.5 shrink-0 rounded-full bg-current" />
      {t(`status.${status}` as MessageKey)}
    </span>
  );
}

/** Green when on, grey when off; products and roles share it. */
export function ActiveBadge({ enabled }: { enabled: boolean }) {
  const { t } = useI18n();
  return (
    <span className={cn("inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium", enabled ? STATUS_CLS.active : STATUS_CLS.disabled)}>
      <span className="size-1.5 shrink-0 rounded-full bg-current" />
      {t(enabled ? "roles.enabled" : "roles.disabled")}
    </span>
  );
}

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  return ((parts[0]?.[0] ?? "") + (parts[1]?.[0] ?? parts[0]?.[1] ?? "")).toUpperCase();
}

/** Date and time in the user's language. */
export function useFormatDate() {
  const { locale } = useI18n();
  return (iso: string) => new Date(iso).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" });
}

/** The red box a form shows above its fields when the server refuses it. */
/** Under a field: its rule or hint while fine, in red once the value breaks a rule. */
export function FieldNote({ id, error, hint }: { id: string; error?: string | null; hint?: string }) {
  if (!error && !hint) return null;
  return <p id={id} className={cn("text-xs", error ? "text-destructive" : "text-muted-foreground")}>{error || hint}</p>;
}

export function FormError({ message }: { message: string | null }) {
  if (!message) return null;
  return (
    <div role="alert" className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
      {message}
    </div>
  );
}

/** Icon button with a tooltip naming what it does. */
export function IconAction({ label, target, className, onClick, children }: { label: string; target: string; className?: string; onClick: () => void; children: React.ReactNode }) {
  return (
    <Tooltip>
      <TooltipTrigger render={<Button variant="ghost" size="icon-sm" className={className} aria-label={`${label}: ${target}`} onClick={onClick} />}>{children}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

/**
 * One part of a form or detail page: title and help beside the content on wide
 * screens, above it on narrow ones. Plain fields stay at a readable width;
 * `wide` lets tables and matrices take the room they need.
 */
export function FormSection({ title, description, wide = false, danger = false, children }: {
  title: string;
  description?: string;
  wide?: boolean;
  danger?: boolean;
  children: React.ReactNode;
}) {
  return (
    <section className="grid gap-3 border-t pt-8 first:border-t-0 first:pt-0 lg:grid-cols-[18rem_minmax(0,1fr)] lg:gap-10">
      <div className="space-y-1">
        <h2 className={cn("font-heading text-base font-semibold", danger && "text-destructive")}>{title}</h2>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      <div className={cn("min-w-0", !wide && "max-w-3xl")}>{children}</div>
    </section>
  );
}

/**
 * The page's form surface: one card holding every section, split by thin rules.
 * Full width like the rest of the platform; FormSection keeps the fields readable.
 */
export function FormSections({ danger = false, children }: { danger?: boolean; children: React.ReactNode }) {
  return <div className={cn("space-y-8 rounded-xl border bg-card p-6 text-card-foreground shadow-xs sm:p-8", danger && "border-destructive/40")}>{children}</div>;
}

/**
 * Save control, sticky at the bottom while there is something to save. As the
 * last child of FormSections, `footer` makes it the card's footer; otherwise it
 * sits inside its section. `submit` makes Save the form's submit button, so Enter saves.
 */
export function SaveBar({ label, error, saving, canSave, saveLabel, discardLabel, onSave, onDiscard, submit = false, footer = false }: {
  label: string;
  error?: string | null;
  saving: boolean;
  canSave: boolean;
  saveLabel: string;
  discardLabel: string;
  onSave?: () => void;
  onDiscard: () => void;
  submit?: boolean;
  footer?: boolean;
}) {
  return (
    <div
      role="region"
      aria-label={label}
      className={cn(
        "sticky z-10 flex flex-wrap items-center gap-3",
        // The negative margins undo FormSections' padding so the bar spans the card.
        footer ? "bottom-0 -mx-6 -mb-6 rounded-b-xl border-t bg-card px-6 py-4 sm:-mx-8 sm:-mb-8 sm:px-8" : "bottom-4 rounded-lg border bg-muted px-4 py-3",
      )}
    >
      <span className="flex-1 text-sm font-medium">{label}</span>
      {error && <div className="basis-full"><FormError message={error} /></div>}
      <Button type="button" variant="ghost" disabled={saving} onClick={onDiscard}>{discardLabel}</Button>
      <Button type={submit ? "submit" : "button"} disabled={saving || !canSave} onClick={submit ? undefined : onSave}>
        {saving && <Loader2 className="size-4 animate-spin" />}
        {saveLabel}
      </Button>
    </div>
  );
}

/** The page's destructive action, kept apart at the bottom so it is never the first button in sight. */
export function DangerZone({ description, label, onClick }: { description: string; label: string; onClick: () => void }) {
  const { t } = useI18n();
  return (
    <FormSections danger>
      <FormSection title={t("common.danger_zone")} danger>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="min-w-0 flex-1 text-sm text-muted-foreground">{description}</p>
          <Button variant="destructive" onClick={onClick}>{label}</Button>
        </div>
      </FormSection>
    </FormSections>
  );
}
