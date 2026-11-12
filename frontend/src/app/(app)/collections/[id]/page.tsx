"use client";

import Link from "next/link";
import { useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FileJson, Plus, Search } from "lucide-react";
import { toast } from "sonner";
import { DangerZone, FieldNote, FormSection, FormSections, PageBody, PageHeader, SaveBar, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { SchemaForm } from "@/components/schema-form";
import { ListSkeleton } from "@/components/skeletons";
import { TablePagination } from "@/components/table-pagination";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ApiError } from "@/lib/api/client";
import { collections, records } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useI18n } from "@/lib/i18n/provider";
import { can } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { searchTerm } from "@/lib/utils";
import { collectionName, description as descriptionRule, ready } from "@/lib/validation";

const STARTER = {
  type: "object",
  required: ["title"],
  properties: {
    title: { type: "string", title: "Título" },
    notes: { type: "string", title: "Notas" },
  },
};

const pretty = (v: unknown) => JSON.stringify(v, null, 2);

/** The schema text as an object, or null while it is not a JSON object. */
function parseSchema(text: string): Record<string, unknown> | null {
  try {
    const v: unknown = JSON.parse(text);
    return v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

/** A short line for a record: its first few plain values, in schema order. */
function summarize(data: unknown): string {
  if (!data || typeof data !== "object") return JSON.stringify(data);
  const parts = Object.values(data as Record<string, unknown>).filter((v) => ["string", "number", "boolean"].includes(typeof v) && v !== "");
  return parts.slice(0, 3).join(" · ") || JSON.stringify(data);
}

function CollectionForm({ collection, editable }: { collection: Collection | null; editable: boolean }) {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const original = pretty(collection?.schema ?? STARTER);
  const [name, setName] = useState(collection?.name ?? "");
  const [desc, setDesc] = useState(collection?.description ?? "");
  const [schemaText, setSchemaText] = useState(original);
  const [error, setError] = useState<string | null>(null);
  const [version, setVersion] = useState(collection?.version);
  const schema = useMemo(() => parseSchema(schemaText), [schemaText]);
  const [previewData, setPreviewData] = useState<unknown>({});
  const schemaChanged = schemaText !== original;
  const dirty = collection
    ? name.trim() !== collection.name || desc.trim() !== collection.description || schemaChanged
    : name.trim() !== "";
  useUnsavedGuard(editable && dirty, t("roles.leave_unsaved"));

  const save = useMutation({
    mutationFn: () => {
      const input = { name: name.trim(), description: desc.trim(), schema };
      if (!collection) return collections.create(input);
      return collections.update(collection.id, { name: input.name, description: input.description, schema: schemaChanged ? schema : undefined, version });
    },
    onSuccess: (saved) => {
      setError(null);
      client.setQueryData(["collections", "detail", saved.id], saved);
      void client.invalidateQueries({ queryKey: ["collections"] });
      toast.success(t("common.saved"));
      if (!collection) router.replace(`/collections/${saved.id}`);
      else setVersion(saved.version);
    },
    onError: (err) => {
      // The backend names the broken schema rule; it reads better than a generic failure.
      setError(err instanceof ApiError && err.field === "schema" ? err.message : errorMessage(err, "common.save_failed", t));
      if (collection && err instanceof ApiError && err.code === "CONFLICT") void client.invalidateQueries({ queryKey: ["collections", "detail", collection.id] });
    },
  });

  const discard = () => {
    if (!collection) return router.push("/collections");
    setName(collection.name);
    setDesc(collection.description);
    setSchemaText(original);
    setVersion(collection.version);
    setError(null);
  };

  return (
    <form onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <FormSections>
        <FormSection title={t("collections.details")} description={t("collections.details_desc")}>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="c-name">{t("auth.name")}</Label>
              <Input id="c-name" required autoFocus={!collection} maxLength={120} aria-invalid={!!collectionName(name)} aria-describedby="c-name-note" disabled={!editable} value={name} onChange={(e) => setName(e.target.value)} />
              <FieldNote id="c-name-note" error={collectionName(name) && t(collectionName(name)!)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="c-desc">{t("collections.description")}</Label>
              <textarea id="c-desc" data-slot="textarea" rows={2} maxLength={2000} aria-invalid={!!descriptionRule(desc)} disabled={!editable} value={desc} onChange={(e) => setDesc(e.target.value)} className="w-full rounded-lg border border-input bg-card px-2.5 py-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50" />
            </div>
          </div>
        </FormSection>
        <FormSection title={t("collections.schema")} description={t("collections.schema_desc")} wide>
          <div className="space-y-2">
            <textarea
              id="c-schema"
              data-slot="textarea"
              aria-label={t("collections.schema")}
              aria-invalid={!schema}
              aria-describedby="c-schema-note"
              spellCheck={false}
              rows={Math.min(30, Math.max(10, schemaText.split("\n").length + 1))}
              disabled={!editable}
              value={schemaText}
              onChange={(e) => setSchemaText(e.target.value)}
              className="w-full rounded-lg border border-input bg-card px-3 py-2 font-mono text-xs leading-relaxed outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 aria-invalid:border-destructive"
            />
            <FieldNote id="c-schema-note" error={!schema ? t("collections.schema_invalid") : undefined} hint={collection && schemaChanged ? t("collections.schema_change_hint") : undefined} />
          </div>
        </FormSection>
        <FormSection title={t("collections.preview")} description={t("collections.preview_desc")}>
          {schema && <SchemaForm key={schemaText} schema={schema} data={previewData} onChange={setPreviewData} idPrefix="preview" />}
        </FormSection>
        {editable && (dirty || !collection) && (
          <SaveBar
            footer
            submit
            label={t(collection ? "roles.unsaved" : "collections.new")}
            error={error}
            saving={save.isPending}
            canSave={dirty && !!schema && ready([name, collectionName]) && !descriptionRule(desc)}
            saveLabel={t(collection ? "common.save" : "common.create")}
            discardLabel={t(collection ? "common.discard" : "common.cancel")}
            onDiscard={discard}
          />
        )}
      </FormSections>
    </form>
  );
}

function RecordList({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const { user } = useMe();
  const router = useRouter();
  const format = useFormatDate();
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(25);
  const [search, setSearch] = useState("");
  const q = searchTerm(useDebounced(search.trim()));
  const list = useQuery({
    queryKey: ["records", collection.id, page, limit, q],
    queryFn: ({ signal }) => records.list({ collection_id: collection.id, page, limit, search: q }, signal),
    placeholderData: keepPreviousData,
  });
  const items = list.data?.items ?? [];
  const newHref = `/records/new?collection=${collection.id}`;

  return (
    <FormSections>
      <FormSection title={t("collections.records")} wide>
        <div className="space-y-3">
          <div className="flex flex-wrap gap-2">
            <div className="relative min-w-56 flex-1 sm:max-w-sm">
              <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input type="search" aria-label={t("common.search")} placeholder={t("records.search")} className="pl-9" value={search} onChange={(e) => { setSearch(e.target.value); setPage(1); }} />
            </div>
            {can(user, "records", "create") && <Button className="ml-auto" nativeButton={false} render={<Link href={newHref} />}><Plus className="size-4" />{t("records.new")}</Button>}
          </div>
          {list.isPending ? (
            <ListSkeleton rows={3} />
          ) : list.isError ? (
            <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
          ) : items.length === 0 ? (
            <EmptyState icon={FileJson} title={t(q ? "common.no_results" : "records.empty")} />
          ) : (
            <Card className="gap-0 overflow-hidden p-0">
              <Table data-responsive data-compact>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t("records.record")}</TableHead>
                    <TableHead className="w-44">{t("common.updated_at")}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((r) => (
                    <TableRow key={r.id} className="cursor-pointer" onClick={() => router.push(`/records/${r.id}`)}>
                      <TableCell data-label={t("records.record")} data-cell="name" className="w-full max-w-0">
                        <Link href={`/records/${r.id}`} className="block truncate font-medium hover:underline" onClick={(e) => e.stopPropagation()}>{summarize(r.data)}</Link>
                      </TableCell>
                      <TableCell data-label={t("common.updated_at")} data-cell="date" className="text-muted-foreground">{format(r.updated_at)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
              <TablePagination page={page} pageSize={limit} totalItems={list.data.pagination.total_items} onPageChange={setPage} onPageSizeChange={(n) => { setLimit(n); setPage(1); }} />
            </Card>
          )}
        </div>
      </FormSection>
    </FormSections>
  );
}

export default function CollectionPage() {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const { user, loading } = useMe();
  const { id } = useParams<{ id: string }>();
  const creating = id === "new";
  const detail = useQuery({ queryKey: ["collections", "detail", id], queryFn: () => collections.get(id), enabled: !creating });
  const collection = detail.data;
  const [confirmDelete, setConfirmDelete] = useState(false);
  const editable = can(user, "collections", creating ? "create" : "update");
  useCrumb(creating ? t("collections.new") : collection?.name);
  const format = useFormatDate();
  const subtitle = collection && (collection.description || `${t("common.created_at")} ${format(collection.created_at)}`);

  return (
    <>
      <PageHeader title={creating ? t("collections.new") : (collection?.name ?? t("nav.collections"))} description={subtitle || undefined} />
      <PageBody>
        {loading ? (
          <ListSkeleton rows={3} />
        ) : creating ? (
          <CollectionForm collection={null} editable={editable} />
        ) : detail.isPending ? (
          <ListSkeleton rows={3} />
        ) : detail.error instanceof ApiError && detail.error.code === "NOT_FOUND" ? (
          <EmptyState title={t("error.not_found")} description={t("error.record_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/collections" />}>{t("common.back_to_list")}</Button>} />
        ) : detail.isError || !collection ? (
          <QueryErrorState error={detail.error} onRetry={() => void detail.refetch()} />
        ) : (
          <>
            {can(user, "records", "read") && <RecordList collection={collection} />}
            <CollectionForm key={collection.id} collection={collection} editable={editable} />
          </>
        )}
        {collection && can(user, "collections", "delete") && (
          <DangerZone description={t("collections.delete_hint")} label={t("collections.delete")} onClick={() => setConfirmDelete(true)} />
        )}
      </PageBody>
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={t("collections.delete")}
        description={`${collection?.name ?? ""}. ${t("collections.delete_hint")}`}
        destructive
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          await collections.remove([id]); // the dialog shows a refusal, such as records still inside
          router.push("/collections");
          client.removeQueries({ queryKey: ["collections", "detail", id] });
          void client.invalidateQueries({ queryKey: ["collections"] });
        }}
      />
    </>
  );
}
