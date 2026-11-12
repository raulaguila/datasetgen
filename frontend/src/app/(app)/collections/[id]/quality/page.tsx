"use client";

import Link from "next/link";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { CircleCheck, ListChecks, RefreshCw, Trash2, Wrench } from "lucide-react";
import { toast } from "sonner";
import { FormSection, FormSections, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { fieldsOf } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { collections } from "@/lib/api/endpoints";
import type { Collection, QualityReport, RecordFix } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { useQuality } from "@/lib/hooks/use-quality";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

/** The fix form: one bulk change, confirmed, then the check runs again. */
function BulkFix({ collection, report, onDone }: { collection: Collection; report: QualityReport; onDone: () => void }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const [op, setOp] = useState<RecordFix["op"]>("set_default");
  const [field, setField] = useState("");
  const [to, setTo] = useState("");
  const [value, setValue] = useState("");
  const [confirm, setConfirm] = useState<"fix" | "delete" | null>(null);
  // Field keys of the schema plus those the issues name, which may be old ones.
  const keys = [...new Set([
    ...fieldsOf(collection.schema ?? null).map((r) => r.key),
    ...report.invalid.flatMap((r) => [...r.issues.map((i) => (i.path.split("/")[1] ?? "")), ...Object.keys(r.data ?? {})]),
  ].filter(Boolean))];
  const parsed = (() => {
    try { return JSON.parse(value) as unknown; } catch { return value; } // plain text is a string
  })();
  const ready = field.trim() !== "" && (op !== "rename" || (to.trim() !== "" && to.trim() !== field.trim())) && (op !== "set_default" || value !== "");
  const fix: RecordFix = { op, field: field.trim(), ...(op === "rename" ? { to: to.trim() } : {}), ...(op === "set_default" ? { value: parsed } : {}) };
  const after = (n: number, key: MessageKey) => {
    toast.success(t(key).replace("{n}", String(n)));
    void client.invalidateQueries({ queryKey: ["records", collection.id] });
    void client.invalidateQueries({ queryKey: ["collections"] });
    onDone();
  };
  const canDelete = report.status === "done" && report.version === report.current_version && report.invalid_count > 0;
  const selectCls = "h-9 w-full rounded-lg border border-input bg-card px-2 text-sm dark:bg-transparent";

  return (
    <FormSections>
      <FormSection title={t("fix.title")} description={t("fix.desc")}>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (ready) setConfirm("fix"); }}>
          <div className="space-y-2">
            <Label htmlFor="fix-op">{t("fix.op")}</Label>
            <select id="fix-op" className={selectCls} value={op} onChange={(e) => setOp(e.target.value as RecordFix["op"])}>
              <option value="set_default">{t("fix.op.set_default")}</option>
              <option value="rename">{t("fix.op.rename")}</option>
              <option value="remove">{t("fix.op.remove")}</option>
            </select>
          </div>
          <div className="space-y-2">
            <Label htmlFor="fix-field">{t(op === "rename" ? "fix.from" : "fix.field")}</Label>
            <Input id="fix-field" list="fix-keys" maxLength={200} value={field} onChange={(e) => setField(e.target.value)} />
            <datalist id="fix-keys">{keys.map((k) => <option key={k} value={k} />)}</datalist>
          </div>
          {op === "rename" && (
            <div className="space-y-2">
              <Label htmlFor="fix-to">{t("fix.to")}</Label>
              <Input id="fix-to" list="fix-keys" maxLength={200} value={to} onChange={(e) => setTo(e.target.value)} />
            </div>
          )}
          {op === "set_default" && (
            <div className="space-y-2">
              <Label htmlFor="fix-value">{t("fix.value")}</Label>
              <Input id="fix-value" value={value} onChange={(e) => setValue(e.target.value)} aria-describedby="fix-value-hint" />
              <p id="fix-value-hint" className="text-xs text-muted-foreground">{t("fix.value_hint")}</p>
            </div>
          )}
          <p className="text-xs text-muted-foreground">{t("fix.versions_hint")}</p>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={!ready}><Wrench className="size-4" />{t("fix.apply")}</Button>
            {canDelete && (
              <Button type="button" variant="outline" className="text-destructive" onClick={() => setConfirm("delete")}>
                <Trash2 className="size-4" />{t("fix.delete_invalid").replace("{n}", String(report.invalid_count))}
              </Button>
            )}
          </div>
        </form>
      </FormSection>
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={t(confirm === "delete" ? "fix.delete_title" : "fix.confirm_title")}
        description={confirm === "delete" ? t("fix.delete_confirm").replace("{n}", String(report.invalid_count)) : t("fix.confirm")}
        destructive={confirm === "delete" || op === "remove"}
        confirmLabel={t(confirm === "delete" ? "common.delete" : "fix.apply")}
        onConfirm={async () => {
          try {
            if (confirm === "delete") after((await collections.deleteInvalid(collection.id)).count, "fix.deleted");
            else after((await collections.fix(collection.id, fix)).count, "fix.changed");
          } catch (err) {
            toast.error(errorMessage(err, "common.save_failed", t));
          }
          setConfirm(null);
        }}
      />
    </FormSections>
  );
}

/** Records that a change of fields left out of format, each with the rules it breaks. */
function Quality({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const { q, start, stale } = useQuality(collection);
  // Issues name the field by its key; show its label, as the form does.
  const labels = new Map(fieldsOf(collection.schema ?? null).map((r) => [r.key, r.field.name]));
  const field = (path: string) => {
    const key = (path.split("/")[1] ?? "").replace(/~1/g, "/").replace(/~0/g, "~");
    return key ? labels.get(key) ?? key : t("quality.root");
  };
  const issue = (keyword: string, message: string) => {
    const text = t(`quality.issue.${keyword}` as MessageKey);
    return text.startsWith("quality.issue.") ? message : text;
  };
  const preview = (id: string, data: Record<string, unknown>) => {
    const first = [...labels.keys()].map((k) => data?.[k]).find((v) => typeof v === "string" && v);
    return typeof first === "string" ? first : `${t("records.record")} ${id.slice(0, 8)}`;
  };

  if (q.isPending) return <ListSkeleton rows={3} />;
  if (q.isError) return <QueryErrorState error={q.error} onRetry={() => void q.refetch()} />;
  const r = q.data;
  const running = r.status === "running" || start.isPending;
  const recheck = (
    <Button variant="outline" disabled={running} onClick={() => start.mutate()}>
      <RefreshCw className={running ? "size-4 animate-spin" : "size-4"} />{t(running ? "quality.running" : "quality.recheck")}
    </Button>
  );
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-3">
        <p className="text-sm text-muted-foreground" role="status">
          {running ? t("quality.running_desc")
            : r.status === "failed" ? t("quality.failed")
            : stale ? t("quality.stale")
            : <>
              {t("quality.checked").replace("{n}", String(r.checked))}
              {r.finished_at && ` · ${format(r.finished_at)}`}
              {r.truncated && ` ${t("quality.truncated")}`}
            </>}
        </p>
        <div className="ml-auto">{recheck}</div>
      </div>
      {r.status === "done" && !stale && (r.invalid_count === 0 ? (
        <EmptyState icon={CircleCheck} title={t("quality.all_valid")} />
      ) : (
        <Card className="gap-0 overflow-hidden p-0">
          <div className="border-b px-4 py-3">
            <h2 className="font-heading text-base font-semibold">{t("quality.invalid").replace("{n}", String(r.invalid_count))}</h2>
            <p className="text-sm text-muted-foreground">
              {t("quality.invalid_desc")}
              {r.invalid_count > r.invalid.length && ` ${t("quality.first").replace("{n}", String(r.invalid.length))}`}
            </p>
          </div>
          <ul aria-label={t("quality.invalid").replace("{n}", String(r.invalid_count))} className="divide-y">
            {r.invalid.map((rec) => (
              <li key={rec.id} className="flex flex-wrap items-start gap-x-4 gap-y-1 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <Link href={`/records/${rec.id}`} className="block truncate text-sm font-medium hover:underline">{preview(rec.id, rec.data)}</Link>
                  <ul className="mt-1 space-y-0.5 text-xs">
                    {rec.issues.map((i, n) => (
                      <li key={n}><span className="font-medium">{field(i.path)}</span>: <span className="text-destructive">{issue(i.keyword, i.message)}</span></li>
                    ))}
                  </ul>
                </div>
                <span className="text-xs text-muted-foreground">{format(rec.updated_at)}</span>
              </li>
            ))}
          </ul>
        </Card>
      ))}
      <BulkFix collection={collection} report={r} onDone={() => start.mutate()} />
    </div>
  );
}

export default function CollectionQualityPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.quality">
      {(collection) => roleAllows(collection.role, "editor")
        ? <Quality collection={collection} />
        : <EmptyState icon={ListChecks} title={t("error.forbidden")} />}
    </CollectionPage>
  );
}
