"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Play, Plus, Trash2, Zap } from "lucide-react";
import { toast } from "sonner";
import { FormError, FormSection, FormSections, IconAction, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { fieldsOf, type FieldRow } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { ViewConditions } from "@/components/collection-stats";
import { RecordRules, whereEmpty, type Where } from "@/components/record-rules";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { tools } from "@/lib/api/endpoints";
import type { Automation, AutomationAction, AutomationInput, AutomationTrigger, Collection } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

const TRIGGERS: AutomationTrigger[] = ["record.created", "record.updated", "daily"];
const ACTIONS: AutomationAction[] = ["set_field", "notify"];
const HOURS = Array.from({ length: 24 }, (_, h) => String(h));
const settable = (rows: FieldRow[]) => rows.filter((r) => !r.advanced && ["string", "email", "url", "number", "integer", "boolean", "enum", "date"].includes(r.field.type));

const blank = (): AutomationInput => ({ name: "", enabled: true, trigger: "record.created", conditions: { any: false, rules: [] }, action: "notify", config: { message: "" } });

/** The value typed for a set_field action, in the field's type; "" clears the field. */
function typed(row: FieldRow | undefined, raw: string): unknown {
  if (raw === "") return null;
  if (row?.field.type === "number" || row?.field.type === "integer") return Number(raw);
  if (row?.field.type === "boolean") return raw === "true";
  return raw;
}

function AutomationForm({ collection, initial, onDone }: { collection: Collection; initial?: Automation; onDone: () => void }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const rows = fieldsOf(collection.schema ?? null);
  const fields = settable(rows);
  const [draft, setDraft] = useState<AutomationInput>(() => (initial ? { name: initial.name, enabled: initial.enabled, trigger: initial.trigger, hour: initial.hour, conditions: initial.conditions, action: initial.action, config: initial.config } : blank()));
  const [raw, setRaw] = useState(() => (initial?.config.value === undefined || initial.config.value === null ? "" : String(initial.config.value)));
  const set = (patch: Partial<AutomationInput>) => setDraft((d) => ({ ...d, ...patch }));
  const target = fields.find((f) => f.key === draft.config.field);
  const save = useMutation({
    mutationFn: () => {
      const input: AutomationInput = { ...draft, hour: draft.trigger === "daily" ? (draft.hour ?? 8) : undefined, config: draft.action === "set_field" ? { field: draft.config.field, value: typed(target, raw) } : { message: draft.config.message } };
      return initial ? tools.updateAutomation(collection.id, initial.id, input) : tools.createAutomation(collection.id, input);
    },
    onSuccess: () => { void client.invalidateQueries({ queryKey: ["automations", collection.id] }); toast.success(t("common.saved")); onDone(); },
  });
  const ready = draft.name.trim() && (draft.action === "notify" ? !!draft.config.message?.trim() : !!draft.config.field);
  const valueInput = () => {
    if (target?.field.type === "boolean" || target?.field.type === "enum") {
      const options = target.field.type === "boolean" ? [["true", t("common.yes")], ["false", t("common.no")]] : target.field.options.split(",").map((o) => o.trim()).filter(Boolean).map((o) => [o, o]);
      return (
        <Select value={raw || null} items={Object.fromEntries(options)} onValueChange={(v) => setRaw(String(v ?? ""))}>
          <SelectTrigger id="auto-value" className="w-full"><SelectValue placeholder={t("automations.value")} /></SelectTrigger>
          <SelectContent>{options.map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}</SelectContent>
        </Select>
      );
    }
    const type = target?.field.type === "number" || target?.field.type === "integer" ? "number" : target?.field.type === "date" ? "date" : "text";
    return <Input id="auto-value" type={type} value={raw} placeholder={t("automations.value_clear")} onChange={(e) => setRaw(e.target.value)} />;
  };
  return (
    <form className="space-y-4 rounded-md border p-4" aria-label={t(initial ? "automations.edit" : "automations.new")} onSubmit={(e) => { e.preventDefault(); if (ready) save.mutate(); }}>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="auto-name">{t("automations.name")}</Label>
        <Input id="auto-name" maxLength={80} value={draft.name} onChange={(e) => set({ name: e.target.value })} />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="auto-trigger">{t("automations.when")}</Label>
          <Select value={draft.trigger} items={Object.fromEntries(TRIGGERS.map((x) => [x, t(`automations.trigger.${x}` as MessageKey)]))} onValueChange={(v) => set({ trigger: v as AutomationTrigger })}>
            <SelectTrigger id="auto-trigger" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>{TRIGGERS.map((x) => <SelectItem key={x} value={x}>{t(`automations.trigger.${x}` as MessageKey)}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        {draft.trigger === "daily" && (
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auto-hour">{t("automations.hour")}</Label>
            <Select value={String(draft.hour ?? 8)} items={Object.fromEntries(HOURS.map((h) => [h, `${h.padStart(2, "0")}:00`]))} onValueChange={(v) => set({ hour: Number(v) })}>
              <SelectTrigger id="auto-hour" className="w-full"><SelectValue /></SelectTrigger>
              <SelectContent>{HOURS.map((h) => <SelectItem key={h} value={h}>{`${h.padStart(2, "0")}:00`}</SelectItem>)}</SelectContent>
            </Select>
          </div>
        )}
      </div>
      <div className="space-y-1.5">
        <p className="text-sm font-medium">{t("automations.if")}</p>
        <div className="flex flex-wrap items-center gap-2">
          <RecordRules fields={rows} value={draft.conditions as Where} onChange={(w) => set({ conditions: w })} />
          <ViewConditions collectionId={collection.id} onPick={(w) => set({ conditions: w })} />
          {whereEmpty(draft.conditions as Where) && <span className="text-xs text-muted-foreground">{t("automations.every_record")}</span>}
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="auto-action">{t("automations.then")}</Label>
          <Select value={draft.action} items={Object.fromEntries(ACTIONS.map((x) => [x, t(`automations.action.${x}` as MessageKey)]))} onValueChange={(v) => set({ action: v as AutomationAction, config: v === "notify" ? { message: "" } : { field: fields[0]?.key } })}>
            <SelectTrigger id="auto-action" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>{ACTIONS.map((x) => <SelectItem key={x} value={x}>{t(`automations.action.${x}` as MessageKey)}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        {draft.action === "set_field" && (
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auto-field">{t("automations.field")}</Label>
            <Select value={draft.config.field ?? null} items={Object.fromEntries(fields.map((f) => [f.key, f.field.name]))} onValueChange={(v) => { set({ config: { field: String(v) } }); setRaw(""); }}>
              <SelectTrigger id="auto-field" className="w-full"><SelectValue placeholder={t("automations.field")} /></SelectTrigger>
              <SelectContent>{fields.map((f) => <SelectItem key={f.key} value={f.key}>{f.field.name}</SelectItem>)}</SelectContent>
            </Select>
          </div>
        )}
      </div>
      {draft.action === "set_field" ? (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="auto-value">{t("automations.value")}</Label>
          {valueInput()}
        </div>
      ) : (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="auto-message">{t("automations.message")}</Label>
          <Textarea id="auto-message" maxLength={500} rows={2} value={draft.config.message ?? ""} onChange={(e) => set({ config: { message: e.target.value } })} />
        </div>
      )}
      <FormError message={save.isError ? errorMessage(save.error, "common.save_failed", t) : null} />
      <div className="flex gap-2">
        <Button type="submit" disabled={!ready || save.isPending}>{t("common.save")}</Button>
        <Button type="button" variant="ghost" onClick={onDone}>{t("common.cancel")}</Button>
      </div>
    </form>
  );
}

function Automations({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["automations", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => tools.automations(collection.id) });
  const [editing, setEditing] = useState<Automation | "new" | null>(null);
  const [removing, setRemoving] = useState<Automation | null>(null);
  const names = new Map(fieldsOf(collection.schema ?? null).map((r) => [r.key, r.field.name]));
  const refresh = () => void client.invalidateQueries({ queryKey: key });
  const toggle = useMutation({
    mutationFn: (a: Automation) => tools.updateAutomation(collection.id, a.id, { name: a.name, enabled: !a.enabled, trigger: a.trigger, hour: a.hour, conditions: a.conditions, action: a.action, config: a.config }),
    onSuccess: refresh,
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const run = useMutation({
    mutationFn: (a: Automation) => tools.runAutomation(collection.id, a.id),
    onSuccess: (a) => { refresh(); if (a.last_error) toast.error(a.last_error); else toast.success(t("automations.ran")); },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const summary = (a: Automation) => {
    const when = a.trigger === "daily" ? t("automations.daily_at").replace("{hour}", `${String(a.hour ?? 0).padStart(2, "0")}:00`) : t(`automations.trigger.${a.trigger}` as MessageKey);
    const then = a.action === "notify" ? t("automations.action.notify") : t("automations.sets").replace("{field}", names.get(a.config.field ?? "") ?? a.config.field ?? "").replace("{value}", a.config.value === null || a.config.value === undefined ? "—" : String(a.config.value));
    const n = a.conditions.rules.length + ((a.conditions as Where).groups ?? []).reduce((k, g) => k + g.rules.length, 0);
    const rules = n ? ` · ${plural(t("automations.n_conditions"), n)}` : "";
    return `${when}${rules} → ${then}`;
  };
  const items = list.data?.items ?? [];
  return (
    <FormSection title={t("automations.title")} description={t("automations.desc")}>
      <div className="space-y-3">
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 && editing !== "new" ? (
          <p className="rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">{t("automations.empty")}</p>
        ) : (
          <ul className="divide-y rounded-md border" aria-label={t("automations.title")}>
            {items.map((a) => editing !== "new" && editing?.id === a.id ? (
              <li key={a.id} className="p-2"><AutomationForm collection={collection} initial={a} onDone={() => setEditing(null)} /></li>
            ) : (
              <li key={a.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
                <Switch checked={a.enabled} aria-label={`${t("automations.enabled")}: ${a.name}`} disabled={toggle.isPending} onCheckedChange={() => toggle.mutate(a)} />
                <div className="min-w-48 flex-1">
                  <p className="flex items-center gap-2 text-sm font-medium">{a.name}{!a.enabled && <Badge variant="outline">{t("automations.paused")}</Badge>}</p>
                  <p className="text-xs text-muted-foreground">{summary(a)}</p>
                  <p className={a.last_error ? "text-xs text-destructive" : "text-xs text-muted-foreground"}>
                    {a.last_run_at ? `${t("automations.last_run").replace("{at}", format(a.last_run_at)).replace("{n}", String(a.runs))}${a.last_error ? ` · ${a.last_error}` : ""}` : t("automations.never_ran")}
                  </p>
                </div>
                <Button size="sm" variant="outline" disabled={run.isPending} onClick={() => run.mutate(a)}><Play className="size-4" />{t("automations.run")}</Button>
                <IconAction label={t("common.edit")} target={a.name} onClick={() => setEditing(a)}><Pencil className="size-4" /></IconAction>
                <IconAction label={t("common.delete")} target={a.name} className="text-destructive" onClick={() => setRemoving(a)}><Trash2 className="size-4" /></IconAction>
              </li>
            ))}
          </ul>
        )}
        {editing === "new" ? <AutomationForm collection={collection} onDone={() => setEditing(null)} /> : (
          <Button variant="outline" disabled={items.length >= 20} onClick={() => setEditing("new")}><Plus className="size-4" />{t("automations.new")}</Button>
        )}
      </div>
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("automations.delete_title").replace("{name}", removing?.name ?? "")}
        description={t("automations.delete_desc")}
        confirmLabel={t("common.delete")}
        destructive
        onConfirm={async () => {
          if (!removing) return;
          await tools.deleteAutomation(collection.id, removing.id).then(refresh, (err) => toast.error(errorMessage(err, "common.save_failed", t)));
          setRemoving(null);
        }}
      />
    </FormSection>
  );
}

export default function CollectionAutomationsPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.automations" description="automations.subtitle">
      {(collection) => roleAllows(collection.role, "owner") ? (
        <FormSections><Automations collection={collection} /></FormSections>
      ) : <EmptyState icon={Zap} title={t("error.forbidden")} description={t("integrations.owners_only")} />}
    </CollectionPage>
  );
}
