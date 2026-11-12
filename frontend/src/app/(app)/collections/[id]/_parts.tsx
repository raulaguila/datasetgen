"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight } from "lucide-react";
import { toast } from "sonner";
import { FieldNote, FormSection, FormSections, PageBody, PageHeader, SaveBar } from "@/components/bits";
import { IconPicker } from "@/components/icon-picker";
import { EmptyState } from "@/components/empty-state";
import { FieldsEditor } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api/client";
import { collections, records } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useI18n } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";
import { can, roleAllows } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { useUIStore } from "@/lib/store/ui";
import { collectionName, description as descriptionRule, ready } from "@/lib/validation";

/** The fields a new collection starts with, named in the user's language. */
const starter = (t: (k: MessageKey) => string) => ({
  type: "object",
  required: ["title"],
  properties: {
    title: { type: "string", title: t("collections.starter_title") },
    notes: { type: "string", title: t("collections.starter_notes") },
  },
});

const pretty = (v: unknown) => JSON.stringify(v, null, 2);

// The form generator is the heaviest code on these pages; load it only where a form shows.
export const SchemaForm = dynamic(() => import("@/components/schema-form").then((m) => m.SchemaForm), { ssr: false, loading: () => <ListSkeleton rows={2} /> });

/** The schema text as an object, or why it is not one (the parser's message names the position). */
function parseSchema(text: string): { schema: Record<string, unknown> | null; error?: string } {
  try {
    const v: unknown = JSON.parse(text);
    return v && typeof v === "object" && !Array.isArray(v) ? { schema: v as Record<string, unknown> } : { schema: null };
  } catch (err) {
    return { schema: null, error: err instanceof Error ? err.message : undefined };
  }
}

/** How many records the collection holds; a one-row page answers the total. */
export function useRecordCount(collectionId: string | undefined, enabled = true) {
  const q = useQuery({
    queryKey: ["records", collectionId, "count"],
    queryFn: ({ signal }) => records.list({ collection_id: collectionId!, page: 1, limit: 1 }, signal),
    enabled: !!collectionId && enabled,
  });
  return q.data?.pagination.total_items;
}

/**
 * Loads the collection in the URL and frames one of its sections: the header with
 * its name, loading and missing states, and the sidebar's current collection.
 */
export function CollectionPage({ section, action, children }: {
  section: MessageKey;
  /** The section's main action, drawn in the header as on every other page. */
  action?: (collection: Collection) => React.ReactNode;
  children: (collection: Collection) => React.ReactNode;
}) {
  const { t } = useI18n();
  const { loading } = useMe();
  const { id } = useParams<{ id: string }>();
  const setCollection = useUIStore((s) => s.setCollection);
  const detail = useQuery({ queryKey: ["collections", "detail", id], queryFn: () => collections.get(id) });
  const collection = detail.data;
  const gone = detail.error instanceof ApiError && detail.error.code === "NOT_FOUND";
  useEffect(() => setCollection(gone ? null : id), [id, gone, setCollection]);
  useCrumb(collection && t(section), collection && { label: collection.name, href: `/collections/${collection.id}` });

  return (
    <>
      <PageHeader title={t(section)} description={collection ? t("collections.section_of").replace("{section}", t(section)).replace("{name}", collection.name) : undefined} action={collection && action?.(collection)} />
      <PageBody>
        {loading || detail.isPending ? (
          <ListSkeleton rows={3} />
        ) : gone ? (
          <EmptyState title={t("error.not_found")} description={t("error.record_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/collections" />}>{t("common.back_to_list")}</Button>} />
        ) : detail.isError || !collection ? (
          <QueryErrorState error={detail.error} onRetry={() => void detail.refetch()} />
        ) : (
          children(collection)
        )}
      </PageBody>
    </>
  );
}

/**
 * Name, description and schema of a collection. A new collection shows both
 * parts; an existing one shows the part its page is for ("details" on Settings,
 * "fields" on Fields), and saving sends only what changed.
 */
export function CollectionForm({ collection, parts }: { collection: Collection | null; parts: ("details" | "fields")[] }) {
  const { t } = useI18n();
  const { user } = useMe();
  const router = useRouter();
  const client = useQueryClient();
  const editable = collection ? roleAllows(collection.role, "owner") : can(user, "collections", "create");
  const recordCount = useRecordCount(collection?.id, parts.includes("fields") && editable);
  const original = pretty(collection?.schema ?? starter(t));
  const [name, setName] = useState(collection?.name ?? "");
  const [desc, setDesc] = useState(collection?.description ?? "");
  const [icon, setIcon] = useState(collection?.icon ?? "");
  const [schemaText, setSchemaText] = useState(original);
  const [error, setError] = useState<string | null>(null);
  const [version, setVersion] = useState(collection?.version);
  const parsed = useMemo(() => parseSchema(schemaText), [schemaText]);
  const schema = parsed.schema;
  // The preview rebuilds once typing pauses, not on every key.
  const previewText = useDebounced(schemaText);
  const preview = useMemo(() => parseSchema(previewText).schema, [previewText]);
  const [previewData, setPreviewData] = useState<unknown>({});
  const schemaChanged = schemaText !== original;
  const dirty = collection
    ? name.trim() !== collection.name || desc.trim() !== collection.description || icon !== collection.icon || schemaChanged
    : name.trim() !== "";
  useUnsavedGuard(editable && dirty, t("roles.leave_unsaved"));

  const save = useMutation({
    mutationFn: () => {
      const input = { name: name.trim(), description: desc.trim(), icon, schema };
      if (!collection) return collections.create(input);
      return collections.update(collection.id, { name: input.name, description: input.description, icon: icon !== collection.icon ? icon : undefined, schema: schemaChanged ? schema : undefined, version });
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
    setIcon(collection.icon);
    setSchemaText(original);
    setVersion(collection.version);
    setError(null);
  };

  // Removing a field, making one required or changing a type can leave stored records out of format.
  const warning = collection && schemaChanged && recordCount ? t("collections.schema_change_hint").replace("{n}", String(recordCount)) : undefined;

  return (
    // noValidate: required fields in the preview are a demo, not part of this save.
    <form noValidate onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <FormSections>
        {!editable && <p role="status" className="rounded-md border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">{t("collections.readonly")}</p>}
        {parts.includes("details") && (
          <FormSection title={t("collections.details")} description={t("collections.details_desc")}>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="c-name">{t("auth.name")}</Label>
                <Input id="c-name" required autoFocus={!collection} maxLength={120} aria-invalid={!!collectionName(name)} aria-describedby="c-name-note" disabled={!editable} value={name} onChange={(e) => setName(e.target.value)} />
                <FieldNote id="c-name-note" error={collectionName(name) && t(collectionName(name)!)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="c-desc">{t("collections.description")}</Label>
                <Textarea id="c-desc" rows={2} maxLength={2000} aria-invalid={!!descriptionRule(desc)} disabled={!editable} value={desc} onChange={(e) => setDesc(e.target.value)} />
              </div>
              <div className="space-y-2">
                <IconPicker value={icon} onChange={setIcon} label={t("collections.icon")} noneLabel={t("collections.icon_none")} disabled={!editable} />
                <p className="text-xs text-muted-foreground">{t("collections.icon_desc")}</p>
              </div>
            </div>
          </FormSection>
        )}
        {parts.includes("fields") && (
          // No side heading here: the list and the preview need the width.
          <section className="space-y-4 border-t pt-8 first:border-t-0 first:pt-0">
            <div className="space-y-1">
              <h2 className="font-heading text-base font-semibold">{t("collections.fields")}</h2>
              <p className="text-sm text-muted-foreground">{t("collections.fields_desc")}</p>
            </div>
            <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)]">
              <div className="min-w-0 space-y-4">
                <FieldsEditor schema={schema} editable={editable} onChange={(next) => setSchemaText(pretty(next))} />
                {warning && <p role="status" className="rounded-md border border-warn/40 bg-warn/10 px-3 py-2 text-sm">{warning}</p>}
                <details className="group rounded-lg border" open={!schema || undefined}>
                  <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-sm font-medium [&::-webkit-details-marker]:hidden">
                    <ChevronRight className="size-4 transition-transform group-open:rotate-90" aria-hidden />
                    {t("collections.json_toggle")}
                  </summary>
                  <div className="space-y-2 border-t p-3">
                    <p className="text-xs text-muted-foreground">{t("collections.schema_desc")}</p>
                    <Textarea
                      id="c-schema"
                      aria-label={t("collections.schema")}
                      aria-invalid={!schema}
                      aria-describedby="c-schema-note"
                      spellCheck={false}
                      rows={Math.min(30, Math.max(10, schemaText.split("\n").length + 1))}
                      disabled={!editable}
                      value={schemaText}
                      onChange={(e) => setSchemaText(e.target.value)}
                      className="px-3 font-mono text-xs leading-relaxed md:text-xs"
                    />
                    <FieldNote id="c-schema-note" error={!schema ? [t("collections.schema_invalid"), parsed.error].filter(Boolean).join(": ") : undefined} />
                  </div>
                </details>
              </div>
              <aside className="min-w-0 space-y-2 lg:sticky lg:top-4 lg:self-start" aria-label={t("collections.preview")}>
                <div>
                  <h3 className="text-sm font-semibold">{t("collections.preview")}</h3>
                  <p className="text-xs text-muted-foreground">{t("collections.preview_desc")}</p>
                </div>
                <div className="rounded-lg border bg-muted/30 p-4">
                  {preview && <SchemaForm key={previewText} schema={preview} data={previewData} onChange={setPreviewData} idPrefix="preview" />}
                </div>
              </aside>
            </div>
          </section>
        )}
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
