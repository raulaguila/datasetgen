"use client";

import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Loader2, Play } from "lucide-react";
import { FormError } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { CodeBlock } from "@/components/secret";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useI18n } from "@/lib/i18n/provider";

type Prop = { type?: string | string[]; description?: string };
export type McpTool = {
  name: string;
  description?: string;
  inputSchema?: { properties?: Record<string, Prop>; required?: string[] };
  annotations?: { readOnlyHint?: boolean };
};
export type Called = { isError?: boolean; content?: { type: string; text?: string }[]; structuredContent?: unknown };

type Kind = "string" | "number" | "boolean" | "json";
function kindOf(p: Prop): Kind {
  const t = Array.isArray(p.type) ? p.type.find((x) => x !== "null") : p.type;
  return t === "string" ? "string" : t === "integer" || t === "number" ? "number" : t === "boolean" ? "boolean" : "json";
}

class ArgError extends Error {
  constructor(readonly key: "mcp_tester.json_invalid" | "mcp_tester.number_invalid", readonly field: string) { super(field); }
}

/** The arguments the form holds, typed from the tool's schema; empty fields are left out. */
function build(props: Record<string, Prop>, raw: Record<string, string>): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, p] of Object.entries(props)) {
    const v = (raw[k] ?? "").trim();
    if (!v) continue;
    const kind = kindOf(p);
    if (kind === "string") out[k] = raw[k];
    else if (kind === "boolean") out[k] = v === "true";
    else if (kind === "number") {
      if (Number.isNaN(Number(v))) throw new ArgError("mcp_tester.number_invalid", k);
      out[k] = Number(v);
    } else {
      try { out[k] = JSON.parse(v); } catch { throw new ArgError("mcp_tester.json_invalid", k); }
    }
  }
  return out;
}

/** The form's text for each argument of an object, the reverse of build. */
function fieldsOf(props: Record<string, Prop>, args: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, p] of Object.entries(props)) {
    const v = args[k];
    if (v === undefined || v === null) continue;
    const kind = kindOf(p);
    out[k] = kind === "boolean" ? (v ? "true" : "") : kind === "json" ? JSON.stringify(v) : String(v);
  }
  return out;
}

/** Runs one tool of the key with arguments filled from its schema, as an assistant would call it. */
export function McpToolTester({ tool, collections, call }: {
  tool: McpTool;
  collections: { id: string; name: string }[];
  call: (name: string, args: Record<string, unknown>) => Promise<Called>;
}) {
  const { t } = useI18n();
  const props = tool.inputSchema?.properties ?? {};
  const required = tool.inputSchema?.required ?? [];
  const writes = tool.annotations?.readOnlyHint !== true;
  const [fields, setFields] = useState<Record<string, string>>(() => ({
    ...("collection_id" in props && collections[0] ? { collection_id: collections[0].id } : {}),
    ...("dry_run" in props ? { dry_run: "true" } : {}), // a bulk change starts as a count
  }));
  const [json, setJson] = useState<string | null>(null); // the whole arguments as text, in JSON mode
  const [argError, setArgError] = useState<string | null>(null);
  const [confirming, setConfirming] = useState<Record<string, unknown> | null>(null);
  const run = useMutation({
    mutationFn: async (args: Record<string, unknown>) => {
      const start = performance.now();
      const res = await call(tool.name, args);
      return { res, ms: Math.round(performance.now() - start) };
    },
  });

  const args = (): Record<string, unknown> | null => {
    try {
      if (json !== null) {
        const v: unknown = JSON.parse(json || "{}");
        if (!v || typeof v !== "object" || Array.isArray(v)) throw new ArgError("mcp_tester.json_invalid", "arguments");
        return v as Record<string, unknown>;
      }
      return build(props, fields);
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
    else { setFields(fieldsOf(props, a)); setJson(null); }
  };
  const submit = () => {
    setArgError(null);
    const a = args();
    if (!a) return;
    if (writes && a.dry_run !== true) setConfirming(a);
    else run.mutate(a);
  };

  const names = Object.fromEntries(collections.map((c) => [c.id, c.name]));
  const res = run.data?.res;
  const shown = res ? (res.isError || res.structuredContent === undefined
    ? (res.content ?? []).map((c) => c.text ?? "").join("\n")
    : JSON.stringify(res.structuredContent, null, 2)) : "";

  return (
    <form aria-label={t("mcp_tester.title")} className="space-y-3 rounded-md border p-3" onSubmit={(e) => { e.preventDefault(); submit(); }}>
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <h4 className="font-mono text-sm font-medium">{tool.name}</h4>
          {writes && <Badge variant="outline">{t("mcp_tester.writes")}</Badge>}
        </div>
        {tool.description && <p className="text-xs text-muted-foreground">{tool.description}</p>}
      </div>

      {json !== null ? (
        <div className="space-y-1">
          <Label htmlFor={`mcp-args-${tool.name}`}>{t("mcp_tester.args")}</Label>
          <Textarea id={`mcp-args-${tool.name}`} rows={8} spellCheck={false} className="font-mono text-xs" value={json} onChange={(e) => setJson(e.target.value)} />
        </div>
      ) : Object.keys(props).length === 0 ? (
        <p className="text-xs text-muted-foreground">{t("mcp_tester.no_args")}</p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          {Object.entries(props).sort(([a], [b]) => Number(required.includes(b)) - Number(required.includes(a))).map(([k, p]) => {
            const id = `mcp-arg-${tool.name}-${k}`;
            const kind = kindOf(p);
            const set = (v: string) => setFields((f) => ({ ...f, [k]: v }));
            const label = <><span className="font-mono">{k}</span>{required.includes(k) && <span className="text-muted-foreground"> · {t("mcp_tester.required")}</span>}</>;
            return (
              <div key={k} className={kind === "json" ? "space-y-1 sm:col-span-2" : "space-y-1"}>
                {kind === "boolean" ? (
                  <div className="flex items-center gap-2">
                    <Switch aria-label={k} checked={fields[k] === "true"} onCheckedChange={(v) => set(v ? "true" : "")} />
                    <span className="text-sm">{label}</span>
                  </div>
                ) : (
                  <>
                    <Label htmlFor={id} className="text-sm font-normal">{label}</Label>
                    {k === "collection_id" && collections.length > 0 ? (
                      <Select value={fields[k] ?? null} items={names} onValueChange={(v) => set(String(v ?? ""))}>
                        <SelectTrigger id={id} className="w-full"><SelectValue /></SelectTrigger>
                        <SelectContent>{collections.map((c) => <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>)}</SelectContent>
                      </Select>
                    ) : kind === "json" ? (
                      <Textarea id={id} rows={2} spellCheck={false} className="font-mono text-xs" placeholder="JSON" value={fields[k] ?? ""} onChange={(e) => set(e.target.value)} />
                    ) : (
                      <Input id={id} inputMode={kind === "number" ? "decimal" : undefined} className="font-mono" value={fields[k] ?? ""} onChange={(e) => set(e.target.value)} />
                    )}
                  </>
                )}
                {p.description && <p className="text-xs text-muted-foreground">{p.description}</p>}
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
      {run.data && (
        <section aria-label={t("mcp_tester.result")} className="space-y-1">
          <p role="status" className={res?.isError ? "text-sm font-medium text-destructive" : "text-sm font-medium"}>
            {t(res?.isError ? "mcp_tester.refused" : "mcp_tester.ok").replace("{ms}", String(run.data.ms))}
          </p>
          <CodeBlock className="max-h-96 overflow-y-auto">{shown}</CodeBlock>
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
