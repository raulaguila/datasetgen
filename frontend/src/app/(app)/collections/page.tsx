"use client";

import { useId, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Database, FileJson, ListTree, LogOut, MoreVertical, Plus, Search, Settings2, Star, Trash2, UserCog, Users, X } from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/bits";
import { CollectionAvatar } from "@/components/collection-avatar";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { collections } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { can, roleAllows } from "@/lib/permissions";
import { useUIStore } from "@/lib/store/ui";

// shortcut: one request of up to 100 collections, filtered and grouped in the browser; page on the server past that.
const LIMIT = 100;

function byActivity(a: Collection, b: Collection): number {
  return (Date.parse(b.last_activity ?? b.updated_at) || 0) - (Date.parse(a.last_activity ?? a.updated_at) || 0) || a.name.localeCompare(b.name);
}

/** "3 h ago" in the locale, with the full date for the tooltip. */
function useRelative() {
  const { locale } = useI18n();
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  return (iso: string) => {
    const secs = (Date.parse(iso) - Date.now()) / 1000;
    const steps: [Intl.RelativeTimeFormatUnit, number][] = [["year", 31536000], ["month", 2592000], ["week", 604800], ["day", 86400], ["hour", 3600], ["minute", 60]];
    const [unit, size] = steps.find(([, s]) => Math.abs(secs) >= s) ?? ["second", 1];
    return { rel: unit === "second" ? rtf.format(0, "minute") : rtf.format(Math.round(secs / size), unit), abs: new Date(iso).toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" }) };
  };
}

interface RowProps {
  c: Collection;
  pinned: boolean;
  current: boolean;
  onPin: () => void;
  onRemove: (c: Collection, kind: "leave" | "delete") => void;
}

export default function CollectionsPage() {
  const { t } = useI18n();
  const { user } = useMe();
  const client = useQueryClient();
  const router = useRouter();
  const [q, setQ] = useState("");
  const list = useQuery({
    queryKey: ["collections", "manage"],
    queryFn: ({ signal }) => collections.list({ page: 1, limit: LIMIT, sort: "name", order: "asc", details: true }, signal),
  });
  const pinned = useUIStore((s) => s.pinned);
  const togglePin = useUIStore((s) => s.togglePin);
  const storedId = useUIStore((s) => s.collectionId);
  const setCollection = useUIStore((s) => s.setCollection);
  const [removing, setRemoving] = useState<{ c: Collection; kind: "leave" | "delete" } | null>(null);

  const all = list.data?.items ?? [];
  const term = q.trim().toLocaleLowerCase();
  const filtered = all.filter((c) => !term || c.name.toLocaleLowerCase().includes(term) || c.description.toLocaleLowerCase().includes(term));
  const isPinned = (c: Collection) => pinned.includes(c.id);
  const groups: [MessageKey, Collection[]][] = [
    ["collections.group_pinned", filtered.filter(isPinned)],
    ["collections.group_mine", filtered.filter((c) => !isPinned(c) && c.member !== false && c.role === "owner")],
    ["collections.group_shared", filtered.filter((c) => !isPinned(c) && c.member !== false && c.role !== "owner")],
    // A platform admin sees every collection without belonging to it.
    ["collections.group_other", filtered.filter((c) => !isPinned(c) && c.member === false)],
  ];
  const total = list.data?.pagination.total_items ?? 0;
  const status = !list.data ? ""
    : term ? t("collections.search_results").replace("{shown}", String(filtered.length)).replace("{total}", String(all.length))
    : t(total === 1 ? "collections.count_one" : "collections.count").replace("{n}", String(total));

  return (
    <div data-ui-collection-management>
      <PageHeader
        title={t("nav.collections")}
        description={t("collections.subtitle")}
        action={can(user, "collections", "create") && <Button onClick={() => router.push("/collections/new")}><Plus className="size-4" />{t("collections.new")}</Button>}
      />
      <div className="collection-management-body">
        <div className="collection-toolbar">
          <div className="collection-search">
            <Search className="size-4" aria-hidden />
            <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("collections.search")} aria-label={t("collections.search")} />
            {q && <button type="button" aria-label={t("common.clear_filters")} onClick={() => setQ("")}><X className="size-4" aria-hidden /></button>}
          </div>
          <p className="collection-result-count" role="status" aria-live="polite">{status}</p>
        </div>

        {list.isPending ? (
          <ListSkeleton rows={4} />
        ) : list.isError ? (
          <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={Database}
            title={t(term ? "common.no_results" : "collections.empty")}
            action={term ? <Button variant="outline" onClick={() => setQ("")}>{t("common.clear_filters")}</Button>
              : can(user, "collections", "create") && <Button onClick={() => router.push("/collections/new")}><Plus className="size-4" />{t("collections.new")}</Button>}
          />
        ) : (
          groups.map(([title, items]) => items.length > 0 && (
            <Group key={title} title={t(title)}>
              {items.sort(byActivity).map((c) => (
                <Row key={c.id} c={c} pinned={isPinned(c)} current={storedId === c.id} onPin={() => togglePin(c.id)} onRemove={(c, kind) => setRemoving({ c, kind })} />
              ))}
            </Group>
          ))
        )}
      </div>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t(removing?.kind === "delete" ? "collections.delete" : "collections.leave")}
        description={removing?.kind === "delete" ? t("collections.delete_hint") : t("collections.leave_confirm").replace("{name}", removing?.c.name ?? "")}
        destructive
        confirmLabel={t(removing?.kind === "delete" ? "collections.delete" : "collections.leave")}
        onConfirm={async () => {
          if (!removing) return;
          const { c, kind } = removing;
          // The dialog shows a refusal, such as the last owner leaving or records left.
          await (kind === "delete" ? collections.remove([c.id]) : collections.leave(c.id));
          if (storedId === c.id) setCollection(null);
          if (pinned.includes(c.id)) togglePin(c.id);
          setRemoving(null);
          toast.success(t(kind === "delete" ? "collections.deleted" : "collections.left"));
          void client.invalidateQueries({ queryKey: ["collections"] });
        }}
      />
    </div>
  );
}

function Group({ title, children }: { title: string; children: React.ReactNode[] }) {
  const id = useId();
  return (
    <section className="collection-group" aria-labelledby={id}>
      <h2 id={id}>{title}<span>{children.length}</span></h2>
      <ul aria-labelledby={id} className="collection-list">{children}</ul>
    </section>
  );
}

function Row({ c, pinned, current, onPin, onRemove }: RowProps) {
  const { t, locale } = useI18n();
  const router = useRouter();
  const relative = useRelative();
  const number = new Intl.NumberFormat(locale);
  const base = `/collections/${c.id}`;
  const open = () => router.push(`${base}/overview`);
  const owner = roleAllows(c.role, "owner");
  const activity = relative(c.last_activity ?? c.updated_at);
  const stats: [typeof FileJson, number, MessageKey, MessageKey][] = [
    [FileJson, c.record_count ?? 0, "collections.unit_record", "collections.unit_records"],
    [ListTree, c.field_count ?? 0, "collections.unit_field", "collections.unit_fields"],
    [Users, c.member_count ?? 0, "collections.unit_member", "collections.unit_members"],
  ];
  return (
    <li className="collection-row" data-current={current ? "" : undefined}>
      <button type="button" className="collection-open" onClick={open} aria-label={c.name}>
        <CollectionAvatar collection={c} className="collection-avatar" />
        <span className="collection-copy">
          <span className="collection-name">{c.name}</span>
          <span className="collection-details">
            {c.role && <Badge variant="secondary">{t(`members.role.${c.role}`)}</Badge>}
            {current && <Badge variant="outline" className="collection-current">{t("collections.current")}</Badge>}
            {c.description && <span className="collection-description" title={c.description}>{c.description}</span>}
          </span>
        </span>
      </button>
      <div className="collection-stats">
        {stats.map(([Icon, n, one, many]) => (
          <span className="collection-stat" key={one}><Icon className="size-3.5" aria-hidden /><span>{number.format(n)} {t(n === 1 ? one : many)}</span></span>
        ))}
        <Tooltip>
          <TooltipTrigger render={<span className="collection-activity">{t("collections.active").replace("{rel}", activity.rel)}</span>} />
          <TooltipContent>{activity.abs}</TooltipContent>
        </Tooltip>
      </div>
      <div className="collection-actions">
        <Button variant="ghost" size="sm" onClick={open} aria-label={`${t("collections.open")} ${c.name}`} className="collection-open-action">
          {t("collections.open")}<ArrowRight className="size-4" aria-hidden />
        </Button>
        <Tooltip>
          <TooltipTrigger render={
            <button type="button" onClick={onPin} aria-label={`${t(pinned ? "collections.unpin" : "collections.pin")}: ${c.name}`} aria-pressed={pinned} className="collection-icon-action" data-pinned={pinned ? "" : undefined}>
              <Star className="size-4" fill={pinned ? "currentColor" : "none"} aria-hidden />
            </button>
          } />
          <TooltipContent>{t(pinned ? "collections.unpin" : "collections.pin")}</TooltipContent>
        </Tooltip>
        <DropdownMenu>
          <DropdownMenuTrigger aria-label={`${t("common.actions")}: ${c.name}`} className="collection-icon-action"><MoreVertical className="size-4" aria-hidden /></DropdownMenuTrigger>
          <DropdownMenuContent align="end" side="bottom" sideOffset={6} className="min-w-48">
            <DropdownMenuItem onClick={open}><ArrowRight className="size-4" aria-hidden />{t("collections.open")}</DropdownMenuItem>
            <DropdownMenuItem onClick={() => router.push(base)}><FileJson className="size-4" aria-hidden />{t("collections.tab.records")}</DropdownMenuItem>
            {owner && <>
              <DropdownMenuItem onClick={() => router.push(`${base}/members`)}><UserCog className="size-4" aria-hidden />{t("collections.tab.members")}</DropdownMenuItem>
              <DropdownMenuItem onClick={() => router.push(`${base}/settings`)}><Settings2 className="size-4" aria-hidden />{t("collections.tab.settings")}</DropdownMenuItem>
            </>}
            {(c.member !== false || (owner && !c.record_count)) && <DropdownMenuSeparator />}
            {c.member !== false && <DropdownMenuItem variant="destructive" onClick={() => onRemove(c, "leave")}><LogOut className="size-4" aria-hidden />{t("collections.leave")}</DropdownMenuItem>}
            {owner && !c.record_count && <DropdownMenuItem variant="destructive" onClick={() => onRemove(c, "delete")}><Trash2 className="size-4" aria-hidden />{t("collections.delete")}</DropdownMenuItem>}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </li>
  );
}
