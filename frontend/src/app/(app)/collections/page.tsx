"use client";

import { useId, useState } from "react";
import { useRouter } from "next/navigation";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, Copy, Database, FileJson, ListTree, LogOut, MoreVertical, Plus, RotateCcw, Search, Settings2, Star, Trash2, UserCog, Users, X } from "lucide-react";
import { toast } from "sonner";
import { PageHeader, useRelativeTime } from "@/components/bits";
import { CollectionAvatar } from "@/components/collection-avatar";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { DuplicateCollectionDialog } from "@/components/duplicate-collection-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { collections, tools } from "@/lib/api/endpoints";
import type { Collection, TrashedCollection } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useMe } from "@/lib/context/me";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import { can, roleAllows } from "@/lib/permissions";
import { useUIStore } from "@/lib/store/ui";

// shortcut: one request of up to 100 collections, grouped in the browser; a search goes to the server too, so it
// reaches past the first 100. Page on the server when one person holds more than 100.
const LIMIT = 100;

function byActivity(a: Collection, b: Collection): number {
  return (Date.parse(b.last_activity ?? b.updated_at) || 0) - (Date.parse(a.last_activity ?? a.updated_at) || 0) || a.name.localeCompare(b.name);
}

interface RowProps {
  c: Collection;
  pinned: boolean;
  onPin: () => void;
  onRemove: (c: Collection, kind: "leave" | "delete") => void;
  /** Set when the caller may create collections. */
  onDuplicate?: (c: Collection) => void;
}

export default function CollectionsPage() {
  const { t } = useI18n();
  const { user } = useMe();
  const client = useQueryClient();
  const router = useRouter();
  const [q, setQ] = useState("");
  const search = useDebounced(q.trim(), 300);
  const list = useQuery({
    queryKey: ["collections", "manage", search],
    queryFn: ({ signal }) => collections.list({ page: 1, limit: LIMIT, sort: "name", order: "asc", details: true, search: search || undefined }, signal),
    placeholderData: keepPreviousData,
  });
  const pinned = useUIStore((s) => s.pinned);
  const togglePin = useUIStore((s) => s.togglePin);
  const storedId = useUIStore((s) => s.collectionId);
  // A short list needs no search box or group headings.
  const FEW = 6;
  const setCollection = useUIStore((s) => s.setCollection);
  const [removing, setRemoving] = useState<{ c: Collection; kind: "leave" | "delete" } | null>(null);
  const [duplicating, setDuplicating] = useState<Collection | null>(null);
  const canCreate = can(user, "collections", "create");

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
  const searchable = all.length > FEW || !!term;
  const headed = groups.filter(([, items]) => items.length > 0).length > 1;

  return (
    <div data-ui-collection-management>
      <PageHeader
        title={t("nav.collections")}
        description={t("collections.subtitle")}
        action={can(user, "collections", "create") && <Button onClick={() => router.push("/collections/new")}><Plus className="size-4" />{t("collections.new")}</Button>}
      />
      <div className="collection-management-body">
        <div className="collection-toolbar">
          {searchable && <div className="collection-search">
            <Search className="size-4" aria-hidden />
            <Input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("collections.search")} aria-label={t("collections.search")} />
            {q && <button type="button" aria-label={t("common.clear_filters")} onClick={() => setQ("")}><X className="size-4" aria-hidden /></button>}
          </div>}
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
            description={term ? undefined : t(canCreate ? "collections.empty_desc" : "collections.empty_ask")}
            action={term ? <Button variant="outline" onClick={() => setQ("")}>{t("common.clear_filters")}</Button>
              : can(user, "collections", "create") && <Button onClick={() => router.push("/collections/new")}><Plus className="size-4" />{t("collections.new")}</Button>}
          />
        ) : (
          groups.map(([title, items]) => items.length > 0 && (
            <Group key={title} title={t(title)} headed={headed}>
              {items.sort(byActivity).map((c) => (
                <Row key={c.id} c={c} pinned={isPinned(c)} onPin={() => togglePin(c.id)} onRemove={(c, kind) => setRemoving({ c, kind })} onDuplicate={canCreate ? setDuplicating : undefined} />
              ))}
            </Group>
          ))
        )}
      </div>

      {duplicating && <DuplicateCollectionDialog source={duplicating} onClose={() => setDuplicating(null)} />}
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t(removing?.kind === "delete" ? "collections.delete" : "collections.leave")}
        description={removing?.kind === "delete" ? plural(t("collections.delete_trash_hint"), removing.c.record_count ?? 0) : t("collections.leave_confirm").replace("{name}", removing?.c.name ?? "")}
        destructive
        confirmLabel={t(removing?.kind === "delete" ? "collections.delete" : "collections.leave")}
        onConfirm={async () => {
          if (!removing) return;
          const { c, kind } = removing;
          // The dialog shows a refusal, such as the last owner leaving.
          await (kind === "delete" ? collections.remove([c.id]) : collections.leave(c.id));
          if (storedId === c.id) setCollection(null);
          if (pinned.includes(c.id)) togglePin(c.id);
          setRemoving(null);
          toast.success(t(kind === "delete" ? "collections.deleted" : "collections.left"));
          void client.invalidateQueries({ queryKey: ["collections"] });
          void client.invalidateQueries({ queryKey: ["trash", "collections"] });
        }}
      />
      <DeletedCollections />
    </div>
  );
}

function Group({ title, headed, children }: { title: string; headed: boolean; children: React.ReactNode[] }) {
  const id = useId();
  return (
    <section className="collection-group" aria-labelledby={id}>
      <h2 id={id} className={headed ? undefined : "sr-only"}>{title}</h2>
      <ul aria-labelledby={id} className="collection-list">{children}</ul>
    </section>
  );
}

function Row({ c, pinned, onPin, onRemove, onDuplicate }: RowProps) {
  const { t, locale } = useI18n();
  const router = useRouter();
  const relative = useRelativeTime();
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
    <li className="collection-row">
      <button type="button" className="collection-open" onClick={open} aria-label={c.name}>
        <CollectionAvatar collection={c} className="collection-avatar" />
        <span className="collection-copy">
          <span className="collection-name">{c.name}</span>
          <span className="collection-details">
            {c.role && <Badge variant="secondary">{t(`members.role.${c.role}`)}</Badge>}
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
              <DropdownMenuItem onClick={() => router.push(`${base}/settings`)}><Settings2 className="size-4" aria-hidden />{t("collections.general_settings")}</DropdownMenuItem>
            </>}
            {onDuplicate && <DropdownMenuItem onClick={() => onDuplicate(c)}><Copy className="size-4" aria-hidden />{t("duplicate.title")}</DropdownMenuItem>}
            {(c.member !== false || owner) && <DropdownMenuSeparator />}
            {c.member !== false && <DropdownMenuItem variant="destructive" onClick={() => onRemove(c, "leave")}><LogOut className="size-4" aria-hidden />{t("collections.leave")}</DropdownMenuItem>}
            {owner && <DropdownMenuItem variant="destructive" onClick={() => onRemove(c, "delete")}><Trash2 className="size-4" aria-hidden />{t("collections.delete")}</DropdownMenuItem>}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </li>
  );
}

/** Collections deleted in the last days: their owner restores them or deletes them for good. */
function DeletedCollections() {
  const { t } = useI18n();
  const client = useQueryClient();
  const relative = useRelativeTime();
  const list = useQuery({ queryKey: ["trash", "collections"], queryFn: () => tools.trashedCollections() });
  const [purging, setPurging] = useState<TrashedCollection | null>(null);
  const items = list.data?.items ?? [];
  if (!items.length) return null;
  const refresh = () => {
    void client.invalidateQueries({ queryKey: ["trash", "collections"] });
    void client.invalidateQueries({ queryKey: ["collections"] });
  };
  const restore = async (c: TrashedCollection) => {
    try {
      await tools.restoreCollection(c.id);
      toast.success(t("trash.collection_restored").replace("{name}", c.name));
      refresh();
    } catch (err) {
      toast.error(errorMessage(err, "common.save_failed", t));
    }
  };
  return (
    <details className="collection-management-body mt-6">
      <summary className="cursor-pointer text-sm font-medium text-muted-foreground hover:text-foreground">
        {t("trash.collections").replace("{n}", String(items.length))}
      </summary>
      <p className="mt-2 text-xs text-muted-foreground">{t("trash.retention").replace("{n}", String(list.data?.retention_days ?? 30))}</p>
      <ul className="mt-2 divide-y rounded-lg border">
        {items.map((c) => (
          <li key={c.id} className="flex flex-wrap items-center gap-3 px-3 py-2">
            <CollectionAvatar collection={c} className="size-8 rounded-md text-xs font-semibold" />
            <div className="min-w-40 flex-1">
              <p className="truncate text-sm font-medium">{c.name}</p>
              <p className="text-xs text-muted-foreground">
                {t(c.record_count === 1 ? "nav.record_one" : "nav.record_many").replace("{n}", String(c.record_count))} · {t("trash.deleted_when").replace("{when}", relative(c.deleted_at).rel)}
              </p>
            </div>
            <Button size="sm" variant="outline" onClick={() => void restore(c)}><RotateCcw className="size-4" />{t("trash.restore")}</Button>
            <Button size="sm" variant="ghost" className="text-destructive" onClick={() => setPurging(c)}><Trash2 className="size-4" />{t("trash.purge")}</Button>
          </li>
        ))}
      </ul>
      <ConfirmDialog
        open={!!purging}
        onOpenChange={(o) => !o && setPurging(null)}
        title={t("trash.purge_collection_title").replace("{name}", purging?.name ?? "")}
        description={t("trash.purge_desc")}
        confirmLabel={t("trash.purge")}
        destructive
        onConfirm={async () => {
          if (!purging) return;
          await tools.purgeCollection(purging.id);
          setPurging(null);
          refresh();
        }}
      />
    </details>
  );
}
