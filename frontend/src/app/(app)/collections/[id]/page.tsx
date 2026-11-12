"use client";

import Link from "next/link";
import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Download, FileJson, FileUp, Plus, Search } from "lucide-react";
import { toast } from "sonner";
import { PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { fieldsOf } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { RecordFilters, filterableFields, type FieldFilters } from "@/components/record-filters";
import { RecordImportDialog } from "@/components/record-import-dialog";
import { ListSkeleton } from "@/components/skeletons";
import { TablePagination } from "@/components/table-pagination";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { records } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { searchTerm } from "@/lib/utils";
import { CollectionForm, CollectionPage } from "./_parts";

/** Up to four plain top-level fields, the values the list preview carries, as table columns. */
function columnsOf(schema: Record<string, unknown> | undefined) {
  return fieldsOf(schema ?? null).filter((r) => !r.advanced && r.field.type !== "strings").slice(0, 4);
}

function RecordList({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const router = useRouter();
  const format = useFormatDate();
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(25);
  const [search, setSearch] = useState("");
  const [filters, setFilters] = useState<FieldFilters>({});
  const [importing, setImporting] = useState(false);
  const q = searchTerm(useDebounced(search.trim()));
  const filter = Object.keys(filters).length ? JSON.stringify(filters) : undefined;
  const list = useQuery({
    queryKey: ["records", collection.id, page, limit, q, filter],
    queryFn: ({ signal }) => records.list({ collection_id: collection.id, page, limit, search: q, filter }, signal),
    placeholderData: keepPreviousData,
  });
  const items = list.data?.items ?? [];
  const columns = columnsOf(collection.schema);
  const show = (v: unknown) => (typeof v === "boolean" ? t(v ? "common.yes" : "common.no") : v === undefined || v === null || v === "" ? "—" : String(v));
  const canCreate = roleAllows(collection.role, "editor");
  const newButton = canCreate && <Button nativeButton={false} render={<Link href={`/records/new?collection=${collection.id}`} />}><Plus className="size-4" />{t("records.new")}</Button>;
  const download = (format: "jsonl" | "csv", valid: boolean) =>
    void records.export(collection.id, format, valid).catch((err) => toast.error(errorMessage(err, "common.save_failed", t)));

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap gap-2">
        <div className="relative min-w-56 flex-1 sm:max-w-sm">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input type="search" aria-label={t("common.search")} placeholder={t("records.search")} className="pl-9" value={search} onChange={(e) => { setSearch(e.target.value); setPage(1); }} />
        </div>
        <RecordFilters fields={filterableFields(fieldsOf(collection.schema ?? null))} value={filters} onChange={(f) => { setFilters(f); setPage(1); }} />
        <div className="ml-auto flex flex-wrap gap-2">
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
          {canCreate && <Button variant="outline" onClick={() => setImporting(true)}><FileUp className="size-4" />{t("import.title")}</Button>}
          {newButton}
        </div>
      </div>
      {canCreate && <RecordImportDialog collectionId={collection.id} open={importing} onOpenChange={setImporting} />}
      {list.isPending ? (
        <ListSkeleton rows={3} />
      ) : list.isError ? (
        <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
      ) : items.length === 0 ? (
        <EmptyState icon={FileJson} title={t(q || filter ? "common.no_results" : "records.empty")} />
      ) : (
        <Card className="gap-0 overflow-hidden p-0">
          <Table data-responsive data-compact>
            <TableHeader>
              <TableRow>
                {columns.length ? columns.map((c) => <TableHead key={c.key}>{c.field.name}</TableHead>) : <TableHead>{t("records.record")}</TableHead>}
                <TableHead className="w-44">{t("common.updated_at")}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {items.map((r) => {
                const data = (r.data ?? {}) as Record<string, unknown>;
                const cells = columns.length ? columns.map((c) => ({ key: c.key, label: c.field.name, text: show(data[c.key]) })) : [{ key: "", label: t("records.record"), text: JSON.stringify(r.data) }];
                return (
                  <TableRow key={r.id} className="cursor-pointer" onClick={() => router.push(`/records/${r.id}`)}>
                    {cells.map((c, i) => (
                      <TableCell key={c.key} data-label={c.label} data-cell={i === 0 ? "name" : undefined} className={i === 0 ? "max-w-0 min-w-40" : "max-w-0 text-muted-foreground"}>
                        {i === 0 ? (
                          <Link href={`/records/${r.id}`} className="block truncate font-medium hover:underline" onClick={(e) => e.stopPropagation()}>{c.text}</Link>
                        ) : (
                          <span className="block truncate">{c.text}</span>
                        )}
                      </TableCell>
                    ))}
                    <TableCell data-label={t("common.updated_at")} data-cell="date" className="text-muted-foreground">{format(r.updated_at)}</TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
          <TablePagination page={page} pageSize={limit} totalItems={list.data.pagination.total_items} onPageChange={setPage} onPageSizeChange={(n) => { setLimit(n); setPage(1); }} />
        </Card>
      )}
    </div>
  );
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
  return <CollectionPage section="collections.tab.records">{(collection) => <RecordList collection={collection} />}</CollectionPage>;
}
