"use client";

import { useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { Eye, Pencil, Search, UserCheck, UserPlus, UserX, Users } from "lucide-react";
import { toast } from "sonner";
import { IconAction, PageBody, PageHeader, StatusBadge, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
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
import { roles, users } from "@/lib/api/endpoints";
import type { SortOrder, UserStatus } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { searchTerm } from "@/lib/utils";
import { useGoToPage, useListParams } from "@/lib/hooks/use-list-params";
import type { MessageKey } from "@/lib/i18n/messages";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n/provider";
import { can } from "@/lib/permissions";

const STATUSES: UserStatus[] = ["active", "pending", "locked", "disabled"];
const SORTS = ["name", "email", "status", "created_at"];

export default function UsersPage() {
  const { t } = useI18n();
  const format = useFormatDate();
  const router = useRouter();
  const { user } = useMe();
  // Editing a user means changing state or roles.
  const canEdit = can(user, "users", "update") || can(user, "roles", "update");
  const canBulk = can(user, "users", "update");
  const client = useQueryClient();
  const [bulk, setBulk] = useState<"disable" | "enable" | null>(null);
  const [params, set] = useListParams({ page: 1, limit: 25, q: "", status: "", role: "", sort: "name", order: "asc" });
  const { page, limit, role } = params;
  const status = STATUSES.includes(params.status as UserStatus) ? (params.status as UserStatus) : "";
  const sort = SORTS.includes(params.sort) ? params.sort : "name";
  const order: SortOrder = params.order === "desc" ? "desc" : "asc";
  const [search, setSearch] = useState(params.q);
  const q = searchTerm(useDebounced(params.q));
  const list = useQuery({
    queryKey: ["users", page, limit, q, status, role, sort, order],
    queryFn: ({ signal }) => users.list({ page, limit, search: q, status, role_id: role, sort, order }, signal),
    placeholderData: keepPreviousData,
  });
  // shortcut: the first 100 roles (the API page cap) fill the filter; add search here if a deployment grows past that.
  const canRoles = can(user, "roles", "read");
  const roleList = useQuery({ queryKey: ["roles", "list", "all"], queryFn: () => roles.list({ page: 1, limit: 100 }), enabled: canRoles });
  const fallback = list.isPlaceholderData ? null : emptyPageFallback(page, limit, list.data);
  useGoToPage(fallback, set); // the last rows of the last page went away
  const sortProps = { sort, order, onSort: (s: string, o: SortOrder) => set({ sort: s, order: o, page: 1 }) };
  // Your own account cannot be disabled, so it is never ticked.
  const selection = useSelection(list.data?.items.filter((u) => u.id !== user?.id).map((u) => u.id) ?? []);
  const roleItems: Record<string, string> = { all: t("users.all_roles"), ...Object.fromEntries((roleList.data?.items ?? []).map((r) => [r.id, r.name])) };
  const statusItems: Record<string, string> = { all: t("users.all_statuses"), ...Object.fromEntries(STATUSES.map((s) => [s, t(`status.${s}` as MessageKey)])) };

  return (
    <>
      <PageHeader
        title={t("nav.users")}
        description={t("users.subtitle")}
        action={can(user, "users", "create") && <Button onClick={() => router.push("/admin/users/new")}><UserPlus className="size-4" />{t("users.invite")}</Button>}
      />
      <PageBody>
        <div className="flex flex-wrap gap-2 max-sm:grid max-sm:grid-cols-2">
          <div className="relative min-w-56 flex-1 sm:max-w-sm max-sm:col-span-2 max-sm:min-w-0">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              type="search"
              aria-label={t("common.search")}
              placeholder={t("users.search")}
              className="pl-9"
              value={search}
              onChange={(e) => { setSearch(e.target.value); set({ q: e.target.value.trim(), page: 1 }); }}
            />
          </div>
          <Select value={status || "all"} items={statusItems} onValueChange={(v) => set({ status: v === "all" ? "" : (v as string), page: 1 })}>
            <SelectTrigger className={cn("w-44 max-sm:w-full", !canRoles && "max-sm:col-span-2")} aria-label={t("users.status")}><SelectValue /></SelectTrigger>
            <SelectContent>
              {Object.entries(statusItems).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}
            </SelectContent>
          </Select>
          {canRoles && (
            <Select value={role || "all"} items={roleItems} onValueChange={(v) => set({ role: v === "all" ? "" : (v as string), page: 1 })}>
              <SelectTrigger className="w-48 max-sm:w-full" aria-label={t("users.role")}><SelectValue /></SelectTrigger>
              <SelectContent>
                {Object.entries(roleItems).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}
              </SelectContent>
            </Select>
          )}
          <SortSelect fields={[{ field: "name", label: t("auth.name") }, { field: "email", label: t("auth.email") }, { field: "status", label: t("users.status") }, { field: "created_at", label: t("common.created_at") }]} className="max-sm:col-span-2" {...sortProps} />
          {search.trim() !== "" && !searchTerm(search) && <p className="basis-full text-xs text-muted-foreground max-sm:col-span-2" role="status">{t("common.search_min")}</p>}
        </div>

        {list.isPending ? (
          <ListSkeleton />
        ) : list.isError ? (
          <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : list.data.items.length === 0 ? (
          <EmptyState
            icon={Users}
            title={t(q || status || role ? "common.no_results" : "users.empty")}
            action={(q || status || role) && <Button variant="outline" onClick={() => { setSearch(""); set({ q: "", status: "", role: "", page: 1 }); }}>{t("common.clear_filters")}</Button>}
          />
        ) : (
          <Card className="gap-0 overflow-hidden p-0">
            <Table data-responsive data-compact>
              <TableHeader>
                <TableRow>
                  {canBulk && <SelectAllHead selection={selection} />}
                  <SortHead field="name" label={t("auth.name")} {...sortProps} />
                  <SortHead field="email" label={t("auth.email")} {...sortProps} />
                  <SortHead field="status" label={t("users.status")} className="w-32" {...sortProps} />
                  <SortHead field="created_at" label={t("common.created_at")} className="w-44" {...sortProps} />
                  <TableHead className="w-14"><span className="sr-only">{t("common.actions")}</span></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.data.items.map((u) => (
                  <TableRow key={u.id} className="cursor-pointer" onClick={() => router.push(`/admin/users/${u.id}`)}>
                    {canBulk && <SelectCell selection={selection} id={u.id} label={u.name} disabled={u.id === user?.id} />}
                    <TableCell data-label={t("auth.name")} data-cell="name" className="w-1/2 max-w-0">
                      {/* the keyboard way into the record on phones, where the action button is hidden */}
                      <Link href={`/admin/users/${u.id}`} className="block truncate font-medium hover:underline" title={u.name} onClick={(e) => e.stopPropagation()}>{u.name}</Link>
                    </TableCell>
                    <TableCell data-label={t("auth.email")} data-cell="sub" className="w-1/2 max-w-0 text-muted-foreground lg:text-foreground"><span className="block truncate" title={u.email}>{u.email}</span></TableCell>
                    <TableCell data-label={t("users.status")} data-cell="status"><StatusBadge status={u.status} /></TableCell>
                    <TableCell data-label={t("common.created_at")} data-cell="date" className="text-muted-foreground">{format(u.created_at)}</TableCell>
                    <TableCell data-cell="action" className="text-right">
                      <IconAction label={t(canEdit ? "common.edit" : "common.view")} target={u.name} onClick={() => router.push(`/admin/users/${u.id}`)}>
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
          <Button variant="outline" onClick={() => setBulk("enable")}><UserCheck className="size-4" />{t("users.enable_selected")}</Button>
          <Button variant="destructive" onClick={() => setBulk("disable")}><UserX className="size-4" />{t("users.disable_selected")}</Button>
        </SelectionBar>
      </PageBody>
      <ConfirmDialog
        open={bulk !== null}
        onOpenChange={(o) => !o && setBulk(null)}
        title={t(bulk === "enable" ? "users.enable_selected" : "users.disable_selected")}
        description={t(bulk === "enable" ? "users.enable_selected_desc" : "users.disable_selected_desc")}
        destructive={bulk === "disable"}
        onConfirm={async () => {
          await (bulk === "enable" ? users.enable : users.disable)(selection.ids);
          selection.clear();
          toast.success(t("common.saved"));
          await client.invalidateQueries({ queryKey: ["users"] });
        }}
      />
    </>
  );
}
