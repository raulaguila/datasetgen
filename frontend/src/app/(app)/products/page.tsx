"use client";

import Link from "next/link";
import { useState } from "react";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, Package, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { ActiveBadge, IconAction, PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { SortHead, SortSelect } from "@/components/sort-head";
import { SelectAllHead, SelectCell, SelectionBar, useSelection } from "@/components/selection";
import { ListSkeleton } from "@/components/skeletons";
import { TablePagination, emptyPageFallback } from "@/components/table-pagination";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { products } from "@/lib/api/endpoints";
import type { SortOrder } from "@/lib/api/types";
import { CURRENCY } from "@/lib/brand";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { searchTerm } from "@/lib/utils";
import { useGoToPage, useListParams } from "@/lib/hooks/use-list-params";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { can } from "@/lib/permissions";
import { deleteWithUndo } from "@/lib/undo-delete";

const SORTS = ["name", "price", "created_at"];
type State = "all" | "on" | "off";

export default function ProductsPage() {
  const { t, locale } = useI18n();
  const format = useFormatDate();
  const { user } = useMe();
  const client = useQueryClient();
  const [params, set] = useListParams({ page: 1, limit: 25, q: "", sort: "created_at", order: "desc", state: "all" });
  const { page, limit } = params;
  const sort = SORTS.includes(params.sort) ? params.sort : "created_at";
  const order: SortOrder = params.order === "asc" ? "asc" : "desc";
  const state: State = params.state === "on" || params.state === "off" ? params.state : "all";
  const [search, setSearch] = useState(params.q);
  const [hidden, setHidden] = useState<string[]>([]);
  const router = useRouter();
  const q = searchTerm(useDebounced(params.q));
  const list = useQuery({
    queryKey: ["products", page, limit, q, sort, order, state],
    queryFn: ({ signal }) => products.list({ page, limit, search: q, sort, order, enabled: state === "all" ? undefined : state === "on" }, signal),
    placeholderData: keepPreviousData,
  });
  const fallback = list.isPlaceholderData ? null : emptyPageFallback(page, limit, list.data);
  useGoToPage(fallback, set); // the last rows of the last page went away
  const items = list.data?.items.filter((p) => !hidden.includes(p.id)) ?? [];
  const canEdit = can(user, "products", "update");
  const selection = useSelection(items.map((p) => p.id));
  const open = (id: string) => router.push(`/products/${id}`);
  const canDelete = can(user, "products", "delete");
  const money = new Intl.NumberFormat(locale, { style: "currency", currency: CURRENCY });
  const stateItems: Record<State, string> = { all: t("products.all_states"), on: t("roles.enabled"), off: t("roles.disabled") };

  const sortProps = { sort, order, onSort: (s: string, o: SortOrder) => set({ sort: s, order: o, page: 1 }) };
  const removeSelected = () => {
    const ids = selection.ids;
    selection.clear();
    setHidden((h) => [...h, ...ids]);
    deleteWithUndo({
      message: t("common.deleted_n").replace("{n}", String(ids.length)),
      undoLabel: t("common.undo"),
      run: () => products.remove(ids),
      onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
      onSettled: async () => {
        await client.invalidateQueries({ queryKey: ["products"] });
        setHidden((h) => h.filter((id) => !ids.includes(id)));
      },
    });
  };

  return (
    <>
      <PageHeader
        title={t("nav.products")}
        description={t("products.subtitle")}
        action={can(user, "products", "create") && <Button onClick={() => open("new")}><Plus className="size-4" />{t("products.new")}</Button>}
      />
      <PageBody>
        <div className="flex flex-wrap gap-2 max-sm:grid max-sm:grid-cols-2">
          <div className="relative min-w-56 flex-1 sm:max-w-sm max-sm:col-span-2 max-sm:min-w-0">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input type="search" aria-label={t("common.search")} placeholder={t("products.search")} className="pl-9" value={search} onChange={(e) => { setSearch(e.target.value); set({ q: e.target.value.trim(), page: 1 }); }} />
          </div>
          <Select value={state} items={stateItems} onValueChange={(v) => set({ state: v as State, page: 1 })}>
            <SelectTrigger className="w-44 max-sm:col-span-2 max-sm:w-full" aria-label={t("roles.state")}><SelectValue /></SelectTrigger>
            <SelectContent>
              {Object.entries(stateItems).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}
            </SelectContent>
          </Select>
          <SortSelect fields={[{ field: "name", label: t("auth.name") }, { field: "price", label: t("products.price") }, { field: "created_at", label: t("common.created_at") }]} {...sortProps} className="max-sm:col-span-2" />
          {search.trim() !== "" && !searchTerm(search) && <p className="basis-full text-xs text-muted-foreground max-sm:col-span-2" role="status">{t("common.search_min")}</p>}
        </div>

        {list.isPending ? (
          <ListSkeleton />
        ) : list.isError ? (
          <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : items.length === 0 ? (
          <EmptyState
            icon={Package}
            title={t(q || state !== "all" ? "common.no_results" : "products.empty")}
            action={(q || state !== "all") && <Button variant="outline" onClick={() => { setSearch(""); set({ q: "", state: "all", page: 1 }); }}>{t("common.clear_filters")}</Button>}
          />
        ) : (
          <Card className="gap-0 overflow-hidden p-0">
            <Table data-responsive data-compact>
              <TableHeader>
                <TableRow>
                  {canDelete && <SelectAllHead selection={selection} />}
                  <SortHead field="name" label={t("auth.name")} {...sortProps} />
                  <SortHead field="price" label={t("products.price")} className="w-36 text-right" {...sortProps} />
                  <TableHead className="w-28">{t("roles.state")}</TableHead>
                  <SortHead field="created_at" label={t("common.created_at")} className="w-44" {...sortProps} />
                  <TableHead className="w-14"><span className="sr-only">{t("common.actions")}</span></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((p) => (
                  <TableRow key={p.id} className="cursor-pointer" onClick={() => open(p.id)}>
                    {canDelete && <SelectCell selection={selection} id={p.id} label={p.name} />}
                    <TableCell data-label={t("auth.name")} data-cell="name" className="w-full max-w-0">
                      <Link href={`/products/${p.id}`} className="block truncate font-medium hover:underline" title={p.name} onClick={(e) => e.stopPropagation()}>{p.name}</Link>
                    </TableCell>
                    <TableCell data-label={t("products.price")} data-cell="sub" className="text-right font-mono tabular-nums">{money.format(Number(p.price))}</TableCell>
                    <TableCell data-label={t("roles.state")} data-cell="status">
                      <ActiveBadge enabled={p.enabled} />
                    </TableCell>
                    <TableCell data-label={t("common.created_at")} data-cell="date" className="text-muted-foreground">{format(p.created_at)}</TableCell>
                    <TableCell data-cell="action" className="text-right" onClick={(e) => e.stopPropagation()}>
                      <IconAction label={t(canEdit ? "common.edit" : "common.view")} target={p.name} onClick={() => open(p.id)}>
                        {canEdit ? <Pencil className="size-4" /> : <Eye className="size-4" />}
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
        <SelectionBar selection={selection}>
          <Button variant="destructive" onClick={removeSelected}><Trash2 className="size-4" />{t("common.delete_selected")}</Button>
        </SelectionBar>
      </PageBody>
    </>
  );
}
