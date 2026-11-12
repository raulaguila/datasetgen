"use client";

import Link from "next/link";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { toast } from "sonner";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, KeyRound, Pencil, Plus, Search, Trash2 } from "lucide-react";
import { ActiveBadge, IconAction, PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { SortHead, SortSelect } from "@/components/sort-head";
import { SelectAllHead, SelectCell, SelectionBar, useSelection } from "@/components/selection";
import { ListSkeleton } from "@/components/skeletons";
import { TablePagination, emptyPageFallback } from "@/components/table-pagination";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { roles } from "@/lib/api/endpoints";
import type { Action, Role, SortOrder } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useGoToPage, useListParams } from "@/lib/hooks/use-list-params";
import { errorMessage } from "@/lib/errors";
import { deleteWithUndo } from "@/lib/undo-delete";
import { useI18n } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";
import { can, resourceText } from "@/lib/permissions";

/** "Produtos: ler, editar" chips, three at most. */
function PermissionChips({ role }: { role: Role }) {
  const { t } = useI18n();
  if ("*" in role.permissions) return <Badge variant="secondary">{t("roles.all_permissions")}</Badge>;
  const entries = Object.entries(role.permissions).filter(([, a]) => a.length > 0);
  if (entries.length === 0) return <span className="text-muted-foreground">-</span>;
  const chip = ([res, actions]: [string, Action[]]) =>
    `${resourceText(t, res).label}: ${actions.map((a) => t(`action.${a}` as MessageKey).toLowerCase()).join(", ")}`;
  return (
    <div className="flex flex-wrap gap-1.5">
      {entries.slice(0, 3).map((e) => <Badge key={e[0]} variant="outline" className="font-normal">{chip(e)}</Badge>)}
      {entries.length > 3 && <Badge variant="outline" title={entries.slice(3).map(chip).join("\n")}>+{entries.length - 3}</Badge>}
    </div>
  );
}

export default function RolesPage() {
  const { t } = useI18n();
  const format = useFormatDate();
  const { user } = useMe();
  const router = useRouter();
  const client = useQueryClient();
  const canEdit = can(user, "roles", "update");
  const canDelete = can(user, "roles", "delete");
  const [params, set] = useListParams({ page: 1, limit: 25, q: "", sort: "name", order: "asc" });
  const { page, limit } = params;
  const sort = params.sort === "created_at" ? "created_at" : "name";
  const order: SortOrder = params.order === "desc" ? "desc" : "asc";
  const [search, setSearch] = useState(params.q);
  const [hidden, setHidden] = useState<string[]>([]);
  const q = useDebounced(params.q);
  const list = useQuery({
    queryKey: ["roles", "list", page, limit, q, sort, order],
    queryFn: ({ signal }) => roles.list({ page, limit, search: q, sort, order }, signal),
    placeholderData: keepPreviousData,
  });
  const fallback = list.isPlaceholderData ? null : emptyPageFallback(page, limit, list.data);
  useGoToPage(fallback, set); // the last rows of the last page went away
  const items = list.data?.items.filter((r) => !hidden.includes(r.id)) ?? [];
  // A default role cannot be deleted, so it is never ticked.
  const selection = useSelection(items.filter((r) => !r.is_default).map((r) => r.id));
  const sortProps = { sort, order, onSort: (s: string, o: SortOrder) => set({ sort: s, order: o, page: 1 }) };
  const removeSelected = () => {
    const ids = selection.ids;
    selection.clear();
    setHidden((h) => [...h, ...ids]);
    deleteWithUndo({
      message: t("common.deleted_n").replace("{n}", String(ids.length)),
      undoLabel: t("common.undo"),
      run: () => roles.remove(ids),
      onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
      onSettled: async () => {
        await client.invalidateQueries({ queryKey: ["roles"] });
        setHidden((h) => h.filter((id) => !ids.includes(id)));
      },
    });
  };
  const open = (id: string) => router.push(`/admin/roles/${id}`);

  return (
    <>
      <PageHeader
        title={t("nav.roles")}
        description={t("roles.subtitle")}
        action={can(user, "roles", "create") && <Button onClick={() => router.push("/admin/roles/new")}><Plus className="size-4" />{t("roles.new")}</Button>}
      />
      <PageBody>
        <div className="flex flex-wrap gap-2 max-sm:grid max-sm:grid-cols-2">
          <div className="relative min-w-56 flex-1 sm:max-w-sm max-sm:col-span-2 max-sm:min-w-0">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input type="search" aria-label={t("common.search")} placeholder={t("roles.search")} className="pl-9" value={search} onChange={(e) => { setSearch(e.target.value); set({ q: e.target.value.trim(), page: 1 }); }} />
          </div>
          <SortSelect fields={[{ field: "name", label: t("auth.name") }, { field: "created_at", label: t("common.created_at") }]} className="max-sm:col-span-2" {...sortProps} />
        </div>
        {list.isPending ? (
          <ListSkeleton />
        ) : list.isError ? (
          <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : items.length === 0 ? (
          <EmptyState
            icon={KeyRound}
            title={t(q ? "common.no_results" : "roles.empty")}
            action={q && <Button variant="outline" onClick={() => { setSearch(""); set({ q: "", page: 1 }); }}>{t("common.clear_filters")}</Button>}
          />
        ) : (
          <Card className="gap-0 overflow-hidden p-0">
            <Table data-responsive data-compact>
              <TableHeader>
                <TableRow>
                  {canDelete && <SelectAllHead selection={selection} />}
                  <SortHead field="name" label={t("auth.name")} {...sortProps} />
                  <TableHead>{t("roles.permissions")}</TableHead>
                  <TableHead className="w-24 text-right">{t("roles.members")}</TableHead>
                  <TableHead className="w-28">{t("roles.state")}</TableHead>
                  <SortHead field="created_at" label={t("common.created_at")} className="w-44" {...sortProps} />
                  <TableHead className="w-14"><span className="sr-only">{t("common.actions")}</span></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {items.map((r) => (
                  <TableRow key={r.id} className="cursor-pointer" onClick={() => open(r.id)}>
                    {canDelete && <SelectCell selection={selection} id={r.id} label={r.name} disabled={r.is_default} />}
                    <TableCell data-label={t("auth.name")} data-cell="name">
                      <Link href={`/admin/roles/${r.id}`} className="font-medium hover:underline" onClick={(e) => e.stopPropagation()}>{r.name}</Link>
                      {r.is_default && <Badge variant="outline" className="ml-2">{t("roles.default")}</Badge>}
                    </TableCell>
                    <TableCell data-label={t("roles.permissions")} data-cell="sub"><PermissionChips role={r} /></TableCell>
                    <TableCell data-label={t("roles.members")} data-cell="meta" className="text-right tabular-nums">{r.member_count}<span className="lg:hidden"> {t(r.member_count === 1 ? "roles.member_one" : "roles.members").toLowerCase()}</span></TableCell>
                    <TableCell data-label={t("roles.state")} data-cell="status"><ActiveBadge enabled={r.enabled} /></TableCell>
                    <TableCell data-label={t("common.created_at")} data-cell="date" className="text-muted-foreground">{format(r.created_at)}</TableCell>
                    <TableCell data-cell="action" className="text-right">
                      <IconAction label={t(canEdit ? "common.edit" : "common.view")} target={r.name} onClick={() => open(r.id)}>
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
        <p className="text-xs text-muted-foreground">{t("roles.protected_note")}</p>
        <SelectionBar selection={selection}>
          <Button variant="destructive" onClick={removeSelected}><Trash2 className="size-4" />{t("common.delete_selected")}</Button>
        </SelectionBar>
      </PageBody>
    </>
  );
}
