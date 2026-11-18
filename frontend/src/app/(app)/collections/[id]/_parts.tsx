"use client";

import dynamic from "next/dynamic";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Braces, ChevronRight, Eye, History, MoreHorizontal, RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { FieldNote, FormSection, FormSections, PageBody, PageHeader, SaveBar, useFormatDate } from "@/components/bits";
import { CollectionAvatar } from "@/components/collection-avatar";
import { IconPicker } from "@/components/icon-picker";
import { EmptyState } from "@/components/empty-state";
import { schemaChanges, type SchemaChange } from "@/components/field-builder";
import { FieldsEditor, SidePanel } from "@/components/fields-editor";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
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
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import type { MessageKey } from "@/lib/i18n/messages";
import { roleAllows } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { useUIStore } from "@/lib/store/ui";
import { cn } from "@/lib/utils";
import { collectionName, description as descriptionRule, ready } from "@/lib/validation";

/** The fields a new collection starts with, named in the user's language. */
export const starter = (t: (k: MessageKey) => string) => ({
  type: "object",
  required: ["title"],
  properties: {
    title: { type: "string", title: t("collections.starter_title") },
    notes: { type: "string", title: t("collections.starter_notes") },
  },
});

export const pretty = (v: unknown) => JSON.stringify(v, null, 2);

// The form generator is the heaviest code on these pages; load it only where a form shows.
export const SchemaForm = dynamic(() => import("@/components/schema-form").then((m) => m.SchemaForm), { ssr: false, loading: () => <ListSkeleton rows={2} /> });

/** The schema text as an object, or why it is not one (the parser's message names the position). */
export function parseSchema(text: string): { schema: Record<string, unknown> | null; error?: string } {
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
export function CollectionPage({ section, description, countRecords = false, action, width, children }: {
  section: MessageKey;
  /** What the section is for, or a line built from the collection and its record count. */
  description?: MessageKey | ((collection: Collection, recordCount?: number) => string | undefined);
  /** Loads the record count for a `description` that shows it. */
  countRecords?: boolean;
  /** The section's main action, drawn in the header as on every other page. */
  action?: (collection: Collection) => React.ReactNode;
  width?: "data" | "form";
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
  useCrumb(collection && t(section), collection && { label: collection.name, href: `/collections/${collection.id}/overview` });
  const recordCount = useRecordCount(collection?.id, countRecords);
  const subtitle = !collection ? undefined
    : typeof description === "function" ? description(collection, recordCount)
    : description && t(description);

  return (
    <>
      <PageHeader title={t(section)} description={subtitle} action={collection && action?.(collection)} />
      <PageBody width={width}>
        {loading || detail.isPending ? (
          <ListSkeleton rows={3} />
        ) : gone ? (
          <EmptyState title={t("error.not_found")} description={t("error.collection_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/collections" />}>{t("common.back_to_list")}</Button>} />
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
function SchemaHistory({ collection, enabled, onLoad }: { collection: Collection; enabled: boolean; onLoad: (schema: Record<string, unknown>) => void }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const list = useQuery({ queryKey: ["schema-versions", collection.id, collection.version], queryFn: ({ signal }) => tools.schemaVersions(collection.id, signal), enabled });
  const items = list.data?.items ?? [];
  return (
    <div className="space-y-2">
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
  );
}

/**
 * Name, description and schema of a collection: each page shows its part
 * ("details" on Settings, "fields" on Fields), and saving sends only what changed.
 * A new collection is made on the new-collection page (_new.tsx).
 */
export function CollectionForm({ collection, parts }: { collection: Collection; parts: ("details" | "fields")[] }) {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const editable = roleAllows(collection.role, "owner");
  const recordCount = useRecordCount(collection.id, parts.includes("fields") && editable);
  const original = pretty(collection.schema ?? starter(t));
  const [name, setName] = useState(collection.name);
  const [desc, setDesc] = useState(collection.description);
  const [icon, setIcon] = useState(collection.icon);
  const [schemaText, setSchemaText] = useState(original);
  const [error, setError] = useState<string | null>(null);
  const [version, setVersion] = useState(collection.version);
  const nameError = collectionName(name);
  const parsed = useMemo(() => parseSchema(schemaText), [schemaText]);
  const schema = parsed.schema;
  const [sheet, setSheet] = useState<"preview" | "history" | "json" | null>(null);
  const [previewData, setPreviewData] = useState<unknown>({});
  const schemaChanged = schemaText !== original;
  const changes = useMemo(() => schemaChanges(collection.schema ?? null, schema), [collection, schema]);
  // Changes that can leave stored records out of format: confirmed first, then Quality is a click away.
  const risky = !!recordCount && changes.some((c) => c.risky);
  const [confirming, setConfirming] = useState(false);
  const dirty = name.trim() !== collection.name || desc.trim() !== collection.description || icon !== collection.icon || schemaChanged;
  useUnsavedGuard(editable && dirty, t("roles.leave_unsaved"));

  // The variable says whether the save was risky, for the toast that follows.
  const save = useMutation<Collection, unknown, boolean>({
    mutationFn: () => {
      return collections.update(collection.id, { name: name.trim(), description: desc.trim(), icon: icon !== collection.icon ? icon : undefined, schema: schemaChanged ? schema : undefined, version });
    },
    onSuccess: (saved, wasRisky) => {
      setError(null);
      client.setQueryData(["collections", "detail", saved.id], saved);
      void client.invalidateQueries({ queryKey: ["collections"] });
      toast.success(t("common.saved"), wasRisky ? { action: { label: t("collections.check_quality"), onClick: () => router.push(`/collections/${saved.id}/quality`) } } : undefined);
      setVersion(saved.version);
    },
    onError: (err) => {
      // The backend names the broken schema rule; it reads better than a generic failure.
      setError(err instanceof ApiError && err.field === "schema" ? err.message : errorMessage(err, "common.save_failed", t));
      if (err instanceof ApiError && err.code === "CONFLICT") void client.invalidateQueries({ queryKey: ["collections", "detail", collection.id] });
    },
  });

  const discard = () => {
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
      if (risky) setConfirming(true);
      else save.mutate(false);
    }}
      // Ctrl/⌘+Enter saves from any field.
      onKeyDown={(e) => { if (editable && dirty && e.key === "Enter" && (e.ctrlKey || e.metaKey)) { e.preventDefault(); e.currentTarget.requestSubmit(); } }}>
      <FormSections>
        {!editable && <p role="status" className="rounded-md border bg-muted/40 px-3 py-2 text-sm text-muted-foreground">{t("collections.readonly")}</p>}
        {parts.includes("details") && (
          <FormSection title={t("collections.details")} description={t("collections.details_desc")}>
            <div className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="c-name">{t("auth.name")}</Label>
                <Input id="c-name" required maxLength={120} aria-invalid={!!nameError} aria-describedby="c-name-note" disabled={!editable} value={name} onChange={(e) => setName(e.target.value)} />
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
                  <CollectionAvatar collection={{ id: collection.id, name: name.trim() || "?", icon }} className="size-6 rounded-md text-xs font-semibold" />
                </p>
              </div>
            </div>
          </FormSection>
        )}
        {parts.includes("fields") && (
          // No side heading here: the list needs the width.
          <section className="space-y-4 border-t pt-8 first:border-t-0 first:pt-0">
            {/* The Fields page says this in its header; the new-collection page needs it here. The save bar says what is unsaved. */}
            {parts.includes("details") && (
              <div className="space-y-1">
                <h2 className="font-heading text-base font-semibold">{t("collections.fields")}</h2>
                <p className="text-sm text-muted-foreground">{t("collections.fields_desc")}</p>
              </div>
            )}
            {/* One column, the card's full width as on every other page: each field opens in a side panel, and so do the preview, the history and the JSON. */}
            <div className="space-y-4">
                <div className="flex flex-wrap items-center justify-end gap-2">
                  <Button type="button" variant="outline" size="sm" disabled={!schema} onClick={() => setSheet("preview")}><Eye className="size-4" />{t("collections.preview")}</Button>
                  <DropdownMenu>
                    <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="icon-sm" aria-label={t("fields.more")} />}><MoreHorizontal className="size-4" /></DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      {editable && <DropdownMenuItem onClick={() => setSheet("history")}><History className="size-4" />{t("field_history.title")}</DropdownMenuItem>}
                      <DropdownMenuItem onClick={() => setSheet("json")}><Braces className="size-4" />{t("collections.json_toggle")}</DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
                {!schema && (
                  <div role="alert" className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm">
                    <span>{[t("collections.schema_invalid"), parsed.error].filter(Boolean).join(": ")}</span>
                    <Button type="button" size="sm" variant="outline" onClick={() => setSheet("json")}><Braces className="size-4" />{t("collections.json_toggle")}</Button>
                  </div>
                )}
                <FieldsEditor
                  schema={schema}
                  original={collection.schema ?? null}
                  editable={editable}
                  keysLocked={recordCount !== 0}
                  onChange={(next) => setSchemaText(pretty(next))}
                />
                {changes.length > 0 && (
                  // Compact: one line saying how many changes, the list on demand.
                  <details role="status" className={cn("group rounded-md border px-3 py-2 text-sm", risky ? "border-warn/40 bg-warn/10" : "bg-muted/40")}>
                    <summary className="flex cursor-pointer list-none items-center gap-2 font-medium [&::-webkit-details-marker]:hidden">
                      <ChevronRight className="size-4 transition-transform group-open:rotate-90" aria-hidden />
                      {plural(t("fields.changes_count"), changes.length)}
                      {risky && <span className="font-normal text-warn">· {t("fields.changes_risky")}</span>}
                    </summary>
                    <ul className="mt-1.5 list-inside list-disc space-y-0.5 pl-6">
                      {changes.map((c, i) => <li key={i}>{changeText(c)}</li>)}
                    </ul>
                    {risky && <p className="mt-1.5 pl-6">{plural(t("collections.schema_change_hint"), recordCount)}</p>}
                  </details>
                )}
              </div>
            <SidePanel open={sheet === "preview"} onOpenChange={(o) => !o && setSheet(null)} title={t("collections.preview")} description={t("collections.preview_desc")} wide>
              {schema && <SchemaForm schema={schema} data={previewData} onChange={setPreviewData} idPrefix="preview" columns={2} />}
            </SidePanel>
            {editable && (
              <SidePanel open={sheet === "history"} onOpenChange={(o) => !o && setSheet(null)} title={t("field_history.title")}>
                <SchemaHistory collection={collection} enabled={sheet === "history"} onLoad={(old) => { setSchemaText(pretty(old)); setSheet(null); toast.info(t("field_history.loaded")); }} />
              </SidePanel>
            )}
            <SidePanel open={sheet === "json"} onOpenChange={(o) => !o && setSheet(null)} title={t("collections.json_toggle")} description={t("collections.schema_desc")} wide>
              <div className="space-y-2">
                <Textarea
                  id="c-schema"
                  aria-label={t("collections.schema")}
                  aria-invalid={!schema}
                  aria-describedby="c-schema-note"
                  spellCheck={false}
                  rows={Math.min(40, Math.max(12, schemaText.split("\n").length + 1))}
                  disabled={!editable}
                  value={schemaText}
                  onChange={(e) => setSchemaText(e.target.value)}
                  className="px-3 font-mono text-xs leading-relaxed md:text-xs"
                />
                <FieldNote id="c-schema-note" error={!schema ? [t("collections.schema_invalid"), parsed.error].filter(Boolean).join(": ") : undefined} />
              </div>
            </SidePanel>
          </section>
        )}
        {editable && dirty && (
          <SaveBar
            footer
            submit
            label={t("roles.unsaved")}
            status={
              <span className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                {t("roles.unsaved")}
                <span className="hidden text-xs font-normal text-muted-foreground sm:inline"><kbd className="font-mono">Ctrl</kbd>+<kbd className="font-mono">Enter</kbd> {t("records.shortcut_save")}</span>
              </span>
            }
            error={error}
            saving={save.isPending}
            canSave={!!schema && !descriptionRule(desc) && dirty && ready([name, collectionName])}
            saveLabel={t("common.save")}
            discardLabel={t("common.discard")}
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
