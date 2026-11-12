"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { History, RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { FormSection, FormSections, useFormatDate } from "@/components/bits";
import { ConfirmDialog, ConfirmationError } from "@/components/confirm-dialog";
import { fieldsOf } from "@/components/field-builder";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/lib/api/client";
import { records } from "@/lib/api/endpoints";
import type { Collection, DataRecord, RecordVersion } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";

type Change = { key: string; before: unknown; after: unknown };

/** Top-level fields whose value differs between two versions of a record. */
export function changes(before: unknown, after: unknown): Change[] {
  const obj = (v: unknown) => (v && typeof v === "object" && !Array.isArray(v) ? (v as Record<string, unknown>) : { "": v });
  const a = obj(before);
  const b = obj(after);
  return [...new Set([...Object.keys(a), ...Object.keys(b)])]
    .filter((k) => JSON.stringify(a[k]) !== JSON.stringify(b[k]))
    .map((key) => ({ key, before: a[key], after: b[key] }));
}

/** Earlier versions, each with what the next change did to it, and a restore button for editors. */
export function RecordHistory({ collection, record, editable }: { collection: Collection; record: DataRecord; editable: boolean }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const client = useQueryClient();
  const [restoring, setRestoring] = useState<RecordVersion | null>(null);
  const list = useQuery({
    queryKey: ["records", "versions", record.id, record.version],
    queryFn: ({ signal }) => records.versions(record.id, signal),
  });
  const labels = new Map(fieldsOf(collection.schema ?? null).map((r) => [r.key, r.field.name]));
  const show = (v: unknown) => {
    if (v === undefined) return t("history.absent");
    const s = typeof v === "string" ? v : JSON.stringify(v);
    return s.length > 80 ? `${s.slice(0, 80)}…` : s;
  };
  const items = list.data?.items ?? [];

  return (
    <FormSections>
      <FormSection title={t("history.title")} description={t("history.desc")}>
        {list.isPending ? (
          <ListSkeleton rows={2} />
        ) : items.length === 0 ? (
          <p className="flex items-center gap-2 text-sm text-muted-foreground"><History className="size-4" />{t("history.empty")}</p>
        ) : (
          <ol className="divide-y rounded-lg border" aria-label={t("history.title")}>
            {items.map((v, i) => {
              // What the change after this version did: compare with the next newer one.
              const next = i === 0 ? record.data : items[i - 1].data;
              const diff = changes(v.data, next);
              return (
                <li key={v.version} className="space-y-1.5 px-3 py-2.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium">{t("history.version").replace("{n}", String(v.version))}</span>
                    <span className="text-xs text-muted-foreground">{format(v.saved_at)}</span>
                    {editable && (
                      <Button size="sm" variant="ghost" className="ml-auto" onClick={() => setRestoring(v)}>
                        <RotateCcw className="size-3.5" />{t("history.restore")}
                      </Button>
                    )}
                  </div>
                  {diff.length > 0 && (
                    <ul className="space-y-0.5 text-xs">
                      {diff.slice(0, 6).map((c) => (
                        <li key={c.key} className="break-words">
                          <span className="font-medium">{labels.get(c.key) ?? (c.key || t("quality.root"))}</span>:{" "}
                          <span className="text-muted-foreground line-through">{show(c.before)}</span> → <span>{show(c.after)}</span>
                        </li>
                      ))}
                      {diff.length > 6 && <li className="text-muted-foreground">+{diff.length - 6}</li>}
                    </ul>
                  )}
                </li>
              );
            })}
          </ol>
        )}
      </FormSection>
      <ConfirmDialog
        open={!!restoring}
        onOpenChange={(o) => !o && setRestoring(null)}
        title={t("history.restore")}
        description={t("history.restore_confirm").replace("{n}", String(restoring?.version ?? ""))}
        confirmLabel={t("history.restore")}
        onConfirm={async () => {
          if (!restoring) return;
          try {
            const saved = await records.restore(record.id, restoring.version, record.version);
            client.setQueryData(["records", "detail", saved.id], saved);
            void client.invalidateQueries({ queryKey: ["records", record.collection_id] });
            toast.success(t("history.restored"));
          } catch (err) {
            // Old data that breaks today's fields cannot come back as it was.
            if (err instanceof ApiError && err.code === "VALIDATION_FAILED") throw new ConfirmationError(t("history.invalid"));
            throw err;
          }
          setRestoring(null);
        }}
      />
    </FormSections>
  );
}
