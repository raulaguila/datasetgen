"use client";

import { FormError } from "@/components/bits";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { collections } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import { collectionName } from "@/lib/validation";

/** Copies fields, description and icon (and the records when asked) into a new collection the caller owns. */
export function DuplicateCollectionDialog({ source, onClose }: { source: Collection; onClose: () => void }) {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const [name, setName] = useState(t("duplicate.default_name").replace("{name}", source.name));
  const [withRecords, setWithRecords] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const rule = collectionName(name);

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const created = await collections.duplicate(source.id, name.trim(), withRecords);
      toast.success(t("duplicate.done"));
      void client.invalidateQueries({ queryKey: ["collections"] });
      onClose();
      router.push(`/collections/${created.id}/overview`);
    } catch (err) {
      setError(errorMessage(err, "common.save_failed", t));
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("duplicate.title")}</DialogTitle>
          <DialogDescription>{t("duplicate.desc")}</DialogDescription>
        </DialogHeader>
        <form id="dup-form" className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (!rule) void submit(); }}>
          <div className="space-y-2">
            <Label htmlFor="dup-name">{t("auth.name")}</Label>
            <Input id="dup-name" autoFocus maxLength={120} aria-invalid={!!rule} value={name} onChange={(e) => setName(e.target.value)} />
            {rule && <p className="text-xs text-destructive">{t(rule)}</p>}
          </div>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={withRecords} onCheckedChange={(v) => setWithRecords(v === true)} />
            {plural(t("duplicate.records"), source.record_count ?? 0)}
          </label>
          <FormError message={error} />
        </form>
        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={onClose}>{t("common.cancel")}</Button>
          <Button type="submit" form="dup-form" disabled={busy || !!rule}>{busy && <Loader2 className="size-4 animate-spin" />}{t("duplicate.submit")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
