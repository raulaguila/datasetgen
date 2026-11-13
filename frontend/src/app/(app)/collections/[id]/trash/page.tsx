"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RotateCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { recordTitle } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { SelectAllHead, SelectCell, SelectionBar, useSelection } from "@/components/selection";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { tools } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

/** Deleted records wait here for 30 days: editors restore them, owners delete them for good. */
function Trash({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["trash", collection.id];
  const list = useQuery({ queryKey: key, queryFn: ({ signal }) => tools.trash(collection.id, signal) });
  const items = list.data?.items ?? [];
  const selection = useSelection(items.map((r) => r.id));
  const owner = roleAllows(collection.role, "owner");
  const [purging, setPurging] = useState<string[] | null>(null);
  const settle = () => {
    selection.clear();
    void client.invalidateQueries({ queryKey: key });
    void client.invalidateQueries({ queryKey: ["records", collection.id] });
    void client.invalidateQueries({ queryKey: ["collections"] });
  };
  const restore = useMutation({
    mutationFn: (ids: string[]) => tools.restoreRecords(collection.id, ids),
    onSuccess: (res) => { toast.success(t("trash.restored").replace("{n}", String(res.count))); settle(); },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const purge = async (ids: string[]) => {
    try {
      const res = await tools.purgeRecords(collection.id, ids);
      toast.success(t("trash.purged").replace("{n}", String(res.count)));
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
        title={t("trash.purge_title").replace("{n}", String(purging?.length ?? 0))}
        description={t("trash.purge_desc")}
        confirmLabel={t("trash.purge")}
        destructive
        onConfirm={async () => { if (purging) await purge(purging); setPurging(null); }}
      />
    </>
  );
}

export default function CollectionTrashPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.trash" description="trash.subtitle">
      {(collection) => roleAllows(collection.role, "editor")
        ? <div className="space-y-4"><Trash collection={collection} /></div>
        : <EmptyState icon={Trash2} title={t("error.forbidden")} description={t("trash.editors_only")} />}
    </CollectionPage>
  );
}
