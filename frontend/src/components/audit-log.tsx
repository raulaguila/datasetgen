"use client";

import { Fragment, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import { CalendarDays, Download, ScrollText, X } from "lucide-react";
import { useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { TablePagination, emptyPageFallback } from "@/components/table-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { audit, collections, users } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { can, permissionsText } from "@/lib/permissions";
import type { AuditRecord } from "@/lib/api/types";
import { targetHref } from "@/lib/audit";
import { errorMessage } from "@/lib/errors";
import { useGoToPage, useListParams } from "@/lib/hooks/use-list-params";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";

const KINDS = ["user.", "role.", "collection.", "record.", "admin."];
const COLLECTION_KINDS = ["collection.", "record."];
const DAY = /^\d{4}-\d{2}-\d{2}$/;

/** Start of a local calendar day as an RFC 3339 time; plusDays moves it. */
function dayStart(day: string, plusDays = 0): string | undefined {
  if (!DAY.test(day)) return undefined;
  const d = new Date(`${day}T00:00:00`);
  d.setDate(d.getDate() + plusDays);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

/** A local calendar day as YYYY-MM-DD, n days before today. */
function daysAgo(n: number): string {
  const d = new Date();
  d.setDate(d.getDate() - n);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}
const PRESETS = [["audit.today", 0], ["audit.last_7", 6], ["audit.last_30", 29]] as const;

/** Adding reads green, removing red, any other change blue. */
const GOOD = /[._](create|import|invite|reinvite|restore|untrash|enable|member_add|bootstrap|promote)$/;
const BAD = /[._](delete|delete_invalid|purge|disable|member_remove|member_leave|sessions_revoke)$/;
function tone(action: string): string {
  return GOOD.test(action) ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
    : BAD.test(action) ? "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300"
    : "border-sky-500/30 bg-sky-500/10 text-sky-700 dark:text-sky-300";
}

/**
 * Detail keys and values in the app language; an unknown key or shape stays as the API sent it.
 * The collection and the target's name are left out: the page and the Target column say them.
 */
function useDetails() {
  const { t } = useI18n();
  return (d: Record<string, unknown>, target?: string): [string, string][] =>
    Object.entries(d).flatMap(([k, v]): [string, string][] => {
      if (k === "collection_id" || (k === "name" && v === target)) return [];
      // The email names the member; its id adds nothing to read.
      if (k === "user_id" && typeof d.email === "string") return [];
      // A change of fields names them: added, removed, changed.
      if (k === "fields" && v && typeof v === "object") {
        return (["added", "removed", "changed"] as const).flatMap((kind): [string, string][] => {
          const names = (v as Record<string, unknown>)[kind];
          return Array.isArray(names) && names.length ? [[t(`audit.fields_${kind}`), names.join(", ")]] : [];
        });
      }
      if (k === "schema") return [[t("audit.fields_changed"), t("common.yes")]];
      const name = t(`audit.detail.${k}` as MessageKey);
      // Codes with a label of their own: a member role, a share link kind, a bulk fix.
      const coded = typeof v === "string" && ({ role: `members.role.${v}`, kind: `share.${v}`, op: `fix.op.${v}` } as Record<string, string>)[k];
      const label = coded ? t(coded as MessageKey) : "";
      const value =
        coded && label !== coded ? label
        : typeof v === "boolean" ? t(v ? "common.yes" : "common.no")
        : typeof v === "string" ? v
        : k === "permissions" && v && typeof v === "object" ? permissionsText(t, v as Record<string, number>)
        : JSON.stringify(v);
      return [[name.startsWith("audit.detail.") ? k : name, value]];
    });
}

/**
 * A date shown in the app language. The native input writes dates in the
 * browser language, so it stays invisible and only lends its calendar.
 * shortcut: the calendar popup itself is in the browser language, swap for a date picker component if that matters.
 */
function DateField({ label, value, min, max, onChange }: { label: string; value: string; min?: string; max?: string; onChange: (day: string) => void }) {
  const { t, locale } = useI18n();
  const input = useRef<HTMLInputElement>(null);
  const shown = DAY.test(value) ? new Date(`${value}T00:00:00`).toLocaleDateString(locale, { dateStyle: "medium" }) : "";
  return (
    <span className="relative inline-flex min-w-0 items-center gap-2 text-sm text-muted-foreground max-sm:col-span-2">
      {label}
      <Button
        type="button"
        variant="outline"
        className="w-40 justify-between font-normal max-sm:w-auto max-sm:min-w-0 max-sm:flex-1"
        aria-label={`${label}: ${shown || t("common.pick_date")}`}
        onClick={() => (input.current?.showPicker ? input.current.showPicker() : input.current?.focus())}
      >
        <span className={cn("truncate", shown && "text-foreground")}>{shown || t("common.pick_date")}</span>
        <CalendarDays className="size-4" aria-hidden="true" />
      </Button>
      <input ref={input} type="date" tabIndex={-1} aria-hidden="true" className="pointer-events-none absolute right-0 bottom-0 size-px opacity-0" value={value} min={min} max={max} onChange={(e) => onChange(e.target.value)} />
      {value && (
        <button type="button" aria-label={`${t("common.clear")}: ${label}`} className="rounded-sm hover:text-foreground" onClick={() => onChange("")}>
          <X className="size-4" />
        </button>
      )}
    </span>
  );
}

/** Picks a user by email from the matches the API returns; the list is native, so it needs no popup code. */
function UserFilter({ onPick }: { onPick: (id: string) => void }) {
  const { t } = useI18n();
  const [text, setText] = useState("");
  const q = useDebounced(text.trim());
  const found = useQuery({
    queryKey: ["users", "pick", q],
    queryFn: ({ signal }) => users.list({ page: 1, limit: 10, search: q }, signal),
    enabled: q.length >= 2,
  });
  // A full email picks the user, whether typed or chosen from the list, once its match arrives.
  const hit = found.data?.items.find((u) => u.email.toLowerCase() === text.trim().toLowerCase());
  useEffect(() => { if (hit) onPick(hit.id); }, [hit, onPick]);
  const listId = "audit-users";
  return (
    <>
      <Input
        type="search"
        className="w-56 max-sm:col-span-2 max-sm:w-full"
        list={listId}
        placeholder={t("audit.user_filter")}
        aria-label={t("audit.user_filter")}
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
      <datalist id={listId}>
        {found.data?.items.map((u) => <option key={u.id} value={u.email}>{u.name}</option>)}
      </datalist>
    </>
  );
}

/** Members picked by email, for a collection's log: anyone outside it never shows there. */
function MemberFilter({ collectionId, onPick }: { collectionId: string; onPick: (id: string) => void }) {
  const { t } = useI18n();
  const members = useQuery({ queryKey: ["collections", "members", collectionId], queryFn: () => collections.members(collectionId) });
  const items = Object.fromEntries((members.data?.items ?? []).map((m) => [m.user_id, m.name]));
  return (
    <Select value="" items={items} onValueChange={(v) => v && onPick(v as string)}>
      <SelectTrigger className="w-56 max-sm:col-span-2 max-sm:w-full" aria-label={t("audit.user_filter")}><SelectValue placeholder={t("audit.user_filter")} /></SelectTrigger>
      <SelectContent>
        {(members.data?.items ?? []).map((m) => <SelectItem key={m.user_id} value={m.user_id}>{m.name} · {m.email}</SelectItem>)}
      </SelectContent>
    </Select>
  );
}

/**
 * The audit log with its filters, table and detail drawer: the whole platform's
 * (export included) or, with collectionId, one collection's own entries.
 */
export function AuditLog({ collectionId }: { collectionId?: string }) {
  const { t, locale } = useI18n();
  const { user: me } = useMe();
  const pathname = usePathname();
  const format = useFormatDate();
  const kinds = collectionId ? COLLECTION_KINDS : KINDS;
  const [params, set] = useListParams({ page: 1, limit: 25, action: "", user: "", from: "", to: "" });
  const { page, limit, user, from, to } = params;
  const action = kinds.includes(params.action) ? params.action : "";
  const filters = { action, user_id: user, from: dayStart(from), to: dayStart(to, 1) };
  const list = useQuery({
    queryKey: ["audit", collectionId ?? "all", page, limit, action, user, from, to],
    queryFn: ({ signal }) => (collectionId ? collections.audit(collectionId, { page, limit, ...filters }, signal) : audit.list({ page, limit, ...filters }, signal)),
    placeholderData: keepPreviousData,
  });
  const pickable = !!collectionId || can(me, "users", "read");
  const picked = useQuery({ queryKey: ["users", "detail", user], queryFn: () => users.get(user), enabled: !!user && !collectionId && pickable });
  const members = useQuery({ queryKey: ["collections", "members", collectionId], queryFn: () => collections.members(collectionId!), enabled: !!user && !!collectionId });
  const pickedEmail = collectionId ? members.data?.items.find((m) => m.user_id === user)?.email : picked.data?.email;
  const download = useMutation({
    mutationFn: () => audit.export(filters),
    onError: (err) => toast.error(errorMessage(err, "audit.export_failed", t)),
  });
  const fallback = list.isPlaceholderData ? null : emptyPageFallback(page, limit, list.data);
  useGoToPage(fallback, set); // the last rows of the last page went away
  const kindItems: Record<string, string> = { all: t("audit.all_actions"), ...Object.fromEntries(kinds.map((k) => [k, t(`audit.kind.${k.slice(0, -1)}` as MessageKey)])) };
  const details = useDetails();
  const [open, setOpen] = useState<AuditRecord | null>(null);
  // Rows come grouped by day: Today, Yesterday, then the date.
  const dayOf = (iso: string) => new Date(iso).toDateString();
  const dayLabel = (iso: string) => {
    const d = new Date(iso);
    const ago = Math.round((new Date(new Date().toDateString()).getTime() - new Date(d.toDateString()).getTime()) / 86_400_000);
    return ago === 0 ? t("audit.today") : ago === 1 ? t("audit.yesterday") : d.toLocaleDateString(locale, { weekday: "long", day: "numeric", month: "long", year: d.getFullYear() === new Date().getFullYear() ? undefined : "numeric" });
  };
  const time = (iso: string) => new Date(iso).toLocaleTimeString(locale, { hour: "2-digit", minute: "2-digit" });
  const label = (a: string) => {
    const text = t(`audit.action.${a}` as MessageKey);
    return text.startsWith("audit.action.") ? a : text;
  };

  return (
    <>
      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-2 max-sm:grid max-sm:grid-cols-2">
          <Select value={action || "all"} items={kindItems} onValueChange={(v) => set({ action: v === "all" ? "" : (v as string), page: 1 })}>
            <SelectTrigger className="w-44 max-sm:col-span-2 max-sm:w-full" aria-label={t("audit.action")}><SelectValue /></SelectTrigger>
            <SelectContent>
              {Object.entries(kindItems).map(([value, text]) => <SelectItem key={value} value={value}>{text}</SelectItem>)}
            </SelectContent>
          </Select>
          <div role="group" aria-label={t("audit.period")} className="flex gap-1 max-sm:col-span-2">
            {PRESETS.map(([key, n]) => {
              const on = from === daysAgo(n) && !to;
              return <Button key={key} type="button" size="sm" variant={on ? "secondary" : "ghost"} aria-pressed={on} onClick={() => set({ from: on ? "" : daysAgo(n), to: "", page: 1 })}>{t(key)}</Button>;
            })}
          </div>
          <DateField label={t("audit.from")} value={from} max={to || undefined} onChange={(day) => set({ from: day, page: 1 })} />
          <DateField label={t("audit.to")} value={to} min={from || undefined} onChange={(day) => set({ to: day, page: 1 })} />
          {!user && pickable && (collectionId ? <MemberFilter collectionId={collectionId} onPick={(id) => set({ user: id, page: 1 })} /> : <UserFilter onPick={(id) => set({ user: id, page: 1 })} />)}
          {user && (
            <Badge variant="secondary" className="gap-1 max-sm:col-span-2 max-sm:justify-self-start">
              {pickedEmail ?? t("audit.filtered_user")}
              <button type="button" aria-label={t("audit.clear_user")} className="-my-1 -mr-1.5 grid size-6 place-items-center rounded-sm hover:text-foreground" onClick={() => set({ user: "", page: 1 })}>
                <X className="size-3" />
              </button>
            </Badge>
          )}
          {!collectionId && (
            <Button variant="outline" className="ml-auto max-sm:col-span-2" disabled={download.isPending || !list.data?.items.length} onClick={() => download.mutate()}>
              <Download className="size-4" />{t("audit.export")}
            </Button>
          )}
        </div>

        {list.isPending ? (
          <ListSkeleton />
        ) : list.isError ? (
          <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : list.data.items.length === 0 ? (
          <EmptyState
            icon={ScrollText}
            title={t(action || user || from || to ? "common.no_results" : "audit.empty")}
            action={(action || user || from || to) && <Button variant="outline" onClick={() => set({ action: "", user: "", from: "", to: "", page: 1 })}>{t("common.clear_filters")}</Button>}
          />
        ) : (
          <Card className="gap-0 overflow-hidden p-0">
            <Table data-responsive data-compact>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-44">{t("audit.when")}</TableHead>
                  <TableHead>{t("audit.actor")}</TableHead>
                  <TableHead>{t("audit.action")}</TableHead>
                  <TableHead>{t("audit.target")}</TableHead>
                  <TableHead>{t("audit.details")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.data.items.map((r, i, all) => {
                  const href = targetHref(r);
                  const target = r.target_label || r.target_id?.slice(0, 8) || "-";
                  const extra = details(r.details, r.target_label).map(([k, v]) => `${k}: ${v}`).join(" · ");
                  return (
                    <Fragment key={r.id}>
                    {(i === 0 || dayOf(all[i - 1].created_at) !== dayOf(r.created_at)) && (
                      <TableRow className="hover:bg-transparent" data-day>
                        <TableCell colSpan={5} className="bg-muted/50 py-1.5 text-xs font-medium text-muted-foreground first-letter:uppercase">{dayLabel(r.created_at)}</TableCell>
                      </TableRow>
                    )}
                    <TableRow className="cursor-pointer" onClick={() => setOpen(r)}>
                      <TableCell data-label={t("audit.when")} data-cell="date" className="text-muted-foreground">
                        {/* the keyboard way to the full record; a click anywhere on the row does the same */}
                        <button type="button" className="text-left tabular-nums hover:underline" title={format(r.created_at)} aria-label={`${t("audit.open")}: ${format(r.created_at)}`}>{time(r.created_at)}</button>
                      </TableCell>
                      <TableCell data-label={t("audit.actor")} data-cell="name" onClick={(e) => e.stopPropagation()}>
                        {r.actor_id ? (
                          <Link href={`${pathname}?user=${r.actor_id}`} className="hover:underline" title={r.actor_email}>{r.actor_name || r.actor_email || r.actor_id.slice(0, 8)}</Link>
                        ) : (
                          <span className="text-muted-foreground">{t("audit.system")}</span>
                        )}
                      </TableCell>
                      <TableCell data-label={t("audit.action")} data-cell="status"><Badge variant="outline" className={cn("font-normal", tone(r.action))}>{label(r.action)}</Badge></TableCell>
                      <TableCell data-label={t("audit.target")} data-cell="sub">
                        <span className={cn("block truncate lg:max-w-56 2xl:max-w-80", r.target_gone && "text-muted-foreground line-through")} title={r.target_gone ? `${target} · ${t("audit.target_gone")}` : target}>
                          {href ? <Link href={href} className="hover:underline" onClick={(e) => e.stopPropagation()}>{target}</Link> : target}
                        </span>
                      </TableCell>
                      <TableCell data-label={t("audit.details")} data-cell="detail" className="text-muted-foreground"><span className="block truncate lg:max-w-56 2xl:max-w-80" title={extra || undefined}>{extra || "-"}</span></TableCell>
                    </TableRow>
                    </Fragment>
                  );
                })}
              </TableBody>
            </Table>
            {/* One page of the smallest size needs no paging controls, only the total. */}
            {page === 1 && list.data.pagination.total_items <= Math.min(limit, 10) ? (
              <p className="border-t px-4 py-2.5 text-xs text-muted-foreground">{t(list.data.pagination.total_items === 1 ? "audit.count_one" : "audit.count").replace("{n}", String(list.data.pagination.total_items))}</p>
            ) : (
              <TablePagination
                page={page}
                pageSize={limit}
                totalItems={list.data.pagination.total_items}
                onPageChange={(n) => set({ page: n })}
                onPageSizeChange={(n) => set({ limit: n, page: 1 })}
              />
            )}
          </Card>
        )}
      </div>
      {/* A side drawer: long details scroll inside it and the list stays in view. */}
      <Sheet open={open !== null} onOpenChange={(o) => !o && setOpen(null)}>
        <SheetContent side="right" className="gap-0 data-[side=right]:w-full data-[side=right]:sm:max-w-xl">
          <SheetHeader className="border-b pr-12"><SheetTitle>{t("audit.record")}</SheetTitle></SheetHeader>
          {open && (
            <dl className="grid min-h-0 flex-1 auto-rows-min grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-3 overflow-y-auto p-4 text-sm">
              {([
                [t("audit.when"), format(open.created_at)],
                [t("audit.actor"), open.actor_id ? [open.actor_name, open.actor_email].filter(Boolean).join(" · ") || open.actor_id : t("audit.system")],
                [t("audit.action"), label(open.action)],
                [t("audit.target"), [open.target_label || open.target_id || "-", open.target_gone && t("audit.target_gone")].filter(Boolean).join(" · ")],
                ...details(open.details, open.target_label),
                ...(open.request_id ? [[t("audit.request"), open.request_id]] : []),
              ] as [string, string][]).map(([k, v]) => (
                <div key={k} className="contents">
                  <dt className="text-muted-foreground">{k}</dt>
                  <dd className="break-words">{v}</dd>
                </div>
              ))}
            </dl>
          )}
        </SheetContent>
      </Sheet>
    </>
  );
}
