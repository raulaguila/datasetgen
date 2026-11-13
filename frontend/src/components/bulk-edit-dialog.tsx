"use client";

import dynamic from "next/dynamic";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { FormError } from "@/components/bits";
import { fieldsOf } from "@/components/field-builder";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { records } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";

const SchemaForm = dynamic(() => import("@/components/schema-form").then((m) => m.SchemaForm), { ssr: false, loading: () => <ListSkeleton rows={1} /> });

/**
 * Sets one field to the same value on the selected records, or clears it. The
 * server saves all or none: one record the value breaks refuses the whole change.
 */
export function BulkEditDialog({ collection, ids, open, onOpenChange, onDone }: {
  collection: Collection;
  ids: string[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDone: () => void;
}) {
  const { t } = useI18n();
  const client = useQueryClient();
  // A unique value cannot repeat across records, so those fields only take one record at a time.
  const fields = fieldsOf(collection.schema ?? null).filter((r) => !r.advanced && (ids.length === 1 || !r.field.unique));
  const [key, setKey] = useState<string>("");
  const [value, setValue] = useState<Record<string, unknown>>({});
  const [clear, setClear] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const row = fields.find((r) => r.key === key);
  const props = (collection.schema?.properties ?? {}) as Record<string, unknown>;
  const reset = () => { setKey(""); setValue({}); setClear(false); setError(null); };
  const ready = !!row && (clear || value[key] !== undefined);

  const save = async () => {
    if (!row) return;
    setSaving(true);
    setError(null);
    try {
      const res = await records.bulkUpdate(ids, { [key]: clear ? null : value[key] });
      toast.success(t("bulk.done").replace("{n}", String(res.count)));
      void client.invalidateQueries({ queryKey: ["records", collection.id] });
      reset();
      onOpenChange(false);
      onDone();
    } catch (err) {
      setError(errorMessage(err, "common.save_failed", t));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => { if (saving) return; if (!o) reset(); onOpenChange(o); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("bulk.title")}</DialogTitle>
          <DialogDescription>{t("bulk.desc").replace("{n}", String(ids.length))}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="bulk-field">{t("bulk.field")}</Label>
            <Select value={key || null} items={Object.fromEntries(fields.map((r) => [r.key, r.field.name]))} onValueChange={(v) => { setKey(String(v ?? "")); setValue({}); setClear(false); }}>
              <SelectTrigger id="bulk-field" className="w-full"><SelectValue placeholder={t("bulk.pick")} /></SelectTrigger>
              <SelectContent>
                {fields.map((r) => <SelectItem key={r.key} value={r.key}>{r.field.name}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          {row && (
            <>
              {!clear && (
                <div className="rounded-lg border bg-muted/30 p-3">
                  <SchemaForm key={key} schema={{ type: "object", properties: { [key]: props[key] } }} collectionId={collection.id} data={value} onChange={(d) => setValue((d ?? {}) as Record<string, unknown>)} idPrefix="bulk" />
                </div>
              )}
              {!row.field.required && (
                <label className="flex items-center gap-2 text-sm">
                  <Checkbox checked={clear} onCheckedChange={(on) => setClear(on === true)} />
                  {t("bulk.clear")}
                </label>
              )}
            </>
          )}
          <FormError message={error} />
        </div>
        <DialogFooter>
          <Button variant="outline" disabled={saving} onClick={() => onOpenChange(false)}>{t("common.cancel")}</Button>
          <Button disabled={!ready || saving} onClick={() => void save()}>{t("bulk.save").replace("{n}", String(ids.length))}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
