"use client";

import Link from "next/link";
import { useState } from "react";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { toast } from "sonner";
import { DangerZone, FormSection, FormSections, PageBody, PageHeader, SaveBar, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { SchemaForm } from "@/components/schema-form";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/lib/api/client";
import { collections, records } from "@/lib/api/endpoints";
import type { Collection, DataRecord, SchemaIssue } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { useUnsavedGuard } from "@/lib/hooks/use-unsaved-guard";
import { useI18n } from "@/lib/i18n/provider";
import { can } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";

/** Create when `record` is null, edit otherwise. The server checks the data against the schema. */
function RecordForm({ collection, record, editable }: { collection: Collection; record: DataRecord | null; editable: boolean }) {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const [data, setData] = useState<unknown>(record?.data ?? {});
  const [version, setVersion] = useState(record?.version);
  const [error, setError] = useState<string | null>(null);
  const [issues, setIssues] = useState<SchemaIssue[]>([]);
  const dirty = JSON.stringify(data) !== JSON.stringify(record?.data ?? {});
  useUnsavedGuard(editable && dirty, t("roles.leave_unsaved"));
  const back = `/collections/${collection.id}`;

  const save = useMutation({
    mutationFn: () => (record ? records.update(record.id, data, version) : records.create(collection.id, data)),
    onSuccess: (saved) => {
      setError(null);
      setIssues([]);
      client.setQueryData(["records", "detail", saved.id], saved);
      void client.invalidateQueries({ queryKey: ["records", collection.id] });
      void client.invalidateQueries({ queryKey: ["collections"] });
      toast.success(t("common.saved"));
      router.push(back);
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
  };

  return (
    <form onSubmit={(e) => { e.preventDefault(); save.mutate(); }}>
      <FormSections>
        <FormSection title={t("records.data")} description={t("records.data_desc")}>
          <SchemaForm schema={collection.schema ?? {}} data={data} disabled={!editable} onChange={setData} />
          {issues.length > 0 && (
            <ul className="mt-4 space-y-1 rounded-lg border border-destructive/40 p-3 text-sm" role="alert">
              {issues.map((i, n) => (
                <li key={n}><code className="font-mono text-xs text-muted-foreground">{i.path || "/"}</code> {i.message}</li>
              ))}
            </ul>
          )}
        </FormSection>
        {editable && (dirty || !record) && (
          <SaveBar
            footer
            submit
            label={t(record ? "roles.unsaved" : "records.new")}
            error={error}
            saving={save.isPending}
            canSave={dirty || !record}
            saveLabel={t(record ? "common.save" : "common.create")}
            discardLabel={t(record ? "common.discard" : "common.cancel")}
            onDiscard={discard}
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
  const { user, loading } = useMe();
  const { id } = useParams<{ id: string }>();
  const creating = id === "new";
  const fromQuery = useSearchParams().get("collection") ?? "";
  const detail = useQuery({ queryKey: ["records", "detail", id], queryFn: () => records.get(id), enabled: !creating });
  const record = detail.data;
  const collectionId = creating ? fromQuery : record?.collection_id;
  const parent = useQuery({ queryKey: ["collections", "detail", collectionId], queryFn: () => collections.get(collectionId!), enabled: !!collectionId });
  const collection = parent.data;
  const [confirmDelete, setConfirmDelete] = useState(false);
  const editable = can(user, "records", creating ? "create" : "update");
  useCrumb(creating ? t("records.new") : t("records.record"));
  const failed = detail.error ?? parent.error;
  const back = collection && <Button variant="ghost" nativeButton={false} render={<Link href={`/collections/${collection.id}`} />}><ArrowLeft className="size-4" />{collection.name}</Button>;

  return (
    <>
      <PageHeader
        title={creating ? t("records.new") : t("records.record")}
        description={record && `${t("common.updated_at")} ${format(record.updated_at)}`}
        action={back}
      />
      <PageBody>
        {loading || (!creating && detail.isPending) || (!!collectionId && parent.isPending) ? (
          <ListSkeleton rows={3} />
        ) : failed instanceof ApiError && failed.code === "NOT_FOUND" || (creating && !collectionId) ? (
          <EmptyState title={t("error.not_found")} description={t("error.record_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/collections" />}>{t("common.back_to_list")}</Button>} />
        ) : failed || !collection ? (
          <QueryErrorState error={failed} onRetry={() => { void detail.refetch(); void parent.refetch(); }} />
        ) : (
          <RecordForm key={record?.id ?? "new"} collection={collection} record={record ?? null} editable={editable} />
        )}
        {record && can(user, "records", "delete") && (
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
