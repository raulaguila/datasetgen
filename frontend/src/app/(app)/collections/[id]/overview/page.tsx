"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, ChevronRight, Circle, Plus } from "lucide-react";
import { avatarStyle, initials, useRelativeTime } from "@/components/bits";
import { CollectionDashboard } from "@/components/collection-dashboard";
import { recordTitle } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { collections, records } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";
import { useQuality } from "@/lib/hooks/use-quality";
import { roleAllows } from "@/lib/permissions";
import { cn } from "@/lib/utils";
import { CollectionPage } from "../_parts";

const DAYS = 30;

/** The last DAYS local days as YYYY-MM-DD, oldest first: the API leaves out days without records. */
function lastDays(): string[] {
  const out: string[] = [];
  const d = new Date();
  d.setDate(d.getDate() - (DAYS - 1));
  for (let i = 0; i < DAYS; i++) {
    out.push(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`);
    d.setDate(d.getDate() + 1);
  }
  return out;
}

/** One number; with `href` the whole card opens the page behind it. */
function Stat({ label, value, hint, href, title }: { label: string; value: React.ReactNode; hint?: string; href?: string; title?: string }) {
  const body = (
    <>
      <p className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
        {label}
        {href && <ChevronRight className="size-3.5 opacity-60 transition-transform group-hover/stat:translate-x-0.5" aria-hidden />}
      </p>
      <p className="mt-1 truncate font-heading text-2xl font-semibold tabular-nums" title={title}>{value}</p>
      {hint && <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>}
    </>
  );
  return (
    <Card className={cn("gap-0 p-0 transition-colors", href && "hover:border-primary/40 hover:bg-muted/40")}>
      {href ? <Link href={href} className="group/stat block rounded-[inherit] px-4 py-3 outline-none focus-visible:ring-2 focus-visible:ring-ring">{body}</Link> : <div className="px-4 py-3">{body}</div>}
    </Card>
  );
}

/** Starter keys: a collection still on them has not had its fields set up. */
const STARTER_KEYS = ["title", "notes"];

/** What a new collection still needs, each step ticked off as it happens. */
function NextSteps({ collection, records, members }: { collection: Collection; records: number; members: number }) {
  const { t } = useI18n();
  const base = `/collections/${collection.id}`;
  const keys = Object.keys((collection.schema?.properties as Record<string, unknown> | undefined) ?? {});
  const steps = [
    { done: records > 0 || keys.some((k) => !STARTER_KEYS.includes(k)), label: t("overview.step_fields"), href: `${base}/fields` },
    { done: records > 0, label: t("overview.step_records"), href: base },
    ...(roleAllows(collection.role, "owner") ? [{ done: members > 1, label: t("overview.step_members"), href: `${base}/members` }] : []),
  ];
  if (steps.every((s) => s.done)) return null;
  return (
    <Card className="gap-0 overflow-hidden p-0">
      <div className="border-b px-4 py-3"><h2 className="font-heading text-base font-semibold">{t("overview.next_steps")}</h2></div>
      <ol className="divide-y">
        {steps.map((s) => (
          <li key={s.href}>
            <Link href={s.href} className="flex items-center gap-3 px-4 py-2.5 text-sm hover:bg-muted/40">
              {s.done ? <CheckCircle2 className="size-4 shrink-0 text-success" aria-hidden /> : <Circle className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
              <span className={cn("min-w-0 flex-1", s.done ? "text-muted-foreground line-through" : "font-medium")}>{s.label}</span>
              {!s.done && <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
            </Link>
          </li>
        ))}
      </ol>
    </Card>
  );
}

function Overview({ collection }: { collection: Collection }) {
  const { t, locale } = useI18n();
  const relative = useRelativeTime();
  const base = `/collections/${collection.id}`;
  const editor = roleAllows(collection.role, "editor");
  const owner = roleAllows(collection.role, "owner");
  const stats = useQuery({ queryKey: ["collections", "stats", collection.id], queryFn: ({ signal }) => collections.stats(collection.id, signal) });
  const { q: quality, stale } = useQuality(collection, editor);
  const invalid = quality.data?.status === "done" && !stale ? quality.data.invalid_count : null;
  const recent = useQuery({ queryKey: ["records", collection.id, "recent"], queryFn: ({ signal }) => records.list({ collection_id: collection.id, page: 1, limit: 5 }, signal) });
  const number = new Intl.NumberFormat(locale);

  if (stats.isPending) return <ListSkeleton rows={4} />;
  if (stats.isError) return <QueryErrorState error={stats.error} onRetry={() => void stats.refetch()} />;
  const s = stats.data;
  const counts = new Map(s.per_day.map((d) => [d.day, d.count]));
  const days = lastDays().map((day) => ({ day, count: counts.get(day) ?? 0 }));
  const peak = Math.max(1, ...days.map((d) => d.count));
  const created = days.reduce((n, d) => n + d.count, 0);
  const dayLabel = (day: string) => new Date(`${day}T00:00:00`).toLocaleDateString(locale, { day: "numeric", month: "short" });
  // A handful of active days reads better as a sentence than as a row of empty bars.
  const active = days.filter((d) => d.count > 0);
  const steps = editor ? <NextSteps collection={collection} records={s.records} members={s.members} /> : null;
  // A single member makes the ranking say nothing.
  const side = steps ?? (s.members > 1 ? "contributors" : null);

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat label={t("overview.records")} value={number.format(s.records)} href={base} />
        <Stat label={t("overview.members")} value={number.format(s.members)} href={owner ? `${base}/members` : undefined} />
        {editor && (
          <Stat
            label={t("overview.invalid")}
            value={invalid === null ? "—" : number.format(invalid)}
            hint={invalid === 0 ? t("overview.all_valid") : invalid === null ? t("quality.running") : undefined}
            href={`${base}/quality`}
          />
        )}
        <Stat label={t("overview.last_update")} value={s.last_update ? relative(s.last_update).rel : t("overview.never")} title={s.last_update ? relative(s.last_update).abs : undefined} />
      </div>

      <Card className="gap-3 p-4">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="font-heading text-base font-semibold">{t("overview.per_day")}</h2>
          {active.length >= 3 && <span className="text-sm tabular-nums text-muted-foreground">{number.format(created)}</span>}
        </div>
        {created === 0 ? (
          <p className="text-sm text-muted-foreground">{t("overview.per_day_empty")}</p>
        ) : active.length < 3 ? (
          <p className="text-sm text-muted-foreground">
            {t("overview.per_day_few").replace("{n}", number.format(created)).replace("{day}", dayLabel(active[active.length - 1].day))}
          </p>
        ) : (
          <>
            <ol className="flex h-24 items-end gap-[3px]" aria-label={t("overview.per_day")}>
              {days.map((d) => {
                const text = t("overview.day_count").replace("{day}", dayLabel(d.day)).replace("{n}", String(d.count));
                return (
                  <li key={d.day} className="flex h-full min-w-0 flex-1 items-end" title={text} aria-label={text}>
                    <span className="block w-full rounded-t-sm bg-primary/70" style={{ height: d.count ? `${Math.max(4, (d.count / peak) * 100)}%` : "2px", opacity: d.count ? 1 : 0.25 }} />
                  </li>
                );
              })}
            </ol>
            <div className="flex justify-between text-xs text-muted-foreground" aria-hidden>
              <span>{dayLabel(days[0].day)}</span>
              <span>{dayLabel(days[days.length - 1].day)}</span>
            </div>
          </>
        )}
      </Card>

      <CollectionDashboard collection={collection} />

      <div className={cn("grid gap-6", side && "lg:grid-cols-2")}>
        <Card className="gap-0 overflow-hidden p-0">
          <div className="flex items-center justify-between border-b px-4 py-3">
            <h2 className="font-heading text-base font-semibold">{t("overview.recent")}</h2>
            <Link href={base} className="text-sm text-primary hover:underline">{t("overview.view_all")}</Link>
          </div>
          {recent.isPending ? (
            <div className="p-4"><ListSkeleton rows={2} /></div>
          ) : !recent.data?.items.length ? (
            <p className="px-4 py-6 text-sm text-muted-foreground">{t("records.empty")}</p>
          ) : (
            <ul className="divide-y">
              {recent.data.items.map((r) => {
                const text = recordTitle(collection.schema ?? null, r.data) ?? `${t("records.record")} ${r.id.slice(0, 8)}`;
                const when = relative(r.created_at);
                return (
                  <li key={r.id} className="flex items-center gap-3 px-4 py-2.5">
                    <Link href={`/records/${r.id}`} className="min-w-0 flex-1 truncate text-sm font-medium hover:underline">{text}</Link>
                    <time dateTime={r.created_at} title={when.abs} className="shrink-0 text-xs text-muted-foreground">{when.rel}</time>
                  </li>
                );
              })}
            </ul>
          )}
        </Card>

        {steps}
        {side === "contributors" && <Card className="gap-0 overflow-hidden p-0">
          <div className="border-b px-4 py-3"><h2 className="font-heading text-base font-semibold">{t("overview.contributors")}</h2></div>
          {s.contributors.length === 0 ? (
            <p className="px-4 py-6 text-sm text-muted-foreground">{t("overview.contributors_empty")}</p>
          ) : (
            <ul className="divide-y">
              {s.contributors.map((c) => (
                <li key={c.user_id} className="flex items-center gap-3 px-4 py-2.5">
                  <span className="grid size-7 shrink-0 place-items-center rounded-full text-[11px] font-semibold" style={avatarStyle(c.user_id)} aria-hidden>{initials(c.name)}</span>
                  <span className="min-w-0 flex-1 truncate text-sm font-medium">{c.name}</span>
                  <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                    {t("overview.created_edited").replace("{c}", number.format(c.created)).replace("{e}", number.format(c.edited))}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Card>}
      </div>

    </div>
  );
}

/** Your role, and the one action worth a shortcut here: the sidebar already lists the sections. */
function OverviewActions({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  return (
    <>
      {collection.role && <span className="text-xs text-muted-foreground">{t("overview.your_role")}: {t(`members.role.${collection.role}`)}</span>}
      {roleAllows(collection.role, "editor") && (
        <Button nativeButton={false} render={<Link href={`/records/new?collection=${collection.id}`} />}><Plus className="size-4" />{t("records.new")}</Button>
      )}
    </>
  );
}

export default function CollectionOverviewPage() {
  return (
    <CollectionPage section="collections.tab.overview" description={(c) => c.description.trim() || undefined} action={(c) => <OverviewActions collection={c} />}>
      {(collection) => <Overview collection={collection} />}
    </CollectionPage>
  );
}
