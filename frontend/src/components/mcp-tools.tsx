"use client";

import { useState } from "react";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2, X } from "lucide-react";
import { toast } from "sonner";
import { FormError, FormSection, IconAction, useFormatDate } from "@/components/bits";
import { ViewConditions } from "@/components/collection-stats";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { fieldsOf, type FieldRow } from "@/components/field-builder";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { RecordRules, type Where } from "@/components/record-rules";
import { CodeBlock } from "@/components/secret";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { tools } from "@/lib/api/endpoints";
import type { Collection, McpParamOp, McpTool, McpToolInput, McpToolParam } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";

const TEXT_OPS: McpParamOp[] = ["eq", "neq", "contains", "in"];
const ORDERED_OPS: McpParamOp[] = ["eq", "neq", "gt", "gte", "lt", "lte", "in"];
const NAME = /^[a-z][a-z0-9_]{2,47}$/;
const PARAM = /^[a-z][a-z0-9_]{0,39}$/;

/** The comparisons a parameter on this field may use; none when it cannot be one. */
export function opsFor(row: FieldRow): McpParamOp[] {
  const { type, result } = row.field;
  if (type === "formula") return result === "number" ? ORDERED_OPS : result === "boolean" ? ["eq"] : result === "date" ? ORDERED_OPS : TEXT_OPS;
  if (type === "boolean") return ["eq"];
  if (type === "strings" || type === "references") return ["has"];
  if (["number", "integer", "autonumber", "date", "datetime"].includes(type)) return ORDERED_OPS;
  if (["string", "text", "email", "url", "enum", "reference", "created_by", "updated_by"].includes(type)) return TEXT_OPS;
  return [];
}

/** A parameter name from a field key: lower case, a-z 0-9 _. */
const paramNameOf = (key: string) => key.toLowerCase().replace(/[^a-z0-9_]/g, "_").replace(/^[^a-z]+/, "").slice(0, 40) || "valor";

/** The JSON an assistant would send for a typed value: numbers and true/false by the field, a list for "in"; undefined when empty. */
export function argOf(row: FieldRow | undefined, op: McpParamOp, raw: string): unknown {
  const one = (v: string): unknown => {
    const type = row?.field.type === "formula" ? row.field.result : row?.field.type;
    if (["number", "integer", "autonumber"].includes(type ?? "")) return Number(v);
    if (type === "boolean") return v === "true";
    return v;
  };
  const text = raw.trim();
  if (!text) return undefined;
  return op === "in" ? text.split(",").map((v) => v.trim()).filter(Boolean).map(one) : one(text);
}

const blank = (): McpToolInput => ({ name: "", description: "", enabled: true, conditions: { any: false, rules: [] }, params: [], fields: [], sort: "updated_at", order: "desc", limit: 20 });

function ToolForm({ collection, initial, onDone }: { collection: Collection; initial?: McpTool; onDone: () => void }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const rows = fieldsOf(collection.schema ?? null);
  const paramFields = rows.filter((r) => opsFor(r).length > 0);
  const [draft, setDraft] = useState<McpToolInput>(() => (initial ? { name: initial.name, description: initial.description, enabled: initial.enabled, conditions: initial.conditions, params: initial.params, fields: initial.fields, sort: initial.sort, order: initial.order, limit: initial.limit } : blank()));
  const set = (patch: Partial<McpToolInput>) => setDraft((d) => ({ ...d, ...patch }));
  const setParam = (i: number, patch: Partial<McpToolParam>) => set({ params: draft.params.map((p, j) => (j === i ? { ...p, ...patch } : p)) });
  const save = useMutation({
    mutationFn: () => (initial ? tools.updateMcpTool(collection.id, initial.id, draft) : tools.createMcpTool(collection.id, draft)),
    onSuccess: () => { void client.invalidateQueries({ queryKey: ["mcp-tools", collection.id] }); toast.success(t("common.saved")); onDone(); },
  });
  // Trying the unsaved settings with the values an assistant would send.
  const [tryArgs, setTryArgs] = useState<Record<string, string>>({});
  const tryRun = useMutation({
    mutationFn: () => {
      const args: Record<string, unknown> = {};
      for (const p of draft.params) {
        const v = argOf(rows.find((r) => r.key === p.field), p.op, tryArgs[p.name] ?? "");
        if (v !== undefined) args[p.name] = v;
      }
      return tools.testMcpTool(collection.id, draft, args);
    },
  });
  const names = draft.params.map((p) => p.name);
  const paramsOk = draft.params.every((p, i) => PARAM.test(p.name) && p.name !== "page" && names.indexOf(p.name) === i && p.field);
  const ready = NAME.test(draft.name) && draft.description.trim() && paramsOk;
  const addParam = () => {
    const row = paramFields[0];
    if (row) set({ params: [...draft.params, { name: paramNameOf(row.key), field: row.key, op: opsFor(row)[0], required: false, description: "" }] });
  };
  const sorts: [string, string][] = [["updated_at", t("mcp_tools.sort_updated")], ["created_at", t("mcp_tools.sort_created")], ...rows.filter((r) => !r.advanced).map((r): [string, string] => [`data.${r.key}`, r.field.name])];
  return (
    <form className="space-y-4 rounded-md border p-4" aria-label={t(initial ? "mcp_tools.edit" : "mcp_tools.new")} onSubmit={(e) => { e.preventDefault(); if (ready) save.mutate(); }}>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="tool-name">{t("mcp_tools.name")}</Label>
        <Input id="tool-name" className="font-mono" maxLength={48} value={draft.name} aria-invalid={!!draft.name && !NAME.test(draft.name)} aria-describedby="tool-name-hint" onChange={(e) => set({ name: e.target.value.toLowerCase() })} placeholder="clientes_por_cidade" />
        <p id="tool-name-hint" className="text-xs text-muted-foreground">{t("mcp_tools.name_hint")}</p>
      </div>
      <div className="flex flex-col gap-1.5">
        <Label htmlFor="tool-desc">{t("mcp_tools.description")}</Label>
        <Textarea id="tool-desc" maxLength={1000} rows={3} value={draft.description} aria-describedby="tool-desc-hint" onChange={(e) => set({ description: e.target.value })} />
        <p id="tool-desc-hint" className="text-xs text-muted-foreground">{t("mcp_tools.description_hint")}</p>
      </div>
      <div className="space-y-1.5">
        <p className="text-sm font-medium">{t("mcp_tools.fixed")}</p>
        <div className="flex flex-wrap items-center gap-2">
          <RecordRules collectionId={collection.id} fields={rows} value={draft.conditions as Where} onChange={(w) => set({ conditions: w })} />
          <ViewConditions collectionId={collection.id} onPick={(w) => set({ conditions: w })} />
        </div>
      </div>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{t("mcp_tools.params")}</legend>
        <p className="text-xs text-muted-foreground">{t("mcp_tools.params_hint")}</p>
        {draft.params.map((p, i) => {
          const row = rows.find((r) => r.key === p.field);
          const ops = row ? opsFor(row) : [];
          return (
            <div key={i} className="grid gap-2 rounded-md border p-2 sm:grid-cols-[1fr_1fr_1fr_auto]">
              <Input aria-label={t("mcp_tools.param_name")} className="font-mono" maxLength={40} value={p.name} aria-invalid={!PARAM.test(p.name) || p.name === "page" || names.indexOf(p.name) !== i} onChange={(e) => setParam(i, { name: e.target.value.toLowerCase() })} />
              <Select value={p.field} items={Object.fromEntries(paramFields.map((r) => [r.key, r.field.name]))} onValueChange={(v) => { const r = rows.find((x) => x.key === v); if (r) setParam(i, { field: r.key, op: opsFor(r).includes(p.op) ? p.op : opsFor(r)[0] }); }}>
                <SelectTrigger aria-label={t("mcp_tools.param_field")} className="w-full"><SelectValue /></SelectTrigger>
                <SelectContent>{paramFields.map((r) => <SelectItem key={r.key} value={r.key}>{r.field.name}</SelectItem>)}</SelectContent>
              </Select>
              <Select value={p.op} items={Object.fromEntries(ops.map((o) => [o, t(`mcp_tools.op.${o}` as MessageKey)]))} onValueChange={(v) => setParam(i, { op: v as McpParamOp })}>
                <SelectTrigger aria-label={t("mcp_tools.param_op")} className="w-full"><SelectValue /></SelectTrigger>
                <SelectContent>{ops.map((o) => <SelectItem key={o} value={o}>{t(`mcp_tools.op.${o}` as MessageKey)}</SelectItem>)}</SelectContent>
              </Select>
              <IconAction label={t("mcp_tools.remove_param")} target={p.name} onClick={() => set({ params: draft.params.filter((_, j) => j !== i) })}><X className="size-4" /></IconAction>
              <Input aria-label={t("mcp_tools.param_description")} placeholder={t("mcp_tools.param_description")} maxLength={200} className="sm:col-span-2" value={p.description ?? ""} onChange={(e) => setParam(i, { description: e.target.value })} />
              <label className="flex items-center gap-2 text-sm">
                <Checkbox checked={p.required} onCheckedChange={(v) => setParam(i, { required: v === true })} />
                {t("mcp_tools.required")}
              </label>
            </div>
          );
        })}
        <Button type="button" variant="outline" size="sm" disabled={draft.params.length >= 10 || paramFields.length === 0} onClick={addParam}><Plus className="size-4" />{t("mcp_tools.add_param")}</Button>
      </fieldset>
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium">{t("mcp_tools.returns")}</legend>
        <p className="text-xs text-muted-foreground">{t("mcp_tools.returns_hint")}</p>
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {rows.filter((r) => !r.advanced).map((r) => (
            <label key={r.key} className="flex items-center gap-2 text-sm">
              <Checkbox checked={draft.fields.includes(r.key)} onCheckedChange={(v) => set({ fields: v === true ? [...draft.fields, r.key] : draft.fields.filter((f) => f !== r.key) })} />
              {r.field.name}
            </label>
          ))}
        </div>
      </fieldset>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="tool-sort">{t("mcp_tools.sort")}</Label>
          <Select value={draft.sort} items={Object.fromEntries(sorts)} onValueChange={(v) => set({ sort: String(v) })}>
            <SelectTrigger id="tool-sort" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent>{sorts.map(([v, l]) => <SelectItem key={v} value={v}>{l}</SelectItem>)}</SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="tool-order">{t("mcp_tools.order")}</Label>
          <Select value={draft.order} items={{ asc: t("mcp_tools.asc"), desc: t("mcp_tools.desc") }} onValueChange={(v) => set({ order: v as McpToolInput["order"] })}>
            <SelectTrigger id="tool-order" className="w-full"><SelectValue /></SelectTrigger>
            <SelectContent><SelectItem value="asc">{t("mcp_tools.asc")}</SelectItem><SelectItem value="desc">{t("mcp_tools.desc")}</SelectItem></SelectContent>
          </Select>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="tool-limit">{t("mcp_tools.limit")}</Label>
          <Input id="tool-limit" type="number" min={1} max={50} value={draft.limit} onChange={(e) => set({ limit: Math.min(50, Math.max(1, Number(e.target.value) || 1)) })} />
        </div>
      </div>
      <fieldset className="space-y-2 rounded-md border border-dashed p-3">
        <legend className="px-1 text-sm font-medium">{t("mcp_tools.try")}</legend>
        <p className="text-xs text-muted-foreground">{t("mcp_tools.try_hint")}</p>
        {draft.params.length > 0 && (
          <div className="grid gap-2 sm:grid-cols-2">
            {draft.params.map((p) => (
              <div key={p.name} className="flex flex-col gap-1">
                <Label htmlFor={`try-${p.name}`} className="font-mono text-xs">{p.name}{p.required && " *"}</Label>
                <Input id={`try-${p.name}`} value={tryArgs[p.name] ?? ""} placeholder={p.op === "in" ? t("mcp_tools.try_list") : undefined} onChange={(e) => setTryArgs({ ...tryArgs, [p.name]: e.target.value })} />
              </div>
            ))}
          </div>
        )}
        <Button type="button" variant="outline" size="sm" disabled={!ready || tryRun.isPending} onClick={() => tryRun.mutate()}>{t("mcp_tools.try_action")}</Button>
        <FormError message={tryRun.isError ? errorMessage(tryRun.error, "common.save_failed", t) : null} />
        {tryRun.data && (
          <div className="space-y-1.5" role="status">
            <p className="text-sm">{t("mcp_tools.try_total").replace("{n}", String(tryRun.data.total))}</p>
            {tryRun.data.records.length > 0 && (
              <CodeBlock copy={false} className="max-h-64 overflow-auto">{JSON.stringify(tryRun.data.records.slice(0, 5).map((r) => r.data), null, 2)}</CodeBlock>
            )}
          </div>
        )}
      </fieldset>
      <FormError message={save.isError ? errorMessage(save.error, "common.save_failed", t) : null} />
      <div className="flex gap-2">
        <Button type="submit" disabled={!ready || save.isPending}>{t("common.save")}</Button>
        <Button type="button" variant="ghost" onClick={onDone}>{t("common.cancel")}</Button>
      </div>
    </form>
  );
}

/** The collection's MCP tools: searches an owner describes for the assistants of its API keys. */
export function McpTools({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const key = ["mcp-tools", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => tools.mcpTools(collection.id) });
  const [editing, setEditing] = useState<McpTool | "new" | null>(null);
  const [removing, setRemoving] = useState<McpTool | null>(null);
  const refresh = () => void client.invalidateQueries({ queryKey: key });
  const toggle = useMutation({
    mutationFn: (x: McpTool) => tools.updateMcpTool(collection.id, x.id, { name: x.name, description: x.description, enabled: !x.enabled, conditions: x.conditions, params: x.params, fields: x.fields, sort: x.sort, order: x.order, limit: x.limit }),
    onSuccess: refresh,
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const items = list.data?.items ?? [];
  return (
    <FormSection id="mcp-tools" title={t("mcp_tools.title")} description={t("mcp_tools.desc_section")}>
      <div className="space-y-3">
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 && editing !== "new" ? (
          <EmptyState compact title={t("mcp_tools.empty")} />
        ) : (
          <ul className="divide-y rounded-md border" aria-label={t("mcp_tools.title")}>
            {items.map((x) => editing !== "new" && editing?.id === x.id ? (
              <li key={x.id} className="p-2"><ToolForm collection={collection} initial={x} onDone={() => setEditing(null)} /></li>
            ) : (
              <li key={x.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
                <Switch checked={x.enabled} aria-label={`${t("mcp_tools.enabled")}: ${x.name}`} disabled={toggle.isPending} onCheckedChange={() => toggle.mutate(x)} />
                <div className="min-w-48 flex-1">
                  <p className="flex items-center gap-2 font-mono text-sm font-medium">{x.name}{!x.enabled && <Badge variant="outline">{t("mcp_tools.off")}</Badge>}</p>
                  <p className="line-clamp-2 text-xs text-muted-foreground">{x.description}</p>
                  {x.params.length > 0 && <p className="font-mono text-xs text-muted-foreground">({x.params.map((p) => `${p.name}${p.required ? "" : "?"}`).join(", ")})</p>}
                </div>
                <IconAction label={t("common.edit")} target={x.name} onClick={() => setEditing(x)}><Pencil className="size-4" /></IconAction>
                <IconAction label={t("common.delete")} target={x.name} className="text-destructive" onClick={() => setRemoving(x)}><Trash2 className="size-4" /></IconAction>
              </li>
            ))}
          </ul>
        )}
        {editing === "new" ? <ToolForm collection={collection} onDone={() => setEditing(null)} /> : (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
            <Button variant="outline" disabled={items.length >= 20} onClick={() => setEditing("new")}><Plus className="size-4" />{t("mcp_tools.new")}</Button>
            <Link href="/api-access#mcp" className="text-sm text-primary underline-offset-2 hover:underline">{t("mcp_tools.connect_link")}</Link>
          </div>
        )}
      </div>
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("mcp_tools.delete_title").replace("{name}", removing?.name ?? "")}
        description={t("mcp_tools.delete_desc")}
        confirmLabel={t("common.delete")}
        destructive
        onConfirm={async () => {
          if (!removing) return;
          await tools.deleteMcpTool(collection.id, removing.id).then(refresh, (err) => toast.error(errorMessage(err, "common.save_failed", t)));
          setRemoving(null);
        }}
      />
    </FormSection>
  );
}

/** What AI assistants did in the collection through MCP: newest 100 calls of the last 30 days. */
export function McpActivity({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const list = useQuery({ queryKey: ["mcp-calls", collection.id], queryFn: () => tools.mcpCalls(collection.id) });
  const items = list.data?.items ?? [];
  return (
    <FormSection id="mcp-activity" title={t("mcp_activity.title")} description={t("mcp_activity.desc")}>
      {list.isPending ? <ListSkeleton rows={2} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
        <EmptyState compact title={t("mcp_activity.empty")} />
      ) : (
        <ul className="max-h-96 divide-y overflow-auto rounded-md border" aria-label={t("mcp_activity.title")}>
          {items.map((c, i) => (
            <li key={i} className="space-y-0.5 px-3 py-2 text-sm">
              <p className="flex flex-wrap items-center gap-2">
                <span className="font-mono font-medium">{c.tool}</span>
                <Badge variant={c.ok ? "outline" : "destructive"}>{t(c.ok ? "mcp_activity.ok" : "mcp_activity.failed")}</Badge>
              </p>
              <p className="text-xs text-muted-foreground">
                {format(c.created_at)} · {c.user_name} · {t("mcp_activity.key").replace("{name}", c.key_name)}{!c.key_live && ` (${t("mcp_activity.key_gone")})`}
              </p>
              {c.error && <p className="break-words text-xs text-destructive">{c.error}</p>}
            </li>
          ))}
        </ul>
      )}
    </FormSection>
  );
}
