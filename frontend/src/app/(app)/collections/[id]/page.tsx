"use client";

import Link from "next/link";
import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { keepPreviousData, useQueries, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Download, FileJson, FileUp, Paperclip, PencilLine, Plus, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { BulkEditDialog } from "@/components/bulk-edit-dialog";
import { fieldsOf, fileRefs, recordTitle, REF, type FieldRow } from "@/components/field-builder";
import { SavedViews } from "@/components/saved-views";
import { ColumnPicker, EditableCell, editsInline, pickColumns, useSaveField } from "@/components/record-grid";
import { RecordGroups, groupableOf } from "@/components/record-groups";
import { RecordRules, andWhere, legacyRules, parseWhere, whereText as toWhereText, type Where } from "@/components/record-rules";
import { BoardView, CalendarView, GalleryView, layoutFields, type Layout } from "@/components/record-views";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { QueryErrorState } from "@/components/query-error-state";
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
import { collections, records, tools } from "@/lib/api/endpoints";
import type { Collection, DataRecord, SortOrder } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useGoToPage, useListParams } from "@/lib/hooks/use-list-params";
import { useQuality } from "@/lib/hooks/use-quality";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { searchTerm } from "@/lib/utils";
import { deleteWithUndo } from "@/lib/undo-delete";
import { CollectionForm, CollectionPage } from "./_parts";

/**
 * The titles of the records reference columns point at, one lookup per column
 * for the page's ids. Absent until loaded; the id stands in meanwhile.
 */
function useRefLabels(columns: FieldRow[], items: DataRecord[], schema: Record<string, unknown> | undefined) {
  const props = (schema?.properties ?? {}) as Record<string, Record<string, unknown>>;
  const refs = columns.filter((c) => c.field.type === "reference" || c.field.type === "references").map((c) => {
    const ids = [...new Set(items.flatMap((r) => [(r.data as Record<string, unknown> | null)?.[c.key]].flat()).filter((v): v is string => typeof v === "string" && /^[0-9a-f-]{36}$/i.test(v)))];
    return { target: String(props[c.key]?.[REF] ?? ""), ids };
  }).filter((r) => r.target && r.ids.length);
  const targets = [...new Set(refs.map((r) => r.target))];
  const metas = useQueries({ queries: targets.map((target) => ({ queryKey: ["collections", "detail", target], queryFn: () => collections.get(target), retry: false })) });
  const found = useQueries({
    queries: refs.map((r) => ({
      queryKey: ["records", r.target, "ids", r.ids.join(",")],
      queryFn: ({ signal }: { signal: AbortSignal }) => records.list({ collection_id: r.target, page: 1, limit: 100, ids: r.ids.join(",") }, signal),
      retry: false,
    })),
  });
  const labels = new Map<string, string>();
  refs.forEach((r, i) => {
    const schemaOf = metas[targets.indexOf(r.target)]?.data?.schema ?? null;
    for (const rec of found[i]?.data?.items ?? []) labels.set(rec.id, recordTitle(schemaOf, rec.data) ?? rec.id.slice(0, 8));
  });
  return labels;
}

/** The "no grouping" choice; a field key is never this. */
const NO_GROUP = "\u0000none";

function RecordList({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const format = useFormatDate();
  // Kept in the URL like every other list: opening a record and coming back keeps the place.
  const [params, set] = useListParams({ page: 1, limit: 25, q: "", sort: "updated_at", order: "desc", filter: "", where: "", group: "", cols: "", layout: "table", by: "" });
  const { page, limit } = params;
  const rows = fieldsOf(collection.schema ?? null);
  const columns = pickColumns(rows, params.cols);
  const where = andWhere(parseWhere(params.where), params.filter ? legacyRules(params.filter) : []);
  const whereText = toWhereText(where) || undefined;
  const setWhere = (w: Where) => set({ where: toWhereText(w), filter: "", page: 1 });
  const groupRow = groupableOf(rows).find((r) => r.key === params.group);
  // A layout needs a field to arrange by; without one the table shows.
  const layoutOptions = (Object.keys(layoutFields) as Exclude<Layout, "table">[]).filter((l) => rows.some(layoutFields[l]));
  const layout: Layout = layoutOptions.includes(params.layout as Exclude<Layout, "table">) ? (params.layout as Layout) : "table";
  const byOptions = layout === "table" ? [] : rows.filter(layoutFields[layout]);
  const by = byOptions.find((r) => r.key === params.by) ?? byOptions[0];
  const saveField = useSaveField(collection.id);
  // Any shown column sorts, except files (their stored value is an object).
  const sortable = columns.filter((c) => c.field.type !== "file" && c.field.type !== "files");
  const sort = params.sort === "created_at" || sortable.some((c) => params.sort === `data.${c.key}`) ? params.sort : "updated_at";
  const order: SortOrder = params.order === "asc" ? "asc" : "desc";
  const [search, setSearch] = useState(params.q);
  const [hidden, setHidden] = useState<string[]>([]);
  const [importing, setImporting] = useState(false);
  const [bulk, setBulk] = useState(false);
  const q = searchTerm(useDebounced(params.q));
  const list = useQuery({
    queryKey: ["records", collection.id, page, limit, q, sort, order, whereText],
    queryFn: ({ signal }) => records.list({ collection_id: collection.id, page, limit, search: q, sort, order, where: whereText }, signal),
    enabled: layout === "table",
    placeholderData: keepPreviousData,
  });
  const fallback = list.isPlaceholderData ? null : emptyPageFallback(page, limit, list.data);
  useGoToPage(fallback, set); // the last rows of the last page went away
  const items = list.data?.items.filter((r) => !hidden.includes(r.id)) ?? [];
  const refLabels = useRefLabels(columns, items, collection.schema);
  const show = (v: unknown, c?: FieldRow) => {
    if (v === undefined || v === null || v === "") return "—";
    if (typeof v === "boolean") return t(v ? "common.yes" : "common.no");
    if (c?.field.type === "reference" && typeof v === "string") return refLabels.get(v) ?? v.slice(0, 8);
    if (c?.field.type === "file" || c?.field.type === "files") {
      const files = fileRefs(v);
      return files.length ? `${files[0].name}${files.length > 1 ? ` +${files.length - 1}` : ""}` : "—";
    }
    if (Array.isArray(v)) return v.length ? v.map((x) => (c?.field.type === "references" ? refLabels.get(String(x)) ?? String(x).slice(0, 8) : String(x))).join(", ") : "—";
    return typeof v === "object" ? JSON.stringify(v) : String(v);
  };
  const canEdit = roleAllows(collection.role, "editor");
  // Out-of-format records come from the latest quality check (editors only), as long as it is current.
  const { q: quality, stale } = useQuality(collection, canEdit);
  const report = quality.data?.status === "done" && !stale ? quality.data : null;
  // shortcut: the check lists the first 100 invalid records; past that some rows go unmarked, fine until collections hold many broken records.
  const invalid = new Map((report?.invalid ?? []).map((r) => [r.id, r.updated_at]));
  // A record edited since the check may be fixed already: it stays unmarked.
  const isInvalid = (id: string, updatedAt: string) => invalid.has(id) && Date.parse(updatedAt) <= Date.parse(invalid.get(id)!);
  const selection = useSelection(canEdit ? items.map((r) => r.id) : []);
  const filtered = !!(q || whereText);
  const scope = { collection_id: collection.id, search: q || undefined, where: whereText };
  // Picking a group filters the list to its value (or to empty).
  const pickGroup = (key: unknown) => {
    if (!groupRow) return;
    const rule = key === null || key === undefined || key === "" ? { field: groupRow.key, op: "empty" as const } : { field: groupRow.key, op: "eq" as const, value: key };
    setWhere(andWhere(where.any ? where : { ...where, rules: where.rules.filter((r) => r.field !== groupRow.key) }, [rule]));
    set({ group: "" });
  };
  const sortProps = { sort, order, onSort: (f: string, o: SortOrder) => set({ sort: f, order: o, page: 1 }) };
  const download = (format: "jsonl" | "csv", valid: boolean) =>
    void records.export(collection.id, format, valid).catch((err) => toast.error(errorMessage(err, "common.save_failed", t)));
  const clear = () => { setSearch(""); set({ q: "", filter: "", where: "", page: 1 }); };
  const applyView = (query: string) => {
    window.history.replaceState(null, "", query ? `?${query}` : window.location.pathname);
    setSearch(new URLSearchParams(query).get("q") ?? "");
  };
  const removeSelected = () => {
    const ids = selection.ids;
    selection.clear();
    setHidden((h) => [...h, ...ids]);
    deleteWithUndo({
      message: t("records.trashed_n").replace("{n}", String(ids.length)),
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
        <RecordRules fields={rows} value={where} onChange={setWhere} />
        {layout === "table" && <ColumnPicker fields={rows} value={columns} onChange={(keys) => set({ cols: keys.join(",") })} />}
        {groupableOf(rows).length > 0 && (
          <Select value={groupRow?.key ?? NO_GROUP} items={{ [NO_GROUP]: t("groups.none"), ...Object.fromEntries(groupableOf(rows).map((r) => [r.key, `${t("groups.by")}: ${r.field.name}`])) }} onValueChange={(v) => set({ group: v === NO_GROUP ? "" : String(v) })}>
            <SelectTrigger className="w-44 max-sm:w-full" aria-label={t("groups.by")}><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_GROUP}>{t("groups.none")}</SelectItem>
              {groupableOf(rows).map((r) => <SelectItem key={r.key} value={r.key}>{`${t("groups.by")}: ${r.field.name}`}</SelectItem>)}
            </SelectContent>
          </Select>
        )}
        {layoutOptions.length > 0 && (
          <Select value={layout} items={{ table: t("layout.table"), ...Object.fromEntries(layoutOptions.map((l) => [l, t(`layout.${l}`)])) }} onValueChange={(v) => set({ layout: v === "table" ? "" : String(v), by: "" })}>
            <SelectTrigger className="w-40 max-sm:w-full" aria-label={t("layout.title")}><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="table">{t("layout.table")}</SelectItem>
              {layoutOptions.map((l) => <SelectItem key={l} value={l}>{t(`layout.${l}`)}</SelectItem>)}
            </SelectContent>
          </Select>
        )}
        {byOptions.length > 1 && by && (
          <Select value={by.key} items={Object.fromEntries(byOptions.map((r) => [r.key, r.field.name]))} onValueChange={(v) => set({ by: String(v) })}>
            <SelectTrigger className="w-44 max-sm:w-full" aria-label={t("layout.by")}><SelectValue /></SelectTrigger>
            <SelectContent>{byOptions.map((r) => <SelectItem key={r.key} value={r.key}>{r.field.name}</SelectItem>)}</SelectContent>
          </Select>
        )}
        <SortSelect fields={[{ field: "updated_at", label: t("common.updated_at") }, { field: "created_at", label: t("common.created_at") }, ...sortable.map((c) => ({ field: `data.${c.key}`, label: c.field.name }))]} {...sortProps} className="max-sm:col-span-2" />
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
          <SavedViews collectionId={collection.id} canShare={canEdit} onApply={applyView} />
          {canEdit && <Button variant="outline" onClick={() => setImporting(true)}><FileUp className="size-4" />{t("import.title")}</Button>}
        </div>
        {search.trim() !== "" && !searchTerm(search) && <p className="basis-full text-xs text-muted-foreground max-sm:col-span-2" role="status">{t("common.search_min")}</p>}
      </div>
      {canEdit && <RecordImportDialog collectionId={collection.id} schema={collection.schema} open={importing} onOpenChange={setImporting} />}
      {groupRow && <RecordGroups selection={scope} group={groupRow} fields={rows} label={(k) => show(k, groupRow)} onPick={pickGroup} />}
      {layout !== "table" && by ? (
        layout === "board" ? <BoardView collection={collection} by={by} search={scope.search} where={where} show={show} extra={columns.filter((c) => c.key !== by.key).slice(0, 3)} canEdit={canEdit} save={saveField} />
        : layout === "calendar" ? <CalendarView collection={collection} by={by} search={scope.search} where={where} show={show} extra={[]} />
        : <GalleryView collection={collection} by={by} search={scope.search} where={where} show={show} extra={columns.filter((c) => c.key !== by.key).slice(0, 2)} />
      ) : list.isPending ? (
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
                {/* The first column, the record's name, gets the room; the others what is left. */}
                {columns.length ? columns.map((c, i) => c.field.type === "file" || c.field.type === "files"
                  ? <TableHead key={c.key} className={i === 0 ? "w-[40%]" : undefined}>{c.field.name}</TableHead>
                  : <SortHead key={c.key} field={`data.${c.key}`} label={c.field.name} className={i === 0 ? "w-[40%]" : undefined} {...sortProps} />) : <TableHead>{t("records.record")}</TableHead>}
                <SortHead field="updated_at" label={t("common.updated_at")} className="w-44" {...sortProps} />
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((r) => {
                const data = (r.data ?? {}) as Record<string, unknown>;
                const cells = columns.length ? columns.map((c) => ({ key: c.key, row: c, label: c.field.name, text: show(data[c.key], c), file: fileRefs(data[c.key])[0] })) : [{ key: "", row: undefined, label: t("records.record"), text: JSON.stringify(r.data), file: undefined }];
                return (
                  <TableRow key={r.id} className="cursor-pointer hover:bg-muted/60" onClick={() => router.push(`/records/${r.id}`)}>
                    {canEdit && <SelectCell selection={selection} id={r.id} label={cells[0].text} />}
                    {cells.map((c, i) => (
                      <TableCell key={c.key} data-label={c.label} data-cell={i === 0 ? "name" : undefined} className={i === 0 ? "max-w-0 min-w-40" : "max-w-0 text-muted-foreground"}>
                        {i === 0 ? (
                          <span className="flex min-w-0 items-center gap-1.5">
                            {isInvalid(r.id, r.updated_at) && (
                              <AlertTriangle className="size-4 shrink-0 text-warn" role="img" aria-label={t("overview.invalid")}><title>{t("records.invalid_hint")}</title></AlertTriangle>
                            )}
                            <Link href={`/records/${r.id}`} className="block truncate font-medium hover:underline" onClick={(e) => e.stopPropagation()}>{c.text}</Link>
                          </span>
                        ) : canEdit && c.row && editsInline(c.row) ? (
                          <EditableCell row={c.row} value={data[c.key]} text={c.text} recordId={r.id} save={saveField} />
                        ) : c.file?.id ? (
                          <a href={tools.fileUrl(c.file.id)} target="_blank" rel="noopener" className="flex min-w-0 items-center gap-1 hover:underline" onClick={(e) => e.stopPropagation()}>
                            <Paperclip className="size-3.5 shrink-0" /><span className="truncate">{c.text}</span>
                          </a>
                        ) : (
                          <span className="block truncate">{c.text}</span>
                        )}
                      </TableCell>
                    ))}
                    <TableCell data-label={t("common.updated_at")} data-cell="date" className="whitespace-nowrap text-muted-foreground">{format(r.updated_at)}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
          {/* One page of the smallest size needs no paging controls, only the total. */}
          {page === 1 && list.data.pagination.total_items <= Math.min(limit, 10) ? (
            <p className="border-t px-4 py-2.5 text-xs text-muted-foreground">{t(list.data.pagination.total_items === 1 ? "nav.record_one" : "nav.record_many").replace("{n}", String(list.data.pagination.total_items))}</p>
          ) : (
            <TablePagination page={page} pageSize={limit} totalItems={list.data.pagination.total_items} onPageChange={(n) => set({ page: n })} onPageSizeChange={(n) => set({ limit: n, page: 1 })} />
          )}
        </Card>
      )}
      <SelectionBar selection={selection}>
        <Button variant="outline" onClick={() => setBulk(true)}><PencilLine className="size-4" />{t("bulk.title")}</Button>
        <Button variant="destructive" onClick={removeSelected}><Trash2 className="size-4" />{t("common.delete_selected")}</Button>
      </SelectionBar>
      {canEdit && <BulkEditDialog collection={collection} ids={selection.ids} open={bulk} onOpenChange={setBulk} onDone={selection.clear} />}
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
  const { t } = useI18n();
  if (id === "new") return <NewCollection />;
  const count = (_: Collection, n?: number) => (n === undefined ? undefined : t(n === 1 ? "nav.record_one" : "nav.record_many").replace("{n}", String(n)));
  return (
    <CollectionPage section="collections.tab.records" countRecords description={count} action={(c) => roleAllows(c.role, "editor") && <NewRecordButton collection={c} />}>
      {(collection) => <RecordList collection={collection} />}
    </CollectionPage>
  );
}
