"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import type Form from "@rjsf/core";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { toast } from "sonner";
import { DangerZone, FormSection, FormSections, PageBody, PageHeader, SaveBar, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { recordTitle } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { RecordHistory } from "@/components/record-history";
import { SchemaForm } from "@/components/schema-form";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { ApiError } from "@/lib/api/client";
import { collections, records } from "@/lib/api/endpoints";
import type { Collection, DataRecord, SchemaIssue } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { useUIStore } from "@/lib/store/ui";

/** Create when `record` is null, edit otherwise. The server checks the data against the schema. */
function RecordForm({ collection, record, editable }: { collection: Collection; record: DataRecord | null; editable: boolean }) {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const [data, setData] = useState<unknown>(record?.data ?? {});
  const [version, setVersion] = useState(record?.version);
  const [error, setError] = useState<string | null>(null);
  const [issues, setIssues] = useState<SchemaIssue[]>([]);
  const formRef = useRef<Form>(null);
  const formEl = useRef<HTMLFormElement>(null);
  const [checked, setChecked] = useState(false); // errors follow the typing once a save was tried
  const dirty = JSON.stringify(data) !== JSON.stringify(record?.data ?? {});
  useUnsavedGuard(editable && dirty, t("roles.leave_unsaved"));
  const back = `/collections/${collection.id}`;
  const canSubmit = editable && (dirty || !record);
  // A new record starts typing in its first field.
  useEffect(() => {
    if (!record && editable) formEl.current?.querySelector<HTMLElement>("input:not([type=hidden]), textarea, select")?.focus();
  }, [record, editable]);

  // The variable is the message shown once saved.
  const save = useMutation<DataRecord, unknown, string>({
    mutationFn: () => (record ? records.update(record.id, data, version) : records.create(collection.id, data)),
    onSuccess: (saved, message) => {
      setError(null);
      setIssues([]);
      client.setQueryData(["records", "detail", saved.id], saved);
      void client.invalidateQueries({ queryKey: ["records", collection.id] });
      void client.invalidateQueries({ queryKey: ["collections"] });
      toast.success(message);
    },
    onError: (err) => {
      const found = err instanceof ApiError && err.code === "VALIDATION_FAILED" ? (err.details?.issues as SchemaIssue[] | undefined) : undefined;
      setIssues(found ?? []);
      setError(found ? t("records.invalid") : errorMessage(err, "common.save_failed", t));
      if (record && err instanceof ApiError && err.code === "CONFLICT") void client.invalidateQueries({ queryKey: ["records", "detail", record.id] });
    },
  });

  const discard = () => {
    if (!record) return router.push(back);
    setData(record.data);
    setVersion(record.version);
    setError(null);
    setIssues([]);
    setChecked(false);
  };

  /**
   * The browser checks first, so broken fields light up without a round trip.
   * After saving: "blank" stays on an empty form, "copy" keeps what was typed
   * so the next record changes only what differs; otherwise back to the list.
   */
  const submit = (again?: "blank" | "copy") => {
    setIssues([]);
    setChecked(true);
    if (!formRef.current?.validateForm()) return setError(t("records.invalid"));
    setError(null);
    save.mutate(t(again === "copy" ? "records.copy_kept" : "common.saved"), {
      onSuccess: () => {
        if (!again) return router.push(back);
        // Entering a batch: stay on the form for the next record.
        if (again === "blank") setData({});
        setChecked(false);
        window.scrollTo({ top: 0, behavior: "smooth" });
        formEl.current?.querySelector<HTMLElement>("input:not([type=hidden]), textarea, select")?.focus();
      },
    });
  };

  return (
    // noValidate: the browser's own bubbles would cut in before validateForm shows errors per field.
    <form ref={formEl} noValidate onSubmit={(e) => { e.preventDefault(); submit(); }}
      // Ctrl/⌘+Enter saves from any field, text areas included; with Shift a new record keeps the data for the next one.
      onKeyDown={(e) => {
        if (!canSubmit || e.key !== "Enter" || !(e.ctrlKey || e.metaKey)) return;
        e.preventDefault();
        submit(e.shiftKey && !record ? "copy" : undefined);
      }}>
      <FormSections>
        <FormSection title={collection.name} description={[collection.description.trim(), t("records.required_legend")].filter(Boolean).join(" ")}>
          <SchemaForm schema={collection.schema ?? {}} collectionId={collection.id} data={data} disabled={!editable} onChange={setData} formRef={formRef} liveValidate={checked} issues={issues} />
        </FormSection>
        {canSubmit && (
          <SaveBar
            footer
            submit
            label={t(record ? "roles.unsaved" : "records.new")}
            status={
              <span className="flex flex-wrap items-baseline gap-x-3 gap-y-1">
                {dirty ? t("roles.unsaved") : t("records.nothing_typed")}
                <span className="hidden text-xs font-normal text-muted-foreground sm:inline"><kbd className="font-mono">Ctrl</kbd>+<kbd className="font-mono">Enter</kbd> {t("records.shortcut_save")}</span>
              </span>
            }
            error={error}
            saving={save.isPending}
            canSave={dirty || !record}
            saveLabel={t(record ? "common.save" : "common.create")}
            discardLabel={t(record ? "common.discard" : "common.cancel")}
            onDiscard={discard}
            extra={!record && (
              // Split button: blank by default, the menu keeps the data for a near-copy.
              <div className="flex">
                <Button type="button" variant="outline" className="rounded-r-none" disabled={save.isPending} onClick={() => submit("blank")}>{t("records.save_and_new")}</Button>
                <DropdownMenu>
                  <DropdownMenuTrigger render={<Button type="button" variant="outline" size="icon" className="rounded-l-none border-l-0" disabled={save.isPending} aria-label={t("records.save_more")} />}>
                    <ChevronDown className="size-4" />
                  </DropdownMenuTrigger>
                  <DropdownMenuContent align="end" className="min-w-64">
                    <DropdownMenuItem onClick={() => submit("copy")}>
                      <span className="flex flex-col">
                        {t("records.save_and_copy")}
                        <span className="text-xs text-muted-foreground">{t("records.save_and_copy_hint")}</span>
                      </span>
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>
            )}
          />
        )}
      </FormSections>
    </form>
  );
}

export default function RecordPage() {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const format = useFormatDate();
  const { loading } = useMe();
  const { id } = useParams<{ id: string }>();
  const creating = id === "new";
  const fromQuery = useSearchParams().get("collection") ?? "";
  const detail = useQuery({ queryKey: ["records", "detail", id], queryFn: () => records.get(id), enabled: !creating });
  const record = detail.data;
  const collectionId = creating ? fromQuery : record?.collection_id;
  const parent = useQuery({ queryKey: ["collections", "detail", collectionId], queryFn: () => collections.get(collectionId!), enabled: !!collectionId });
  const collection = parent.data;
  // The sidebar follows the collection this record belongs to.
  const setCollection = useUIStore((st) => st.setCollection);
  useEffect(() => { if (collection) setCollection(collection.id); }, [collection, setCollection]);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const editable = roleAllows(collection?.role, "editor");
  // Named by its first text value, as lists show it; the id when it has none.
  const title = creating ? t("records.new") : record ? recordTitle(collection?.schema ?? null, record.data) ?? `${t("records.record")} ${record.id.slice(0, 8)}` : t("records.record");
  useCrumb(title, collection && { label: collection.name, href: `/collections/${collection.id}` });
  const failed = detail.error ?? parent.error;

  return (
    <>
      <PageHeader
        title={title}
        description={record && `${t("common.updated_at")} ${format(record.updated_at)}`}
      />
      <PageBody>
        {loading || (!creating && detail.isPending) || (!!collectionId && parent.isPending) ? (
          <ListSkeleton rows={3} />
        ) : failed instanceof ApiError && failed.code === "NOT_FOUND" || (creating && !collectionId) ? (
          <EmptyState title={t("error.not_found")} description={t("error.record_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/collections" />}>{t("common.back_to_list")}</Button>} />
        ) : failed || !collection ? (
          <QueryErrorState error={failed} onRetry={() => { void detail.refetch(); void parent.refetch(); }} />
        ) : (
          <>
            {/* Keyed by version: a restore elsewhere on the page reloads the form. */}
            <RecordForm key={record ? `${record.id}:${record.version}` : "new"} collection={collection} record={record ?? null} editable={editable} />
            {record && <RecordHistory collection={collection} record={record} editable={editable} />}
          </>
        )}
        {record && editable && (
          <DangerZone description={t("records.delete_hint")} label={t("records.delete")} onClick={() => setConfirmDelete(true)} />
        )}
      </PageBody>
      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={t("records.delete")}
        description={t("records.delete_hint")}
        destructive
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          await records.remove([id]);
          router.push(collectionId ? `/collections/${collectionId}` : "/collections");
          client.removeQueries({ queryKey: ["records", "detail", id] });
          void client.invalidateQueries({ queryKey: ["records"] });
          void client.invalidateQueries({ queryKey: ["collections"] });
        }}
      />
    </>
  );
}
