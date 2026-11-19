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
import { collections, tools } from "@/lib/api/endpoints";
import type { Automation, AutomationAction, AutomationInput, AutomationTrigger, Collection } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

const TRIGGERS: AutomationTrigger[] = ["record.created", "record.updated", "daily"];
const ACTIONS: AutomationAction[] = ["set_field", "notify", "email", "webhook", "create_record"];
const HOURS = Array.from({ length: 24 }, (_, h) => String(h));
const settable = (rows: FieldRow[]) => rows.filter((r) => !r.advanced && ["string", "email", "url", "number", "integer", "boolean", "enum", "date"].includes(r.field.type));

const blank = (): AutomationInput => ({ name: "", enabled: true, trigger: "record.created", conditions: { any: false, rules: [] }, action: "notify", config: { message: "" } });

/** The config each action starts with. */
function startConfig(action: AutomationAction, fields: FieldRow[]): AutomationInput["config"] {
  switch (action) {
    case "set_field": return { field: fields[0]?.key };
    case "email": return { to: "me", subject: "", message: "" };
    case "webhook": return { webhook_id: undefined };
    case "create_record": return { collection_id: undefined, data: {} };
    default: return { message: "" };
  }
}

/** Whether the action's settings are filled in enough to save. */
function configReady(action: AutomationAction, c: AutomationInput["config"]): boolean {
  switch (action) {
    case "set_field": return !!c.field;
    case "email": return !!c.subject?.trim() && !!c.message?.trim();
    case "webhook": return !!c.webhook_id;
    case "create_record": return !!c.collection_id && Object.keys(c.data ?? {}).length > 0;
    default: return !!c.message?.trim();
  }
}

const FIXED = "\u0000fixed"; // the mapping choice "a fixed value" (no field key can hold a NUL)
const RECORD_ID = "{{id}}";

/**
 * create_record settings: the target collection, then for each of its fields
 * nothing, a fixed value, the matching record's id, or one of its fields.
 */
function CreateRecordConfig({ sources, value, onChange }: { sources: FieldRow[]; value: AutomationInput["config"]; onChange: (c: AutomationInput["config"]) => void }) {
  const { t } = useI18n();
  const options = useQuery({ queryKey: ["collections", "options"], queryFn: ({ signal }) => collections.options(signal) });
  const writable = (options.data?.items ?? []).filter((c) => roleAllows(c.role, "editor"));
  const target = useQuery({ queryKey: ["collection", value.collection_id], queryFn: () => collections.byID(value.collection_id!), enabled: !!value.collection_id });
  const fields = fieldsOf(target.data?.schema ?? null).filter((r) => !r.advanced);
  const data = value.data ?? {};
  const choice = (key: string) => {
    const v = data[key];
    if (v === undefined) return "";
    if (typeof v === "string" && /^\{\{.+\}\}$/.test(v)) return v;
    return FIXED;
  };
  const setData = (key: string, v: unknown) => {
    const next = { ...data };
    if (v === undefined) delete next[key];
    else next[key] = v;
    onChange({ ...value, data: next });
  };
  const sourceItems: [string, string][] = [["", t("automations.map.none")], [FIXED, t("automations.map.fixed")], [RECORD_ID, t("automations.map.record_id")], ...sources.map((s): [string, string] => [`{{${s.key}}}`, s.field.name])];
  return (
    <div className="space-y-3">
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="auto-target">{t("automations.target")}</Label>
        <Select value={value.collection_id ?? null} items={Object.fromEntries(writable.map((c) => [c.id, c.name]))} onValueChange={(v) => onChange({ collection_id: String(v), data: {} })}>
          <SelectTrigger id="auto-target" className="w-full"><SelectValue placeholder={t("automations.target")} /></SelectTrigger>
          <SelectContent>{writable.map((c) => <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>)}</SelectContent>
        </Select>
      </div>
      {value.collection_id && (
        <div className="space-y-2" role="group" aria-label={t("automations.map.title")}>
          <p className="text-sm font-medium">{t("automations.map.title")}</p>
          {fields.length === 0 && <p className="text-xs text-muted-foreground">{t("automations.map.no_fields")}</p>}
          {fields.map((f) => (
            <div key={f.key} className="grid items-center gap-2 sm:grid-cols-[minmax(8rem,1fr)_minmax(10rem,1fr)_minmax(10rem,1fr)]">
              <span className="truncate text-sm" id={`map-${f.key}`}>{f.field.name}</span>
              <Select value={choice(f.key)} items={Object.fromEntries(sourceItems)} onValueChange={(v) => setData(f.key, v === "" ? undefined : v === FIXED ? typed(f, "") ?? "" : v)}>
                <SelectTrigger aria-labelledby={`map-${f.key}`} className="w-full"><SelectValue /></SelectTrigger>
                <SelectContent>{sourceItems.map(([k, l]) => <SelectItem key={k} value={k}>{l}</SelectItem>)}</SelectContent>
              </Select>
              {choice(f.key) === FIXED ? (
                <Input aria-label={`${f.field.name}: ${t("automations.map.fixed")}`} type={f.field.type === "number" || f.field.type === "integer" ? "number" : f.field.type === "date" ? "date" : "text"}
                  value={data[f.key] === null || data[f.key] === undefined ? "" : String(data[f.key])} onChange={(e) => setData(f.key, typed(f, e.target.value) ?? "")} />
              ) : <span />}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

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
      const input: AutomationInput = { ...draft, hour: draft.trigger === "daily" ? (draft.hour ?? 8) : undefined, config: draft.action === "set_field" ? { field: draft.config.field, value: typed(target, raw) } : draft.config };
      return initial ? tools.updateAutomation(collection, initial.id, input) : tools.createAutomation(collection, input);
    },
    onSuccess: () => { void client.invalidateQueries({ queryKey: ["automations", collection.id] }); toast.success(t("common.saved")); onDone(); },
  });
  const ready = draft.name.trim() && configReady(draft.action, draft.config);
  const hooks = useQuery({ queryKey: ["webhooks", collection.id], queryFn: () => tools.webhooks(collection), enabled: draft.action === "webhook" });
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
          <RecordRules collection={collection} fields={rows} value={draft.conditions as Where} onChange={(w) => set({ conditions: w })} />
          <ViewConditions collection={collection} onPick={(w) => set({ conditions: w })} />
          {whereEmpty(draft.conditions as Where) && <span className="text-xs text-muted-foreground">{t("automations.every_record")}</span>}
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="auto-action">{t("automations.then")}</Label>
          <Select value={draft.action} items={Object.fromEntries(ACTIONS.map((x) => [x, t(`automations.action.${x}` as MessageKey)]))} onValueChange={(v) => set({ action: v as AutomationAction, config: startConfig(v as AutomationAction, fields) })}>
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
        {draft.action === "email" && (
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auto-to">{t("automations.to")}</Label>
            <Select value={draft.config.to ?? "me"} items={{ me: t("automations.to.me"), members: t("automations.to.members") }} onValueChange={(v) => set({ config: { ...draft.config, to: v as "me" | "members" } })}>
              <SelectTrigger id="auto-to" className="w-full"><SelectValue /></SelectTrigger>
              <SelectContent><SelectItem value="me">{t("automations.to.me")}</SelectItem><SelectItem value="members">{t("automations.to.members")}</SelectItem></SelectContent>
            </Select>
          </div>
        )}
        {draft.action === "webhook" && (
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auto-hook">{t("automations.webhook")}</Label>
            <Select value={draft.config.webhook_id ?? null} items={Object.fromEntries((hooks.data?.items ?? []).map((h) => [h.id, h.url]))} onValueChange={(v) => set({ config: { webhook_id: String(v) } })}>
              <SelectTrigger id="auto-hook" className="w-full"><SelectValue placeholder={t("automations.webhook")} /></SelectTrigger>
              <SelectContent>{(hooks.data?.items ?? []).map((h) => <SelectItem key={h.id} value={h.id}>{h.url}</SelectItem>)}</SelectContent>
            </Select>
            {hooks.data && hooks.data.items.length === 0 && <p className="text-xs text-muted-foreground">{t("automations.webhook_none")}</p>}
          </div>
        )}
      </div>
      {draft.action === "set_field" ? (
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="auto-value">{t("automations.value")}</Label>
          {valueInput()}
        </div>
      ) : draft.action === "email" ? (
        <>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auto-subject">{t("automations.subject")}</Label>
            <Input id="auto-subject" maxLength={80} value={draft.config.subject ?? ""} onChange={(e) => set({ config: { ...draft.config, subject: e.target.value } })} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auto-message">{t("automations.email_message")}</Label>
            <Textarea id="auto-message" maxLength={500} rows={3} value={draft.config.message ?? ""} onChange={(e) => set({ config: { ...draft.config, message: e.target.value } })} />
            <p className="text-xs text-muted-foreground">{t("automations.email_hint")}</p>
          </div>
        </>
      ) : draft.action === "webhook" ? (
        <p className="text-xs text-muted-foreground">{t("automations.webhook_hint")}</p>
      ) : draft.action === "create_record" ? (
        <CreateRecordConfig sources={rows.filter((r) => !r.advanced)} value={draft.config} onChange={(config) => set({ config })} />
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
  const list = useQuery({ queryKey: key, queryFn: () => tools.automations(collection) });
  const [editing, setEditing] = useState<Automation | "new" | null>(null);
  const [removing, setRemoving] = useState<Automation | null>(null);
  const names = new Map(fieldsOf(collection.schema ?? null).map((r) => [r.key, r.field.name]));
  const refresh = () => void client.invalidateQueries({ queryKey: key });
  const toggle = useMutation({
    mutationFn: (a: Automation) => tools.updateAutomation(collection, a.id, { name: a.name, enabled: !a.enabled, trigger: a.trigger, hour: a.hour, conditions: a.conditions, action: a.action, config: a.config }),
    onSuccess: refresh,
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const run = useMutation({
    mutationFn: (a: Automation) => tools.runAutomation(collection, a.id),
    onSuccess: (a) => { refresh(); if (a.last_error) toast.error(a.last_error); else toast.success(t("automations.ran")); },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const summary = (a: Automation) => {
    const when = a.trigger === "daily" ? t("automations.daily_at").replace("{hour}", `${String(a.hour ?? 0).padStart(2, "0")}:00`) : t(`automations.trigger.${a.trigger}` as MessageKey);
    const then = a.action === "set_field"
      ? t("automations.sets").replace("{field}", names.get(a.config.field ?? "") ?? a.config.field ?? "").replace("{value}", a.config.value === null || a.config.value === undefined ? "—" : String(a.config.value))
      : a.action === "email" ? `${t("automations.action.email")}: ${a.config.subject ?? ""}` : t(`automations.action.${a.action}` as MessageKey);
    const n = a.conditions.rules.length + ((a.conditions as Where).groups ?? []).reduce((k, g) => k + g.rules.length, 0);
    const rules = n ? ` · ${plural(t("automations.n_conditions"), n)}` : "";
    return `${when}${rules} → ${then}`;
  };
  const items = list.data?.items ?? [];
  return (
    <FormSection description={t("automations.desc")} action={editing !== "new" && <Button variant="outline" disabled={items.length >= 20} onClick={() => setEditing("new")}><Plus className="size-4" />{t("automations.new")}</Button>}>
      <div className="space-y-3">
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 && editing !== "new" ? (
          <EmptyState compact title={t("automations.empty")} />
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
        {editing === "new" && <AutomationForm collection={collection} onDone={() => setEditing(null)} />}
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
          await tools.deleteAutomation(collection, removing.id).then(refresh, (err) => toast.error(errorMessage(err, "common.save_failed", t)));
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
      ) : <EmptyState icon={Zap} title={t("error.forbidden_page")} description={t("integrations.owners_only")} />}
    </CollectionPage>
  );
}
