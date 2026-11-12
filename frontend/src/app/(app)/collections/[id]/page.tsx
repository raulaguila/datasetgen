"use client";

import Link from "next/link";
import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { keepPreviousData, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, FileJson, FileUp, Plus, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { fieldsOf } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { RecordFilters, filterableFields, type FieldFilters } from "@/components/record-filters";
import { RecordImportDialog } from "@/components/record-import-dialog";
import { ListSkeleton } from "@/components/skeletons";
import { SelectAllHead, SelectCell, SelectionBar, useSelection } from "@/components/selection";
import { SortHead, SortSelect } from "@/components/sort-head";
import { TablePagination, emptyPageFallback } from "@/components/table-pagination";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { records } from "@/lib/api/endpoints";
import type { Collection, SortOrder } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useGoToPage, useListParams } from "@/lib/hooks/use-list-params";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { searchTerm } from "@/lib/utils";
import { deleteWithUndo } from "@/lib/undo-delete";
import { CollectionForm, CollectionPage } from "./_parts";

/** Up to four plain top-level fields, the values the list preview carries, as table columns. */
function columnsOf(schema: Record<string, unknown> | undefined) {
  return fieldsOf(schema ?? null).filter((r) => !r.advanced && r.field.type !== "strings").slice(0, 4);
}

/** The filters kept in the URL as JSON; anything else there reads as none. */
function parseFilters(raw: string): FieldFilters {
  try {
    const v: unknown = JSON.parse(raw);
    return v && typeof v === "object" && !Array.isArray(v) ? (v as FieldFilters) : {};
  } catch {
    return {};
  }
}

function RecordList({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const format = useFormatDate();
  // Kept in the URL like every other list: opening a record and coming back keeps the place.
  const [params, set] = useListParams({ page: 1, limit: 25, q: "", order: "desc", filter: "" });
  const { page, limit } = params;
  const order: SortOrder = params.order === "asc" ? "asc" : "desc";
  const filters = params.filter ? parseFilters(params.filter) : {};
  const filter = Object.keys(filters).length ? JSON.stringify(filters) : undefined;
  const [search, setSearch] = useState(params.q);
  const [hidden, setHidden] = useState<string[]>([]);
  const [importing, setImporting] = useState(false);
  const q = searchTerm(useDebounced(params.q));
  const list = useQuery({
    queryKey: ["records", collection.id, page, limit, q, order, filter],
    queryFn: ({ signal }) => records.list({ collection_id: collection.id, page, limit, search: q, order, filter }, signal),
    placeholderData: keepPreviousData,
  });
  const fallback = list.isPlaceholderData ? null : emptyPageFallback(page, limit, list.data);
  useGoToPage(fallback, set); // the last rows of the last page went away
  const items = list.data?.items.filter((r) => !hidden.includes(r.id)) ?? [];
  const columns = columnsOf(collection.schema);
  const show = (v: unknown) => (typeof v === "boolean" ? t(v ? "common.yes" : "common.no") : v === undefined || v === null || v === "" ? "—" : String(v));
  const canEdit = roleAllows(collection.role, "editor");
  const selection = useSelection(canEdit ? items.map((r) => r.id) : []);
  const filtered = !!(q || filter);
  const sortProps = { sort: "created_at", order, onSort: (_: string, o: SortOrder) => set({ order: o, page: 1 }) };
  const download = (format: "jsonl" | "csv", valid: boolean) =>
    void records.export(collection.id, format, valid).catch((err) => toast.error(errorMessage(err, "common.save_failed", t)));
  const clear = () => { setSearch(""); set({ q: "", filter: "", page: 1 }); };
  const removeSelected = () => {
    const ids = selection.ids;
    selection.clear();
    setHidden((h) => [...h, ...ids]);
    deleteWithUndo({
      message: t("common.deleted_n").replace("{n}", String(ids.length)),
      undoLabel: t("common.undo"),
      run: () => records.remove(ids),
      onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
      onSettled: async () => {
        await Promise.all([
          client.invalidateQueries({ queryKey: ["records", collection.id] }),
          client.invalidateQueries({ queryKey: ["collections"] }),
        ]);
        setHidden((h) => h.filter((id) => !ids.includes(id)));
      },
    });
  };

  return (
    <>
      <div className="flex flex-wrap gap-2 max-sm:grid max-sm:grid-cols-2">
        <div className="relative min-w-56 flex-1 sm:max-w-sm max-sm:col-span-2 max-sm:min-w-0">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input type="search" aria-label={t("common.search")} placeholder={t("records.search")} className="pl-9" value={search} onChange={(e) => { setSearch(e.target.value); set({ q: e.target.value.trim(), page: 1 }); }} />
        </div>
        <RecordFilters fields={filterableFields(fieldsOf(collection.schema ?? null))} value={filters} onChange={(f) => set({ filter: Object.keys(f).length ? JSON.stringify(f) : "", page: 1 })} />
        <SortSelect fields={[{ field: "created_at", label: t("common.created_at") }]} {...sortProps} className="max-sm:col-span-2" />
        <div className="ml-auto flex flex-wrap gap-2 max-sm:contents">
          <DropdownMenu>
            <DropdownMenuTrigger render={<Button variant="outline" />}><Download className="size-4" />{t("export.title")}</DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-56">
              <DropdownMenuItem onClick={() => download("jsonl", false)}>{t("export.jsonl")}</DropdownMenuItem>
              <DropdownMenuItem onClick={() => download("csv", false)}>{t("export.csv")}</DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => download("jsonl", true)}>{t("export.jsonl_valid")}</DropdownMenuItem>
              <DropdownMenuItem onClick={() => download("csv", true)}>{t("export.csv_valid")}</DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          {canEdit && <Button variant="outline" onClick={() => setImporting(true)}><FileUp className="size-4" />{t("import.title")}</Button>}
        </div>
        {search.trim() !== "" && !searchTerm(search) && <p className="basis-full text-xs text-muted-foreground max-sm:col-span-2" role="status">{t("common.search_min")}</p>}
      </div>
      {canEdit && <RecordImportDialog collectionId={collection.id} open={importing} onOpenChange={setImporting} />}
      {list.isPending ? (
        <ListSkeleton rows={3} />
      ) : list.isError ? (
        <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState
          icon={FileJson}
          title={t(filtered ? "common.no_results" : "records.empty")}
          action={filtered && <Button variant="outline" onClick={clear}>{t("common.clear_filters")}</Button>}
        />
      ) : (
        <Card className="gap-0 overflow-hidden p-0">
          <Table data-responsive data-compact>
            <TableHeader>
              <TableRow>
                {canEdit && <SelectAllHead selection={selection} />}
                {columns.length ? columns.map((c) => <TableHead key={c.key}>{c.field.name}</TableHead>) : <TableHead>{t("records.record")}</TableHead>}
                <SortHead field="created_at" label={t("common.created_at")} className="w-44" {...sortProps} />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((r) => {
                const data = (r.data ?? {}) as Record<string, unknown>;
                const cells = columns.length ? columns.map((c) => ({ key: c.key, label: c.field.name, text: show(data[c.key]) })) : [{ key: "", label: t("records.record"), text: JSON.stringify(r.data) }];
                return (
                  <TableRow key={r.id} className="cursor-pointer" onClick={() => router.push(`/records/${r.id}`)}>
                    {canEdit && <SelectCell selection={selection} id={r.id} label={cells[0].text} />}
                    {cells.map((c, i) => (
                      <TableCell key={c.key} data-label={c.label} data-cell={i === 0 ? "name" : undefined} className={i === 0 ? "max-w-0 min-w-40" : "max-w-0 text-muted-foreground"}>
                        {i === 0 ? (
                          <Link href={`/records/${r.id}`} className="block truncate font-medium hover:underline" onClick={(e) => e.stopPropagation()}>{c.text}</Link>
                        ) : (
                          <span className="block truncate">{c.text}</span>
                        )}
                      </TableCell>
                    ))}
                    <TableCell data-label={t("common.created_at")} data-cell="date" className="text-muted-foreground">{format(r.created_at)}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
          <TablePagination page={page} pageSize={limit} totalItems={list.data.pagination.total_items} onPageChange={(n) => set({ page: n })} onPageSizeChange={(n) => set({ limit: n, page: 1 })} />
        </Card>
      )}
      <SelectionBar selection={selection}>
        <Button variant="destructive" onClick={removeSelected}><Trash2 className="size-4" />{t("common.delete_selected")}</Button>
      </SelectionBar>
    </>
  );
}

function NewRecordButton({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  return <Button nativeButton={false} render={<Link href={`/records/new?collection=${collection.id}`} />}><Plus className="size-4" />{t("records.new")}</Button>;
}

function NewCollection() {
  const { t } = useI18n();
  const { loading } = useMe();
  useCrumb(t("collections.new"));
  return (
    <>
      <PageHeader title={t("collections.new")} />
      <PageBody>{loading ? <ListSkeleton rows={3} /> : <CollectionForm collection={null} parts={["details", "fields"]} />}</PageBody>
    </>
  );
}

export default function CollectionRecordsPage() {
  const { id } = useParams<{ id: string }>();
  if (id === "new") return <NewCollection />;
  return (
    <CollectionPage section="collections.tab.records" action={(c) => roleAllows(c.role, "editor") && <NewRecordButton collection={c} />}>
      {(collection) => <RecordList collection={collection} />}
    </CollectionPage>
  );
}
