"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Maximize2, Minimize2, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { IconAction } from "@/components/bits";
import { WIDGET_KINDS, WidgetView, fieldsFor } from "@/components/collection-stats";
import { fieldsOf } from "@/components/field-builder";
import { RecordRules, type Where } from "@/components/record-rules";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { tools } from "@/lib/api/endpoints";
import type { Collection, Widget } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";

const MAX = 24;

/** The form of one widget: what it shows, over which field and records. */
function WidgetDialog({ collection, initial, onClose, onSave }: { collection: Collection; initial: Widget | null; onClose: () => void; onSave: (w: Widget) => void }) {
  const { t } = useI18n();
  const rows = fieldsOf(collection.schema ?? null);
  const [kind, setKind] = useState<Widget["kind"]>(initial?.kind ?? "count");
  const [title, setTitle] = useState(initial?.title ?? "");
  const [field, setField] = useState(initial?.field ?? "");
  const [where, setWhere] = useState<Where>((initial?.where as Where | undefined) ?? { any: false, rules: [] });
  const fit = fieldsFor(kind, rows);
  const picked = fit?.find((r) => r.key === field);
  const kinds = Object.fromEntries(WIDGET_KINDS.map((k) => [k, t(`dashboard.kind.${k}` as MessageKey)]));
  // A blank title names the widget after what it shows.
  const auto = `${kinds[kind]}${picked ? `: ${picked.field.name}` : ""}`;
  const ready = !fit || !!picked;
  return (
    // The conditions popover sits outside the dialog: a click in it must not close the dialog.
    <Dialog open disablePointerDismissal onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader><DialogTitle>{t(initial ? "dashboard.edit_widget" : "dashboard.add_widget")}</DialogTitle></DialogHeader>
        <form className="space-y-4" onSubmit={(e) => {
          e.preventDefault();
          if (!ready) return;
          onSave({ id: initial?.id ?? crypto.randomUUID(), kind, title: title.trim() || auto, ...(fit ? { field } : {}), ...(where.rules.length || where.groups?.length ? { where } : {}), size: initial?.size ?? (kind === "bars" || kind === "list" ? 2 : 1) });
        }}>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="w-kind">{t("dashboard.kind")}</Label>
              <Select value={kind} items={kinds} onValueChange={(v) => { setKind(v as Widget["kind"]); setField(""); }}>
                <SelectTrigger id="w-kind" className="w-full"><SelectValue /></SelectTrigger>
                <SelectContent>{WIDGET_KINDS.map((k) => <SelectItem key={k} value={k}>{kinds[k]}</SelectItem>)}</SelectContent>
              </Select>
            </div>
            {fit && (
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="w-field">{t("dashboard.field")}</Label>
                <Select value={field || null} items={Object.fromEntries(fit.map((r) => [r.key, r.field.name]))} onValueChange={(v) => setField(String(v ?? ""))}>
                  <SelectTrigger id="w-field" className="w-full"><SelectValue placeholder={t(fit.length ? "fix.pick" : "dashboard.no_fields")} /></SelectTrigger>
                  <SelectContent>{fit.map((r) => <SelectItem key={r.key} value={r.key}>{r.field.name}</SelectItem>)}</SelectContent>
                </Select>
              </div>
            )}
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="w-title">{t("dashboard.title_label")}</Label>
            <Input id="w-title" maxLength={80} value={title} placeholder={auto} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <p className="text-sm font-medium">{t("dashboard.records")}</p>
            <div className="flex flex-wrap items-center gap-2">
              <RecordRules fields={rows} value={where} onChange={setWhere} />
              {!where.rules.length && !where.groups?.length && <span className="text-xs text-muted-foreground">{t("dashboard.all_records")}</span>}
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
            <Button type="submit" disabled={!ready}>{t(initial ? "common.save" : "dashboard.add_widget")}</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * The collection's own dashboard: counts, sums, bars and lists the owners pick,
 * each over the records its conditions select. Every member sees it.
 */
export function CollectionDashboard({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const owner = roleAllows(collection.role, "owner");
  const key = ["collections", "dashboard", collection.id];
  const q = useQuery({ queryKey: key, queryFn: ({ signal }) => tools.dashboard(collection.id, signal) });
  const [draft, setDraft] = useState<Widget[] | null>(null);
  const [editing, setEditing] = useState<Widget | "new" | null>(null);
  const save = useMutation({
    mutationFn: (widgets: Widget[]) => tools.saveDashboard(collection.id, widgets),
    onSuccess: (_, widgets) => { client.setQueryData(key, { widgets }); setDraft(null); toast.success(t("common.saved")); },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const rows = fieldsOf(collection.schema ?? null);
  const widgets = draft ?? q.data?.widgets ?? [];
  if (q.isPending || q.isError) return null;
  if (!widgets.length && !draft && !owner) return null;

  const move = (i: number, d: -1 | 1) => setDraft((w) => { const next = [...(w ?? [])]; [next[i], next[i + d]] = [next[i + d], next[i]]; return next; });
  const put = (w: Widget) => {
    setDraft((list) => { const cur = list ?? widgets; return cur.some((x) => x.id === w.id) ? cur.map((x) => (x.id === w.id ? w : x)) : [...cur, w]; });
    setEditing(null);
  };
  return (
    <section aria-label={t("dashboard.title")} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-heading text-base font-semibold">{t("dashboard.title")}</h2>
        {owner && (draft ? (
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={() => setDraft(null)}>{t("common.cancel")}</Button>
            <Button variant="outline" size="sm" disabled={widgets.length >= MAX} onClick={() => setEditing("new")}><Plus className="size-4" />{t("dashboard.add_widget")}</Button>
            <Button size="sm" disabled={save.isPending} onClick={() => save.mutate(widgets)}>{t("common.save")}</Button>
          </div>
        ) : (
          <Button variant="outline" size="sm" onClick={() => setDraft(widgets)}><Pencil className="size-4" />{t("dashboard.edit")}</Button>
        ))}
      </div>
      {widgets.length === 0 ? (
        <button type="button" className="w-full rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground hover:bg-muted/40" onClick={() => { setDraft([]); setEditing("new"); }}>
          <Plus className="mx-auto mb-1 size-5" />{t("dashboard.empty")}
        </button>
      ) : (
        <ul className="grid auto-rows-fr gap-3 sm:grid-cols-2 xl:grid-cols-4">
          {widgets.map((w, i) => (
            <li key={w.id} className={w.size === 2 ? "relative sm:col-span-2" : "relative"}>
              <WidgetView collection={collection} widget={w} rows={rows} />
              {draft && (
                <div className="absolute right-2 top-2 flex gap-0.5 rounded-md border bg-popover p-0.5 shadow-sm">
                  <IconAction label={t("dashboard.move_up")} target={w.title} disabled={i === 0} onClick={() => move(i, -1)}><ArrowUp className="size-4" /></IconAction>
                  <IconAction label={t("dashboard.move_down")} target={w.title} disabled={i === widgets.length - 1} onClick={() => move(i, 1)}><ArrowDown className="size-4" /></IconAction>
                  <IconAction label={t(w.size === 2 ? "dashboard.narrow" : "dashboard.wide")} target={w.title} onClick={() => put({ ...w, size: w.size === 2 ? 1 : 2 })}>{w.size === 2 ? <Minimize2 className="size-4" /> : <Maximize2 className="size-4" />}</IconAction>
                  <IconAction label={t("dashboard.edit_widget")} target={w.title} onClick={() => setEditing(w)}><Pencil className="size-4" /></IconAction>
                  <IconAction label={t("dashboard.remove")} target={w.title} className="text-destructive" onClick={() => setDraft((list) => (list ?? []).filter((x) => x.id !== w.id))}><Trash2 className="size-4" /></IconAction>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      {editing && <WidgetDialog collection={collection} initial={editing === "new" ? null : editing} onClose={() => setEditing(null)} onSave={put} />}
    </section>
  );
}
