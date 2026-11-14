"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, FileUp, History, RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { FieldNote, FormSection, FormSections, PageBody, PageHeader, SaveBar, useFormatDate } from "@/components/bits";
import { CollectionAvatar } from "@/components/collection-avatar";
import { IconPicker } from "@/components/icon-picker";
import { EmptyState } from "@/components/empty-state";
import { FieldsEditor, schemaChanges, type SchemaChange } from "@/components/field-builder";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api/client";
import { collections, records, tools } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import type { MessageKey } from "@/lib/i18n/messages";
import { can, roleAllows } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { useUIStore } from "@/lib/store/ui";
import { cn } from "@/lib/utils";
import { TEMPLATE_IDS, templateFields, type TemplateId } from "@/lib/collection-templates";
import { schemaFromFile, templateSchema } from "@/lib/infer-schema";
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
export function CollectionPage({ section, description, countRecords = false, action, children }: {
  section: MessageKey;
  /** What the section is for, or a line built from the collection and its record count; by default "<section> of <collection>". */
  description?: MessageKey | ((collection: Collection, recordCount?: number) => string | undefined);
  /** Loads the record count for a `description` that shows it. */
  countRecords?: boolean;
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
  const recordCount = useRecordCount(collection?.id, countRecords);
  const subtitle = !collection ? undefined
    : typeof description === "function" ? description(collection, recordCount)
    : description ? t(description)
    : t("collections.section_of").replace("{section}", t(section)).replace("{name}", collection.name);

  return (
    <>
      <PageHeader title={t(section)} description={subtitle} action={collection && action?.(collection)} />
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
 * The schemas the collection had before each change, newest first; one loads
 * into the editor, and saving it is a change like any other.
 */
function SchemaHistory({ collection, onLoad }: { collection: Collection; onLoad: (schema: Record<string, unknown>) => void }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const [open, setOpen] = useState(false);
  const list = useQuery({ queryKey: ["schema-versions", collection.id, collection.version], queryFn: ({ signal }) => tools.schemaVersions(collection.id, signal), enabled: open });
  const items = list.data?.items ?? [];
  return (
    <details className="group rounded-lg border" onToggle={(e) => setOpen(e.currentTarget.open)}>
      <summary className="flex cursor-pointer list-none items-center gap-2 px-3 py-2 text-sm font-medium [&::-webkit-details-marker]:hidden">
        <ChevronRight className="size-4 transition-transform group-open:rotate-90" aria-hidden />
        <History className="size-4" aria-hidden />{t("field_history.title")}
      </summary>
      <div className="space-y-2 border-t p-3">
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("field_history.empty")}</p>
        ) : (
          <ul className="divide-y">
            {items.map((v, i) => {
              // What the change after this version did: compared with the next newer schema.
              const changes = schemaChanges(v.schema, i === 0 ? collection.schema ?? null : items[i - 1].schema);
              return (
                <li key={v.version} className="flex flex-wrap items-start gap-x-3 gap-y-1 py-2">
                  <div className="min-w-48 flex-1 text-sm">
                    <p className="font-medium">{t("field_history.version").replace("{n}", String(v.version))}<span className="font-normal text-muted-foreground"> · {format(v.saved_at)}{v.saved_by_name && ` · ${v.saved_by_name}`}</span></p>
                    <p className="text-xs text-muted-foreground">
                      {changes.length ? `${t("field_history.then")} ${changes.map((c) => t(`collections.change_${c.kind}`).replace("{name}", c.name)).join(" · ")}` : t("field_history.no_field_changes")}
                    </p>
                  </div>
                  <Button type="button" size="sm" variant="outline" onClick={() => onLoad(v.schema)}><RotateCcw className="size-4" />{t("field_history.load")}</Button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </details>
  );
}

/**
 * Name, description and schema of a collection. A new collection shows both
 * parts; an existing one shows the part its page is for ("details" on Settings,
 * "fields" on Fields), and saving sends only what changed.
 */
export function CollectionForm({ collection, parts }: { collection: Collection | null; parts: ("details" | "fields")[] }) {
  const { t, locale } = useI18n();
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
  // A new collection: the name's error shows once Create was pressed without one.
  const [tried, setTried] = useState(false);
  const [template, setTemplate] = useState<TemplateId | "file">("basic");
  const [fileNote, setFileNote] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const nameError = collectionName(name) ?? (tried && !name.trim() ? "collections.name_needed" : null);
  const pickTemplate = (id: TemplateId) => {
    setTemplate(id);
    setFileNote(null);
    setSchemaText(pretty(id === "basic" ? starter(t) : templateSchema(templateFields(locale, id))));
  };
  const pickFile = async (file: File) => {
    const next = schemaFromFile(file.name, await file.text());
    if (!next) return setFileNote(t("collections.template_file_empty"));
    setTemplate("file");
    setSchemaText(pretty(next));
    setFileNote(t("collections.template_file_loaded").replace("{n}", String(Object.keys(next.properties as object).length)).replace("{file}", file.name));
  };
  const parsed = useMemo(() => parseSchema(schemaText), [schemaText]);
  const schema = parsed.schema;
  // The preview rebuilds once typing pauses, not on every key.
  const previewText = useDebounced(schemaText);
  const preview = useMemo(() => parseSchema(previewText).schema, [previewText]);
  const [previewData, setPreviewData] = useState<unknown>({});
  const schemaChanged = schemaText !== original;
  const changes = useMemo(() => (collection ? schemaChanges(collection.schema ?? null, schema) : []), [collection, schema]);
  // Changes that can leave stored records out of format: confirmed first, then Quality is a click away.
  const risky = !!recordCount && changes.some((c) => c.risky);
  const [confirming, setConfirming] = useState(false);
  const dirty = collection
    ? name.trim() !== collection.name || desc.trim() !== collection.description || icon !== collection.icon || schemaChanged
    : name.trim() !== "";
  useUnsavedGuard(editable && dirty, t("roles.leave_unsaved"));

  // The variable says whether the save was risky, for the toast that follows.
  const save = useMutation<Collection, unknown, boolean>({
    mutationFn: () => {
      const input = { name: name.trim(), description: desc.trim(), icon, schema };
      if (!collection) return collections.create(input);
      return collections.update(collection.id, { name: input.name, description: input.description, icon: icon !== collection.icon ? icon : undefined, schema: schemaChanged ? schema : undefined, version });
    },
    onSuccess: (saved, wasRisky) => {
      setError(null);
      client.setQueryData(["collections", "detail", saved.id], saved);
      void client.invalidateQueries({ queryKey: ["collections"] });
      toast.success(t("common.saved"), wasRisky ? { action: { label: t("collections.check_quality"), onClick: () => router.push(`/collections/${saved.id}/quality`) } } : undefined);
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

  const changeText = (c: SchemaChange) => t(`collections.change_${c.kind}`).replace("{name}", c.name);

  return (
    // noValidate: required fields in the preview are a demo, not part of this save.
    <form noValidate onSubmit={(e) => {
      e.preventDefault();
      if (!collection && !ready([name, collectionName])) {
        setTried(true);
        document.getElementById("c-name")?.focus();
        return;
      }
      if (risky) setConfirming(true);
      else save.mutate(false);
    }}
      // Ctrl/⌘+Enter saves from any field.
      onKeyDown={(e) => { if (editable && (dirty || !collection) && e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); e.currentTarget.requestSubmit(); } }}>
      <FormSections>
        {!editable && <p role="status" className="rounded-md border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">{t("collections.readonly")}</p>}
        {parts.includes("details") && (
          <FormSection title={t("collections.details")} description={t("collections.details_desc")}>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="c-name">{t("auth.name")}</Label>
                <Input id="c-name" required autoFocus={!collection} maxLength={120} aria-invalid={!!nameError} aria-describedby="c-name-note" disabled={!editable} value={name} onChange={(e) => setName(e.target.value)} />
                <FieldNote id="c-name-note" error={nameError && t(nameError)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="c-desc">{t("collections.description")}</Label>
                <Textarea id="c-desc" rows={2} maxLength={2000} aria-invalid={!!descriptionRule(desc)} disabled={!editable} value={desc} onChange={(e) => setDesc(e.target.value)} />
              </div>
              <div className="space-y-2">
                <IconPicker value={icon} onChange={setIcon} label={t("collections.icon")} noneLabel={t("collections.icon_none")} disabled={!editable} />
                <p className="flex items-center gap-2 text-xs text-muted-foreground">
                  {t("collections.icon_desc")}
                  <CollectionAvatar collection={{ id: collection?.id ?? "new", name: name.trim() || "?", icon }} className="size-6 rounded-md text-[10px] font-semibold" />
                </p>
              </div>
            </div>
          </FormSection>
        )}
        {parts.includes("fields") && (
          // No side heading here: the list and the preview need the width.
          <section className="space-y-4 border-t pt-8 first:border-t-0 first:pt-0">
            {/* The Fields page says this in its header; the new-collection page needs it here. The save bar says what is unsaved. */}
            {parts.includes("details") && (
              <div className="space-y-1">
                <h2 className="font-heading text-base font-semibold">{t("collections.fields")}</h2>
                <p className="text-sm text-muted-foreground">{t("collections.fields_desc")}</p>
              </div>
            )}
            {/* The list stops at a readable width on wide screens; the preview takes the rest. */}
            <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,24rem)] 2xl:grid-cols-[minmax(0,48rem)_minmax(24rem,1fr)]">
              <div className="min-w-0 space-y-4">
                {!collection && editable && (
                  // A new collection starts from a ready-made set of fields, or from a data file's columns.
                  <div className="space-y-2">
                    <div role="radiogroup" aria-label={t("collections.start_with")} className="flex flex-wrap items-center gap-2">
                      <span className="text-sm text-muted-foreground">{t("collections.start_with")}</span>
                      {TEMPLATE_IDS.map((id) => (
                        <Button key={id} type="button" size="sm" variant={template === id ? "secondary" : "outline"} role="radio" aria-checked={template === id} onClick={() => pickTemplate(id)}>
                          {t(`collections.template.${id}`)}
                        </Button>
                      ))}
                      <Button type="button" size="sm" variant={template === "file" ? "secondary" : "outline"} role="radio" aria-checked={template === "file"} onClick={() => fileInput.current?.click()}>
                        <FileUp className="size-4" />{t("collections.template.file")}
                      </Button>
                      <input ref={fileInput} type="file" accept=".json,.jsonl,.ndjson,.csv,application/json,text/csv" className="hidden" aria-label={t("collections.template.file")}
                        onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ""; if (f) void pickFile(f); }} />
                    </div>
                    <p className="text-xs text-muted-foreground" role="status">{fileNote ?? t("collections.template_file_hint")}</p>
                  </div>
                )}
                <FieldsEditor
                  schema={schema}
                  original={collection?.schema ?? null}
                  editable={editable}
                  keysLocked={!!collection && recordCount !== 0}
                  onChange={(next) => setSchemaText(pretty(next))}
                />
                {changes.length > 0 && (
                  <div role="status" className={cn("space-y-1.5 rounded-md border px-3 py-2 text-sm", risky ? "border-warn/40 bg-warn/10" : "bg-muted/40")}>
                    <p className="font-medium">{t("collections.changes_title")}</p>
                    <ul className="list-inside list-disc space-y-0.5">
                      {changes.map((c, i) => <li key={i}>{changeText(c)}</li>)}
                    </ul>
                    {risky && <p>{plural(t("collections.schema_change_hint"), recordCount)}</p>}
                  </div>
                )}
                {collection && editable && (
                  <SchemaHistory collection={collection} onLoad={(old) => { setSchemaText(pretty(old)); toast.info(t("field_history.loaded")); }} />
                )}
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
            status={
              <span className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                {collection ? t("roles.unsaved") : t(name.trim() ? "collections.ready_to_create" : "collections.name_needed")}
                <span className="hidden text-xs font-normal text-muted-foreground sm:inline"><kbd className="font-mono">Ctrl</kbd>+<kbd className="font-mono">Enter</kbd> {t("records.shortcut_save")}</span>
              </span>
            }
            error={error}
            saving={save.isPending}
            // A new collection's Create stays live: pressing it without a name says what is missing.
            canSave={!!schema && !descriptionRule(desc) && (collection ? dirty && ready([name, collectionName]) : !collectionName(name))}
            saveLabel={t(collection ? "common.save" : "common.create")}
            discardLabel={t(collection ? "common.discard" : "common.cancel")}
            onDiscard={discard}
          />
        )}
      </FormSections>
      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t("collections.save_fields_title")}
        description={`${changes.filter((c) => c.risky).map(changeText).join(" · ")}. ${plural(t("collections.save_fields_desc"), recordCount ?? 0)}`}
        confirmLabel={t("common.save")}
        onConfirm={async () => { await save.mutateAsync(true).catch(() => undefined); }}
      />
    </form>
  );
}
