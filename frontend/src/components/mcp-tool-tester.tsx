"use client";

import { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Info, Loader2, Play, Search } from "lucide-react";
import { FormError, FormSection } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { ListSkeleton } from "@/components/skeletons";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { fieldsOf } from "@/components/field-builder";
import { RecordRules } from "@/components/record-rules";
import type { Where } from "@/components/rule-model";
import { useOrigin } from "@/components/rest-examples";
import { CodeBlock } from "@/components/secret";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { collections as collectionsApi } from "@/lib/api/endpoints";
import { useI18n } from "@/lib/i18n/provider";
import type { MessageKey } from "@/lib/i18n/messages";
import { cn } from "@/lib/utils";

/** The server's own read tools: the OpenAI example allows these, and the list groups by them. */
export const MCP_READ_TOOLS = ["workspace_overview", "get_collection", "search_records", "get_record", "related_records", "collection_stats",
  "aggregate_records", "field_values", "get_records", "record_history", "list_views", "search_everywhere"];
const MCP_WRITE_TOOLS = ["list_trash", "create_record", "create_records", "update_record", "update_records", "delete_records", "restore_records"];

type Prop = { type?: string | string[]; description?: string; items?: Prop };
export type McpTool = {
  name: string;
  description?: string;
  inputSchema?: { properties?: Record<string, Prop>; required?: string[] };
  annotations?: { readOnlyHint?: boolean };
};
export type Called = { isError?: boolean; content?: { type: string; text?: string }[]; structuredContent?: unknown };
/** A collection as workspace_overview gives it, with the fields the pickers offer. */
export type ReachedCollection = { id: string; name: string; access: string; fields?: { key: string; name?: string; type: string }[] };

type Widget = "collection" | "field" | "group_by" | "sort" | "order" | "bucket" | "keys" | "numbers" | "refs" | "list" | "where" | "hidden" | "string" | "number" | "boolean" | "json";
type Values = Record<string, unknown>;
type Run = { tool: string; args: Values; res: Called; ms: number; at: Date };

const typeOf = (p: Prop) => (Array.isArray(p.type) ? p.type.find((x) => x !== "null") : p.type);
const ENUMS: Partial<Record<Widget, string[]>> = { order: ["asc", "desc"], bucket: ["day", "week", "month", "year"] };

/** How the form edits an argument: the record tools' known names get pickers, the rest goes by type. */
function widgetOf(name: string, p: Prop, props: Record<string, Prop>): Widget {
  if ("rules" in props && ["rules", "groups", "any"].includes(name)) return name === "rules" ? "where" : "hidden";
  const fixed: Record<string, Widget> = { collection_id: "collection", field: "field", group_by: "group_by", sort: "sort", order: "order", bucket: "bucket", fields: "keys", numbers: "numbers", expand: "refs" };
  if (fixed[name] && "collection_id" in props) return fixed[name];
  const t = typeOf(p);
  if (t === "string") return "string";
  if (t === "integer" || t === "number") return "number";
  if (t === "boolean") return "boolean";
  if (t === "array" && typeOf(p.items ?? {}) === "string") return "list";
  return "json";
}

class ArgError extends Error {
  constructor(readonly key: "mcp_tester.json_invalid" | "mcp_tester.number_invalid", readonly field: string) { super(field); }
}

/** The arguments the form holds, typed for the call; empty ones are left out. */
function build(props: Record<string, Prop>, v: Values): Values {
  const out: Values = {};
  for (const [k, p] of Object.entries(props)) {
    const w = widgetOf(k, p, props);
    const val = v[k];
    if (w === "where") {
      const where = val as Where | undefined;
      if (where && where.rules.length) out.rules = where.rules;
      if (where?.groups?.length) out.groups = where.groups;
      if (where?.any) out.any = true;
    } else if (w === "hidden") continue;
    else if (w === "boolean") { if (val) out[k] = true; }
    else if (Array.isArray(val)) { if (val.length) out[k] = val; }
    else {
      const s = String(val ?? "").trim();
      if (!s) continue;
      if (w === "number") {
        if (Number.isNaN(Number(s))) throw new ArgError("mcp_tester.number_invalid", k);
        out[k] = Number(s);
      } else if (w === "json") {
        try { out[k] = JSON.parse(s); } catch { throw new ArgError("mcp_tester.json_invalid", k); }
      } else out[k] = val;
    }
  }
  return out;
}

/** The form's values for arguments given as JSON, the reverse of build. */
function valuesOf(props: Record<string, Prop>, args: Values): Values {
  const out: Values = {};
  for (const [k, p] of Object.entries(props)) {
    const w = widgetOf(k, p, props);
    const a = args[k];
    if (w === "where") out[k] = { any: !!args.any, rules: (args.rules as Where["rules"]) ?? [], groups: (args.groups as Where["groups"]) ?? [] };
    else if (a === undefined || a === null || w === "hidden") continue;
    else if (w === "boolean") out[k] = !!a;
    else if (["keys", "numbers", "refs", "list"].includes(w)) out[k] = Array.isArray(a) ? a.map(String) : [String(a)];
    else out[k] = w === "json" ? JSON.stringify(a) : String(a);
  }
  return out;
}

const firstSentence = (s = "") => s.match(/^.*?(?<!\be\.g|\bi\.e)[.;:](\s|$)/)?.[0].trim() ?? s;
const bytes = (s: string) => new Blob([s]).size;

/** Tries any tool a key reaches: a list to pick from, a form built from its schema, and what came back. */
export function McpToolLab({ tools, collections, call }: {
  tools: McpTool[];
  collections: ReachedCollection[];
  call: (name: string, args: Values) => Promise<Called>;
}) {
  const { t } = useI18n();
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<string | null>(null);
  const [history, setHistory] = useState<Run[]>([]);
  const [shown, setShown] = useState<Run | null>(null);
  const [form, setForm] = useState<{ tool: string; values: Values } | null>(null);
  const [opened, setOpened] = useState(0); // remounts the form each time a tool or past call is picked
  const tool = tools.find((x) => x.name === picked);

  const groupOf = (x: McpTool) => (MCP_WRITE_TOOLS.includes(x.name) ? "write" : MCP_READ_TOOLS.includes(x.name) ? "read" : "own");
  const q = query.trim().toLowerCase();
  const listed = tools.filter((x) => !q || x.name.includes(q) || (x.description ?? "").toLowerCase().includes(q));
  const groups = (["read", "write", "own"] as const).map((g) => ({ g, items: listed.filter((x) => groupOf(x) === g) })).filter((x) => x.items.length);

  const pick = (name: string, values?: Values) => {
    setPicked(name);
    setForm(values ? { tool: name, values } : null);
    setShown(null);
    setOpened((n) => n + 1);
  };

  return (
    <FormSection id="mcp-lab" title={t("mcp_tester.title")} description={t("mcp_tester.desc")} wide headAbove>
      <div className="grid gap-4 md:grid-cols-[15rem_minmax(0,1fr)]">
        <div className="space-y-2">
          <div className="relative">
            <Search aria-hidden className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input aria-label={t("mcp_tester.search")} placeholder={t("mcp_tester.search")} className="pl-8" value={query} onChange={(e) => setQuery(e.target.value)} />
          </div>
          <nav aria-label={t("mcp_tester.tools")} className="max-h-[28rem] space-y-3 overflow-y-auto pr-1">
            {groups.map(({ g, items }) => (
              <div key={g} className="space-y-1">
                <p className="px-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">{t(`mcp_tester.group_${g}` as MessageKey)}</p>
                <ul className="space-y-0.5">
                  {items.map((x) => (
                    <li key={x.name}>
                      <button type="button" aria-current={picked === x.name ? "true" : undefined} onClick={() => pick(x.name)}
                        className="w-full rounded-md px-2 py-1.5 text-left hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring aria-[current]:bg-muted">
                        <span className="block font-mono text-xs font-medium">{x.name}</span>
                        <span className="line-clamp-1 text-xs text-muted-foreground">{firstSentence(x.description)}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </nav>
        </div>

        <div className="min-w-0 space-y-4">
          {tool ? (
            <ToolForm key={opened} tool={tool} collections={collections}
              initial={form?.tool === tool.name ? form.values : undefined}
              call={call} shown={shown?.tool === tool.name ? shown : null}
              onRun={(run) => { setShown(run); setHistory((h) => [run, ...h].slice(0, 10)); }} />
          ) : (
            <EmptyState compact title={t("mcp_tester.pick")} />
          )}
          {history.length > 0 && (
            <section aria-label={t("mcp_tester.history")} className="space-y-1">
              <h4 className="text-sm font-medium">{t("mcp_tester.history")}</h4>
              <ul className="divide-y rounded-md border text-xs">
                {history.map((r, i) => (
                  <li key={i}>
                    <button type="button" className="flex w-full flex-wrap items-center gap-x-2 px-2 py-1.5 text-left hover:bg-muted"
                      onClick={() => { const p = tools.find((x) => x.name === r.tool)?.inputSchema?.properties ?? {}; pick(r.tool, valuesOf(p, r.args)); setShown(r); }}>
                      <span className="font-mono font-medium">{r.tool}</span>
                      <span className={r.res.isError ? "text-destructive" : "text-muted-foreground"}>{t(r.res.isError ? "mcp_activity.failed" : "mcp_activity.ok")}</span>
                      <span className="text-muted-foreground">{r.ms} ms · {r.at.toLocaleTimeString()}</span>
                      <code className="basis-full truncate text-muted-foreground">{JSON.stringify(r.args)}</code>
                    </button>
                  </li>
                ))}
              </ul>
            </section>
          )}
        </div>
      </div>
    </FormSection>
  );
}

/** One tool's form, run button and answer. */
function ToolForm({ tool, collections, initial, call, shown, onRun }: {
  tool: McpTool; collections: ReachedCollection[]; initial?: Values;
  call: (name: string, args: Values) => Promise<Called>; shown: Run | null; onRun: (run: Run) => void;
}) {
  const { t } = useI18n();
  const origin = useOrigin();
  const props = tool.inputSchema?.properties ?? {};
  const required = tool.inputSchema?.required ?? [];
  const writes = tool.annotations?.readOnlyHint !== true;
  const [values, setValues] = useState<Values>(() => initial ?? {
    ...("collection_id" in props && collections[0] ? { collection_id: collections[0].id } : {}),
    ...("dry_run" in props ? { dry_run: true } : {}), // a bulk change starts as a count
  });
  const [json, setJson] = useState<string | null>(null);
  const [argError, setArgError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<Values | null>(null);
  const [tab, setTab] = useState<"result" | "request">("result");
  const set = (k: string, v: unknown) => setValues((x) => ({ ...x, [k]: v }));

  const cid = typeof values.collection_id === "string" ? values.collection_id : "";
  const fields = collections.find((c) => c.id === cid)?.fields ?? [];
  const schema = useQuery({ queryKey: ["collections", cid], queryFn: () => collectionsApi.byID(cid), enabled: !!cid && "rules" in props });
  const rows = schema.data ? fieldsOf(schema.data.schema ?? null) : [];

  const run = useMutation({
    mutationFn: async (args: Values) => {
      const start = performance.now();
      const res = await call(tool.name, args);
      return { tool: tool.name, args, res, ms: Math.round(performance.now() - start), at: new Date() } satisfies Run;
    },
    onSuccess: onRun,
  });

  const args = (): Values | null => {
    try {
      if (json !== null) {
        const v: unknown = JSON.parse(json || "{}");
        if (!v || typeof v !== "object" || Array.isArray(v)) throw new ArgError("mcp_tester.json_invalid", "arguments");
        return v as Values;
      }
      return build(props, values);
    } catch (e) {
      setArgError(t(e instanceof ArgError ? e.key : "mcp_tester.json_invalid").replace("{field}", e instanceof ArgError ? e.field : "arguments"));
      return null;
    }
  };
  const toggleJson = () => {
    setArgError(null);
    const a = args();
    if (!a) return;
    if (json === null) setJson(JSON.stringify(a, null, 2));
    else { setValues(valuesOf(props, a)); setJson(null); }
  };
  const submit = () => {
    setArgError(null);
    const a = args();
    if (!a) return;
    if (writes && a.dry_run !== true) setConfirming(a);
    else run.mutate(a);
  };

  const res = shown?.res;
  const text = res ? (res.isError || res.structuredContent === undefined
    ? (res.content ?? []).map((c) => c.text ?? "").join("\n")
    : JSON.stringify(res.structuredContent, null, 2)) : "";
  const size = bytes(text);
  const request = shown ? JSON.stringify({ jsonrpc: "2.0", id: 1, method: "tools/call", params: { name: shown.tool, arguments: shown.args } }) : "";
  const curl = shown ? `curl ${origin}/api/proxy/v1/mcp \\\n  -H "Authorization: Bearer $DATASET_KEY" \\\n  -H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" \\\n  -d '${request.replaceAll("'", "'\\''")}'` : "";
  const fieldsOfType = (types: string[]) => fields.filter((f) => types.includes(f.type));
  const dateGroup = fieldsOfType(["date", "datetime"]).some((f) => f.key === values.group_by);

  const widget = (k: string, p: Prop) => {
    const w = widgetOf(k, p, props);
    const id = `mcp-arg-${tool.name}-${k}`;
    const str = typeof values[k] === "string" ? (values[k] as string) : "";
    const list = Array.isArray(values[k]) ? (values[k] as string[]) : [];
    const pickOne = (options: { value: string; label: string }[], allowNone = false) => (
      <Select value={str || null} items={Object.fromEntries(options.map((o) => [o.value, o.label]))} onValueChange={(v) => set(k, v === "__none" ? "" : String(v ?? ""))}>
        <SelectTrigger id={id} className="w-full"><SelectValue placeholder={t("mcp_tester.none")} /></SelectTrigger>
        <SelectContent>
          {allowNone && <SelectItem value="__none">{t("mcp_tester.none")}</SelectItem>}
          {options.map((o) => <SelectItem key={o.value} value={o.value}>{o.label}</SelectItem>)}
        </SelectContent>
      </Select>
    );
    const fieldOptions = (fs: { key: string; name?: string }[]) => fs.map((f) => ({ value: f.key, label: f.name && f.name !== f.key ? `${f.key} · ${f.name}` : f.key }));
    const multi = (fs: { key: string; name?: string }[]) => (
      <div role="group" aria-labelledby={`${id}-label`} className="flex flex-wrap gap-1">
        {fs.length === 0 && <span className="text-xs text-muted-foreground">{t("mcp_tester.no_fields")}</span>}
        {fs.map((f) => {
          const on = list.includes(f.key);
          return (
            <button key={f.key} type="button" aria-pressed={on} onClick={() => set(k, on ? list.filter((x) => x !== f.key) : [...list, f.key])}
              className="rounded border px-1.5 py-0.5 font-mono text-xs hover:border-foreground/40 aria-pressed:border-primary aria-pressed:bg-primary aria-pressed:text-primary-foreground">
              {f.key}
            </button>
          );
        })}
      </div>
    );
    switch (w) {
      case "hidden": return null;
      case "where":
        return !cid ? null : schema.data ? (
          <RecordRules fields={rows} collection={schema.data} value={(values.rules as Where) ?? { any: false, rules: [] }} onChange={(v) => set("rules", v)} />
        ) : <ListSkeleton rows={2} />;
      case "collection": return pickOne(collections.map((c) => ({ value: c.id, label: c.name })));
      case "field": return pickOne(fieldOptions(fields));
      case "group_by": return pickOne(fieldOptions(fields), true);
      case "sort": return pickOne([{ value: "updated_at", label: "updated_at" }, { value: "created_at", label: "created_at" }, ...fields.map((f) => ({ value: `data.${f.key}`, label: `data.${f.key}` }))], true);
      case "order": case "bucket": return pickOne((ENUMS[w] ?? []).map((v) => ({ value: v, label: v })), true);
      case "keys": return multi(fields);
      case "numbers": return multi(fieldsOfType(["number", "integer", "formula", "rollup", "auto_number"]));
      case "refs": return multi(fieldsOfType(["reference", "references"]));
      case "boolean": return <Switch aria-label={k} checked={values[k] === true} onCheckedChange={(v) => set(k, v)} />;
      case "list": return <Textarea id={id} rows={2} spellCheck={false} className="font-mono text-xs" placeholder={t("mcp_tester.one_per_line")} value={list.join("\n")} onChange={(e) => set(k, e.target.value.split(/[\n,]/).map((x) => x.trim()).filter(Boolean))} />;
      case "json": return <Textarea id={id} rows={3} spellCheck={false} className="font-mono text-xs" placeholder="JSON" value={str} onChange={(e) => set(k, e.target.value)} />;
      default: return <Input id={id} inputMode={w === "number" ? "decimal" : undefined} className="font-mono" value={str} onChange={(e) => set(k, e.target.value)} />;
    }
  };

  const entries = Object.entries(props)
    .filter(([k, p]) => widgetOf(k, p, props) !== "hidden" && !(k === "bucket" && !dateGroup))
    .sort(([a], [b]) => Number(required.includes(b)) - Number(required.includes(a)));

  return (
    <form aria-label={t("mcp_tester.form")} className="space-y-4" onSubmit={(e) => { e.preventDefault(); submit(); }}>
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <h4 className="font-mono text-sm font-semibold">{tool.name}</h4>
          {writes && <Badge variant="outline">{t("mcp_tester.writes")}</Badge>}
        </div>
        {tool.description && <p className="text-sm text-muted-foreground">{tool.description}</p>}
      </div>

      {json !== null ? (
        <div className="space-y-1">
          <Label htmlFor={`mcp-args-${tool.name}`}>{t("mcp_tester.args")}</Label>
          <Textarea id={`mcp-args-${tool.name}`} rows={10} spellCheck={false} className="font-mono text-xs" value={json} onChange={(e) => setJson(e.target.value)} />
        </div>
      ) : entries.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("mcp_tester.no_args")}</p>
      ) : (
        <div className="grid gap-x-4 gap-y-3 lg:grid-cols-2">
          {entries.map(([k, p]) => {
            const w = widgetOf(k, p, props);
            const wide = ["where", "json", "keys", "numbers", "refs", "list"].includes(w);
            const desc = w === "where" ? t("mcp_tester.rules_hint") : p.description ?? "";
            const short = firstSentence(desc);
            return (
              <div key={k} className={cn("min-w-0 space-y-1", wide && "lg:col-span-2")}>
                <div className="flex items-center gap-1">
                  <Label id={`mcp-arg-${tool.name}-${k}-label`} htmlFor={`mcp-arg-${tool.name}-${k}`} className="text-sm font-normal">
                    <span className="font-mono">{w === "where" ? t("mcp_tester.conditions") : k}</span>
                    {required.includes(k) && <span className="text-muted-foreground"> · {t("mcp_tester.required")}</span>}
                  </Label>
                  {desc.length > short.length && (
                    <Tooltip>
                      <TooltipTrigger render={<button type="button" aria-label={`${t("mcp_tester.about")} ${k}`} className="text-muted-foreground hover:text-foreground" />}><Info className="size-3.5" /></TooltipTrigger>
                      <TooltipContent className="max-w-sm">{desc}</TooltipContent>
                    </Tooltip>
                  )}
                </div>
                {widget(k, p)}
                {short && <p className="text-xs text-muted-foreground">{short}</p>}
              </div>
            );
          })}
        </div>
      )}

      <FormError message={argError} />
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={run.isPending}>
          {run.isPending ? <Loader2 className="size-4 animate-spin" /> : <Play className="size-4" />}
          {t("mcp_tester.run")}
        </Button>
        <Button type="button" variant="ghost" onClick={toggleJson}>{t(json === null ? "mcp_tester.as_json" : "mcp_tester.as_form")}</Button>
      </div>

      <FormError message={run.isError ? t(run.error.message === "unauthorized" ? "mcp_connect.test_unauthorized" : "mcp_connect.test_failed") : null} />
      {shown && res && (
        <section aria-label={t("mcp_tester.result")} className="space-y-2">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <p role="status" className={res.isError ? "text-sm font-medium text-destructive" : "text-sm font-medium"}>
              {t(res.isError ? "mcp_tester.refused" : "mcp_tester.ok").replace("{ms}", String(shown.ms))}
            </p>
            <p className="text-xs text-muted-foreground">{t("mcp_tester.size").replace("{kb}", (size / 1024).toFixed(1)).replace("{tokens}", Math.ceil(text.length / 4).toLocaleString())}</p>
            <div role="tablist" aria-label={t("mcp_tester.result")} className="ml-auto flex gap-1 text-xs">
              {(["result", "request"] as const).map((x) => (
                <button key={x} type="button" role="tab" aria-selected={tab === x} onClick={() => setTab(x)}
                  className="rounded px-2 py-0.5 text-muted-foreground aria-selected:bg-muted aria-selected:font-medium aria-selected:text-foreground">
                  {t(`mcp_tester.tab_${x}` as MessageKey)}
                </button>
              ))}
            </div>
          </div>
          {tab === "result" ? <CodeBlock className="max-h-96 overflow-y-auto">{text}</CodeBlock> : (
            <div className="space-y-1">
              <CodeBlock className="max-h-64 overflow-y-auto">{JSON.stringify(JSON.parse(request), null, 2)}</CodeBlock>
              <p className="text-xs text-muted-foreground">{t("mcp_tester.curl_hint")}</p>
              <CodeBlock>{curl}</CodeBlock>
            </div>
          )}
        </section>
      )}

      <ConfirmDialog
        open={confirming !== null}
        onOpenChange={(o) => { if (!o) setConfirming(null); }}
        title={t("mcp_tester.confirm_title").replace("{tool}", tool.name)}
        description={t("mcp_tester.confirm_desc")}
        confirmLabel={t("mcp_tester.run")}
        destructive
        onConfirm={() => { if (confirming) run.mutate(confirming); }}
      />
    </form>
  );
}
