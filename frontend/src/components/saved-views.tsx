"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bookmark, Trash2, Users } from "lucide-react";
import { toast } from "sonner";
import { FormError, IconAction } from "@/components/bits";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { tools } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";

/** The list's query string to keep in a view: the page number is left out. */
function currentQuery(): string {
  const q = new URLSearchParams(window.location.search);
  q.delete("page");
  return q.toString();
}

/**
 * Named record list queries (search, filters, sort): the user's own, and those
 * an editor shared with every member.
 */
export function SavedViews({ collectionId, canShare, onApply }: { collectionId: string; canShare: boolean; onApply: (query: string) => void }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [shared, setShared] = useState(false);
  const key = ["views", collectionId];
  const list = useQuery({ queryKey: key, queryFn: () => tools.views(collectionId), enabled: open });
  const create = useMutation({
    mutationFn: () => tools.createView(collectionId, { name: name.trim(), query: currentQuery(), shared }),
    onSuccess: () => { setName(""); setShared(false); toast.success(t("views.saved")); void client.invalidateQueries({ queryKey: key }); },
  });
  const remove = useMutation({
    mutationFn: (viewId: string) => tools.deleteView(collectionId, viewId),
    onSuccess: () => void client.invalidateQueries({ queryKey: key }),
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const items = list.data?.items ?? [];

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button variant="outline" />}><Bookmark className="size-4" />{t("views.title")}</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("views.title")}</DialogTitle>
          <DialogDescription>{t("views.desc")}</DialogDescription>
        </DialogHeader>
        {items.length === 0 ? (
          <p className="rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">{list.isPending ? t("views.loading") : t("views.empty")}</p>
        ) : (
          <ul className="max-h-64 divide-y overflow-auto rounded-md border">
            {items.map((v) => (
              <li key={v.id} className="flex items-center gap-2 px-2 py-1.5">
                <button type="button" className="min-w-0 flex-1 truncate rounded px-1 py-1 text-left text-sm font-medium hover:underline" onClick={() => { onApply(v.query); setOpen(false); }}>
                  {v.name}
                </button>
                {v.shared && <Users className="size-4 shrink-0 text-muted-foreground" role="img" aria-label={t("views.shared")}><title>{t("views.shared")}</title></Users>}
                {v.mine && <IconAction label={t("common.delete")} target={v.name} className="text-destructive" onClick={() => remove.mutate(v.id)}><Trash2 className="size-4" /></IconAction>}
              </li>
            ))}
          </ul>
        )}
        <form className="space-y-2 border-t pt-4" onSubmit={(e) => { e.preventDefault(); if (name.trim()) create.mutate(); }}>
          <Label htmlFor="view-name">{t("views.save_current")}</Label>
          <div className="flex gap-2">
            <Input id="view-name" maxLength={80} placeholder={t("views.name")} value={name} onChange={(e) => setName(e.target.value)} />
            <Button type="submit" disabled={!name.trim() || create.isPending}>{t("common.save")}</Button>
          </div>
          {canShare && (
            <label className="flex items-center gap-2 text-sm">
              <Checkbox checked={shared} onCheckedChange={(on) => setShared(on === true)} />
              {t("views.share")}
            </label>
          )}
          <FormError message={create.isError ? errorMessage(create.error, "common.save_failed", t) : null} />
        </form>
      </DialogContent>
    </Dialog>
  );
}
