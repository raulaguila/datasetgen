"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, RotateCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { FormError, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { recordTitle } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { SelectAllHead, SelectCell, SelectionBar, useSelection } from "@/components/selection";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { tools } from "@/lib/api/endpoints";
import { ApiError } from "@/lib/api/client";
import type { Collection, TrashedRecord } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

/** Deleted records wait here for 30 days: editors restore them, owners delete them for good. */
function Trash({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["trash", collection.id];
  const list = useQuery({ queryKey: key, queryFn: ({ signal }) => tools.trash(collection, signal) });
  const items = list.data?.items ?? [];
  const selection = useSelection(items.map((r) => r.id));
  const owner = roleAllows(collection.role, "owner");
  const [purging, setPurging] = useState<string[] | null>(null);
  const [renewing, setRenewing] = useState<{ record: TrashedRecord; fields: string[] } | null>(null);
  const settle = () => {
    selection.clear();
    void client.invalidateQueries({ queryKey: key });
    void client.invalidateQueries({ queryKey: ["records", collection.id] });
    void client.invalidateQueries({ queryKey: ["collections"] });
  };
  const restore = useMutation({
    mutationFn: (ids: string[]) => tools.restoreRecords(collection, ids),
    onSuccess: (res) => { toast.success(plural(t("trash.restored"), res.count)); settle(); },
    onError: (err, ids) => {
      const fields = takenFields(err);
      const one = ids.length === 1 ? items.find((r) => r.id === ids[0]) : undefined;
      // A unique value another record took: one record takes a new value; several, one at a time.
      if (fields && one) setRenewing({ record: one, fields });
      else if (fields) toast.error(t("trash.taken_many").replace("{fields}", fields.map((k) => fieldTitle(collection, k)).join(", ")));
      else toast.error(errorMessage(err, "common.save_failed", t));
    },
  });
  const purge = async (ids: string[]) => {
    try {
      const res = await tools.purgeRecords(collection, ids);
      toast.success(plural(t("trash.purged"), res.count));
      settle();
    } catch (err) {
      toast.error(errorMessage(err, "common.save_failed", t));
    }
  };

  if (list.isPending) return <ListSkeleton rows={3} />;
  if (list.isError) return <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />;
  if (!items.length) return <EmptyState icon={Trash2} title={t("trash.empty")} description={t("trash.retention").replace("{n}", String(list.data.retention_days))} />;
  return (
    <>
      <p className="text-sm text-muted-foreground">{t("trash.retention").replace("{n}", String(list.data.retention_days))}</p>
      <Card className="gap-0 overflow-hidden p-0">
        <Table data-responsive data-compact>
          <TableHeader>
            <TableRow>
              <SelectAllHead selection={selection} />
              <TableHead className="w-[45%]">{t("records.record")}</TableHead>
              <TableHead>{t("trash.deleted_by")}</TableHead>
              <TableHead className="w-44">{t("trash.deleted_at")}</TableHead>
              <TableHead className="w-28"><span className="sr-only">{t("common.actions")}</span></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((r) => {
              const title = recordTitle(collection.schema ?? null, r.data) ?? r.id.slice(0, 8);
              return (
                <TableRow key={r.id}>
                  <SelectCell selection={selection} id={r.id} label={title} />
                  <TableCell data-label={t("records.record")} data-cell="name" className="max-w-0 min-w-40"><span className="block truncate font-medium">{title}</span></TableCell>
                  <TableCell data-label={t("trash.deleted_by")} className="text-muted-foreground">{r.deleted_by_name ?? "—"}</TableCell>
                  <TableCell data-label={t("trash.deleted_at")} data-cell="date" className="whitespace-nowrap text-muted-foreground">{format(r.deleted_at)}</TableCell>
                  <TableCell className="text-right">
                    <Button size="sm" variant="outline" disabled={restore.isPending} onClick={() => restore.mutate([r.id])}><RotateCcw className="size-4" />{t("trash.restore")}</Button>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </Card>
      <SelectionBar selection={selection}>
        <Button variant="outline" disabled={restore.isPending} onClick={() => restore.mutate(selection.ids)}><RotateCcw className="size-4" />{t("trash.restore")}</Button>
        {owner && <Button variant="destructive" onClick={() => setPurging(selection.ids)}><Trash2 className="size-4" />{t("trash.purge")}</Button>}
      </SelectionBar>
      <ConfirmDialog
        open={!!purging}
        onOpenChange={(o) => !o && setPurging(null)}
        title={plural(t("trash.purge_title"), purging?.length ?? 0)}
        description={t("trash.purge_desc")}
        confirmLabel={t("trash.purge")}
        destructive
        onConfirm={async () => { if (purging) await purge(purging); setPurging(null); }}
      />
      {renewing && <RenewDialog collection={collection} {...renewing} onClose={() => setRenewing(null)} onDone={(n) => { toast.success(plural(t("trash.restored"), n)); setRenewing(null); settle(); }} />}
    </>
  );
}

/** The unique fields a refused restore names, when that is why it was refused. */
function takenFields(err: unknown): string[] | null {
  const fields = err instanceof ApiError && err.code === "CONFLICT" ? err.details?.fields : undefined;
  return Array.isArray(fields) && fields.length ? fields.map(String) : null;
}

const fieldOf = (collection: Collection, key: string) => (collection.schema?.properties as Record<string, { title?: string; type?: string }> | undefined)?.[key];
const fieldTitle = (collection: Collection, key: string) => fieldOf(collection, key)?.title || key;

/** Restores one record whose unique values another record took since, with new ones. */
function RenewDialog({ collection, record, fields, onClose, onDone }: { collection: Collection; record: TrashedRecord; fields: string[]; onClose: () => void; onDone: (n: number) => void }) {
  const { t } = useI18n();
  const held = (record.data ?? {}) as Record<string, unknown>;
  const [values, setValues] = useState<Record<string, string>>(() => Object.fromEntries(fields.map((k) => [k, held[k] == null ? "" : String(held[k])])));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const numeric = (k: string) => ["number", "integer"].includes(fieldOf(collection, k)?.type ?? "");
  const blank = fields.some((k) => !values[k]?.trim());

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const sent = Object.fromEntries(fields.map((k) => [k, numeric(k) ? Number(values[k]) : values[k].trim()]));
      const res = await tools.restoreRecords(collection, [record.id], sent);
      onDone(res.count);
    } catch (err) {
      setError(takenFields(err) ? t("trash.still_taken") : errorMessage(err, "common.save_failed", t));
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("trash.taken_title")}</DialogTitle>
          <DialogDescription>{t("trash.taken_desc")}</DialogDescription>
        </DialogHeader>
        <form id="renew-form" className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (!blank) void submit(); }}>
          {fields.map((k, i) => (
            <div key={k} className="space-y-2">
              <Label htmlFor={`renew-${k}`}>{fieldTitle(collection, k)}</Label>
              <Input id={`renew-${k}`} autoFocus={i === 0} inputMode={numeric(k) ? "decimal" : undefined} value={values[k]} onChange={(e) => setValues((v) => ({ ...v, [k]: e.target.value }))} />
            </div>
          ))}
          <FormError message={error} />
        </form>
        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={onClose}>{t("common.cancel")}</Button>
          <Button type="submit" form="renew-form" disabled={busy || blank}>{busy && <Loader2 className="size-4 animate-spin" />}{t("trash.restore")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export default function CollectionTrashPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.trash" description="trash.subtitle">
      {(collection) => roleAllows(collection.role, "editor")
        ? <div className="space-y-4"><Trash collection={collection} /></div>
        : <EmptyState icon={Trash2} title={t("error.forbidden_page")} description={t("trash.editors_only")} />}
    </CollectionPage>
  );
}
