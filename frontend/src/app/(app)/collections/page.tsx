"use client";

import Link from "next/link";
import { useState } from "react";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { Database, Eye, LogOut, Pencil, Pin, PinOff, Plus, Search } from "lucide-react";
import { toast } from "sonner";
import { useRouter } from "next/navigation";
import { IconAction, PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { CollectionAvatar } from "@/components/collection-avatar";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { SortHead, SortSelect } from "@/components/sort-head";
import { ListSkeleton } from "@/components/skeletons";
import { TablePagination, emptyPageFallback } from "@/components/table-pagination";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { collections } from "@/lib/api/endpoints";
import type { Collection, SortOrder } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { searchTerm } from "@/lib/utils";
import { useGoToPage, useListParams } from "@/lib/hooks/use-list-params";
import { useI18n } from "@/lib/i18n/provider";
import { can, roleAllows } from "@/lib/permissions";
import { useUIStore } from "@/lib/store/ui";

const SORTS = ["name", "updated_at", "created_at"];

// Deleting lives on the detail page: a collection goes only once it has no records.
export default function CollectionsPage() {
  const { t, locale } = useI18n();
  const format = useFormatDate();
  const { user } = useMe();
  const [params, set] = useListParams({ page: 1, limit: 25, q: "", sort: "name", order: "asc" });
  const { page, limit } = params;
  const sort = SORTS.includes(params.sort) ? params.sort : "name";
  const order: SortOrder = params.order === "desc" ? "desc" : "asc";
  const [search, setSearch] = useState(params.q);
  const router = useRouter();
  const q = searchTerm(useDebounced(params.q));
  const list = useQuery({
    queryKey: ["collections", page, limit, q, sort, order],
    queryFn: ({ signal }) => collections.list({ page, limit, search: q, sort, order }, signal),
    placeholderData: keepPreviousData,
  });
  const fallback = list.isPlaceholderData ? null : emptyPageFallback(page, limit, list.data);
  useGoToPage(fallback, set);
  const items = list.data?.items ?? [];
  const client = useQueryClient();
  const pinned = useUIStore((s) => s.pinned);
  const togglePin = useUIStore((s) => s.togglePin);
  const storedId = useUIStore((s) => s.collectionId);
  const setCollection = useUIStore((s) => s.setCollection);
  const [leaving, setLeaving] = useState<Collection | null>(null);
  const open = (id: string) => router.push(id === "new" ? "/collections/new" : `/collections/${id}/overview`);
  const count = new Intl.NumberFormat(locale);
  const sortProps = { sort, order, onSort: (s: string, o: SortOrder) => set({ sort: s, order: o, page: 1 }) };

  return (
    <>
      <PageHeader
        title={t("nav.collections")}
        description={t("collections.subtitle")}
        action={can(user, "collections", "create") && <Button onClick={() => open("new")}><Plus className="size-4" />{t("collections.new")}</Button>}
      />
      <PageBody>
        <div className="flex flex-wrap gap-2 max-sm:grid max-sm:grid-cols-2">
          <div className="relative min-w-56 flex-1 sm:max-w-sm max-sm:col-span-2 max-sm:min-w-0">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input type="search" aria-label={t("common.search")} placeholder={t("collections.search")} className="pl-9" value={search} onChange={(e) => { setSearch(e.target.value); set({ q: e.target.value.trim(), page: 1 }); }} />
          </div>
          <SortSelect fields={[{ field: "name", label: t("auth.name") }, { field: "updated_at", label: t("common.updated_at") }, { field: "created_at", label: t("common.created_at") }]} {...sortProps} className="max-sm:col-span-2" />
          {search.trim() !== "" && !searchTerm(search) && <p className="basis-full text-xs text-muted-foreground max-sm:col-span-2" role="status">{t("common.search_min")}</p>}
        </div>

        {list.isPending ? (
          <ListSkeleton />
        ) : list.isError ? (
          <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : items.length === 0 ? (
          <EmptyState
            icon={Database}
            title={t(q ? "common.no_results" : "collections.empty")}
            action={q && <Button variant="outline" onClick={() => { setSearch(""); set({ q: "", page: 1 }); }}>{t("common.clear_filters")}</Button>}
          />
        ) : (
          <Card className="gap-0 overflow-hidden p-0">
            <Table data-responsive data-compact>
              <TableHeader>
                <TableRow>
                  <SortHead field="name" label={t("auth.name")} {...sortProps} />
                  <TableHead className="w-28 text-right">{t("collections.record_count")}</TableHead>
                  <SortHead field="updated_at" label={t("common.updated_at")} className="w-44" {...sortProps} />
                  <TableHead className="w-14"><span className="sr-only">{t("common.actions")}</span></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((c) => (
                  <TableRow key={c.id} className="cursor-pointer" onClick={() => open(c.id)}>
                    <TableCell data-label={t("auth.name")} data-cell="name" className="w-full max-w-0">
                      <div className="flex items-center gap-3">
                        <CollectionAvatar collection={c} className="size-8 rounded-full text-xs font-semibold" />
                        <div className="min-w-0">
                          <Link href={`/collections/${c.id}/overview`} className="block truncate font-medium hover:underline" title={c.name} onClick={(e) => e.stopPropagation()}>{c.name}</Link>
                          {c.description && <span className="block truncate text-xs text-muted-foreground" title={c.description}>{c.description}</span>}
                        </div>
                      </div>
                    </TableCell>
                    <TableCell data-label={t("collections.record_count")} data-cell="sub" className="text-right font-mono tabular-nums">{count.format(c.record_count ?? 0)}</TableCell>
                    <TableCell data-label={t("common.updated_at")} data-cell="date" className="text-muted-foreground">{format(c.updated_at)}</TableCell>
                    <TableCell data-cell="action" className="text-right whitespace-nowrap" onClick={(e) => e.stopPropagation()}>
                      <IconAction label={t(pinned.includes(c.id) ? "collections.unpin" : "collections.pin")} target={c.name} onClick={() => togglePin(c.id)}>
                        {pinned.includes(c.id) ? <PinOff className="size-4" /> : <Pin className="size-4" />}
                      </IconAction>
                      {c.member && <IconAction label={t("collections.leave")} target={c.name} onClick={() => setLeaving(c)}><LogOut className="size-4" /></IconAction>}
                      <IconAction label={t(roleAllows(c.role, "owner") ? "common.edit" : "common.view")} target={c.name} onClick={() => roleAllows(c.role, "owner") ? router.push(`/collections/${c.id}/settings`) : open(c.id)}>
                        {roleAllows(c.role, "owner") ? <Pencil className="size-4" /> : <Eye className="size-4" />}
                      </IconAction>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            <TablePagination
              page={page}
              pageSize={limit}
              totalItems={list.data.pagination.total_items}
              onPageChange={(n) => set({ page: n })}
              onPageSizeChange={(n) => set({ limit: n, page: 1 })}
            />
          </Card>
        )}
      </PageBody>
      <ConfirmDialog
        open={!!leaving}
        onOpenChange={(o) => !o && setLeaving(null)}
        title={t("collections.leave")}
        description={t("collections.leave_confirm").replace("{name}", leaving?.name ?? "")}
        destructive
        confirmLabel={t("collections.leave")}
        onConfirm={async () => {
          if (!leaving) return;
          await collections.leave(leaving.id); // the dialog shows a refusal, such as the last owner
          if (storedId === leaving.id) setCollection(null);
          if (pinned.includes(leaving.id)) togglePin(leaving.id);
          setLeaving(null);
          toast.success(t("collections.left"));
          void client.invalidateQueries({ queryKey: ["collections"] });
        }}
      />
    </>
  );
}
