"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Loader2 } from "lucide-react";
import { FieldNote, FormError } from "@/components/bits";
import { CollectionAvatar } from "@/components/collection-avatar";
import { IconPicker } from "@/components/icon-picker";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ApiError } from "@/lib/api/client";
import { collections } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { collectionName, description as descriptionRule } from "@/lib/validation";

/** Name, description and icon of a collection; its fields have their own page. */
export function EditCollectionDialog({ collection, onClose }: { collection: Collection; onClose: () => void }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const [name, setName] = useState(collection.name);
  const [desc, setDesc] = useState(collection.description);
  const [icon, setIcon] = useState(collection.icon);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const rule = collectionName(name);
  const changed = name.trim() !== collection.name || desc.trim() !== collection.description || icon !== collection.icon;

  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const saved = await collections.update(collection, { name: name.trim(), description: desc.trim(), icon: icon !== collection.icon ? icon : undefined, version: collection.version });
      client.setQueryData(["collections", "detail", saved.id], saved);
      void client.invalidateQueries({ queryKey: ["collections"] });
      toast.success(t("common.saved"));
      onClose();
    } catch (err) {
      setError(errorMessage(err, "common.save_failed", t));
      // Someone else saved first: reload so the next try carries their version.
      if (err instanceof ApiError && err.code === "CONFLICT") void client.invalidateQueries({ queryKey: ["collections"] });
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("collections.edit")}</DialogTitle>
        </DialogHeader>
        <form id="edit-collection" className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (!rule && changed) void submit(); }}>
          <div className="space-y-2">
            <Label htmlFor="ec-name">{t("auth.name")}</Label>
            <Input id="ec-name" autoFocus required maxLength={120} aria-invalid={!!rule} aria-describedby="ec-name-note" value={name} onChange={(e) => setName(e.target.value)} />
            <FieldNote id="ec-name-note" error={rule && t(rule)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="ec-desc">{t("collections.description")}</Label>
            <Textarea id="ec-desc" rows={3} maxLength={2000} aria-invalid={!!descriptionRule(desc)} value={desc} onChange={(e) => setDesc(e.target.value)} />
          </div>
          <div className="space-y-2">
            <IconPicker value={icon} onChange={setIcon} label={t("collections.icon")} noneLabel={t("collections.icon_none")} />
            <p className="flex items-center gap-2 text-xs text-muted-foreground">
              {t("collections.icon_desc")}
              <CollectionAvatar collection={{ id: collection.id, name: name.trim() || "?", icon }} className="size-6 rounded-md text-xs font-semibold" />
            </p>
          </div>
          <FormError message={error} />
        </form>
        <DialogFooter>
          <Button variant="ghost" disabled={busy} onClick={onClose}>{t("common.cancel")}</Button>
          <Button type="submit" form="edit-collection" disabled={busy || !!rule || !!descriptionRule(desc) || !changed}>{busy && <Loader2 className="size-4 animate-spin" />}{t("common.save")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
