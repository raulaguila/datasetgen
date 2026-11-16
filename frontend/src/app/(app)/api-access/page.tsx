"use client";

import { useState } from "react";
import { Tabs } from "@base-ui/react/tabs";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Loader2, PlugZap } from "lucide-react";
import { FormError, FormSection, FormSections, PageBody, PageHeader } from "@/components/bits";
import { ApiKeysCard } from "@/components/api-keys";
import { QueryErrorState } from "@/components/query-error-state";
import { RestExamples, useOrigin } from "@/components/rest-examples";
import { CodeBlock } from "@/components/secret";
import { ListSkeleton } from "@/components/skeletons";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { collections } from "@/lib/api/endpoints";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useI18n } from "@/lib/i18n/provider";

const MCP_PATH = "/api/proxy/v1/mcp";

type Reach = { tools: string[]; collections: { name: string; access: string }[] };

/** One JSON-RPC call to the MCP endpoint with key, as an assistant would make it. */
async function rpc(key: string, method: string, params: object): Promise<Record<string, unknown>> {
  const res = await fetch(MCP_PATH, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json, text/event-stream", Authorization: `Bearer ${key}` },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }),
  });
  if (res.status === 401) throw new Error("unauthorized");
  const body = (await res.json().catch(() => null)) as { result?: Record<string, unknown> } | null;
  if (!res.ok || !body?.result) throw new Error("failed");
  return body.result;
}

/** What a key reaches through MCP: its tools and its collections. */
async function reachOf(key: string): Promise<Reach> {
  const list = await rpc(key, "tools/list", {});
  const call = await rpc(key, "tools/call", { name: "list_collections", arguments: {} });
  const tools = (list.tools as { name: string }[] | undefined) ?? [];
  const collections = ((call.structuredContent as { collections?: Reach["collections"] } | undefined)?.collections) ?? [];
  return { tools: tools.map((x) => x.name), collections };
}

/** How to plug an AI assistant into /v1/mcp; a key just made fills the examples. */
function McpConnectCard({ secret }: { secret: string | null }) {
  const { t } = useI18n();
  const url = `${useOrigin()}${MCP_PATH}`;
  const key = secret ?? t("mcp_connect.key_placeholder");
  const claudeCode = `claude mcp add --transport http dataset \\\n  ${url} \\\n  --header "Authorization: Bearer ${key}"`;
  const generic = JSON.stringify({ mcpServers: { dataset: { url, headers: { Authorization: `Bearer ${key}` } } } }, null, 2);
  // Claude Desktop runs local servers only; mcp-remote bridges it to the HTTP endpoint.
  const desktop = JSON.stringify({
    mcpServers: { dataset: { command: "npx", args: ["-y", "mcp-remote", url, "--header", "Authorization:${DATASET_AUTH}"], env: { DATASET_AUTH: `Bearer ${key}` } } },
  }, null, 2);
  const codex = `# export DATASET_KEY="${key}"\n[mcp_servers.dataset]\nurl = "${url}"\nbearer_token_env_var = "DATASET_KEY"`;
  const openai = `curl https://api.openai.com/v1/responses \\\n  -H "Authorization: Bearer $OPENAI_API_KEY" -H "Content-Type: application/json" \\\n  -d '${JSON.stringify({
    model: "gpt-5",
    tools: [{ type: "mcp", server_label: "dataset", server_url: url, headers: { Authorization: `Bearer ${key}` }, require_approval: "never" }],
    input: t("mcp_connect.openai_prompt"),
  }, null, 2)}'`;

  // The typed key wins; a key just made fills the field until then.
  const [typed, setTyped] = useState<string | null>(null);
  const testKey = (typed ?? secret ?? "").trim();
  const check = useMutation({ mutationFn: () => reachOf(testKey) });

  // Hosted clients (the OpenAI API) call the server from the internet: a local or plain-HTTP address cannot work for them.
  const local = /^http:|\/\/(localhost|127\.|\[::1\])/.test(url);
  const clients: { id: string; name: string; hint?: string; code: string; note?: string }[] = [
    { id: "claude-code", name: "Claude Code", code: claudeCode },
    { id: "claude-desktop", name: "Claude Desktop", hint: t("mcp_connect.desktop_hint"), code: desktop },
    { id: "codex", name: "Codex", hint: t("mcp_connect.codex_hint"), code: codex },
    { id: "openai", name: t("mcp_connect.openai_api"), hint: t("mcp_connect.openai_hint"), code: openai, note: t("mcp_connect.chatgpt_note") },
    { id: "other", name: t("mcp_connect.generic"), hint: t("mcp_connect.generic_hint"), code: generic },
  ];

  return (
    <FormSection id="mcp" title={t("mcp_connect.title")} description={t(secret ? "mcp_connect.desc_filled" : "mcp_connect.desc")} wide>
      <ol className="space-y-6 text-sm">
        <Step n={1} title={t("mcp_connect.url")}>
          <CodeBlock>{url}</CodeBlock>
          <p className="text-xs text-muted-foreground">{t("mcp_connect.url_hint")}</p>
          {local && <p role="note" className="rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground">{t("mcp_connect.local_warning")}</p>}
        </Step>

        <Step n={2} title={t("mcp_connect.step_client")}>
          <Tabs.Root defaultValue="claude-code">
            <Tabs.List aria-label={t("mcp_connect.step_client")} className="flex gap-1 overflow-x-auto border-b">
              {clients.map((c) => (
                <Tabs.Tab key={c.id} value={c.id} className="-mb-px shrink-0 whitespace-nowrap border-b-2 border-transparent px-3 py-1.5 text-sm text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-[active]:border-primary data-[active]:font-medium data-[active]:text-foreground">
                  {c.name}
                </Tabs.Tab>
              ))}
            </Tabs.List>
            {clients.map((c) => (
              <Tabs.Panel key={c.id} value={c.id} className="space-y-1.5 pt-3 outline-none">
                {c.hint && <p className="text-xs text-muted-foreground">{c.hint}</p>}
                <CodeBlock>{c.code}</CodeBlock>
                {c.note && <p className="text-xs text-muted-foreground">{c.note}</p>}
              </Tabs.Panel>
            ))}
          </Tabs.Root>
        </Step>

        <Step n={3} title={t("mcp_connect.test")}>
          <form className="space-y-2" aria-label={t("mcp_connect.test")} onSubmit={(e) => { e.preventDefault(); if (testKey) check.mutate(); }}>
            <p className="text-xs text-muted-foreground">{t("mcp_connect.test_hint")}</p>
            <Label htmlFor="mcp-test-key" className="sr-only">{t("mcp_connect.test_key")}</Label>
            <div className="flex flex-wrap gap-2">
              <Input id="mcp-test-key" type="password" autoComplete="off" className="min-w-0 flex-1 font-mono" placeholder="dsk_…" value={typed ?? secret ?? ""} onChange={(e) => setTyped(e.target.value)} />
              <Button type="submit" variant="outline" disabled={!testKey || check.isPending}>
                {check.isPending ? <Loader2 className="size-4 animate-spin" /> : <PlugZap className="size-4" />}
                {t("mcp_connect.test_action")}
              </Button>
            </div>
            <FormError message={check.isError ? t(check.error.message === "unauthorized" ? "mcp_connect.test_unauthorized" : "mcp_connect.test_failed") : null} />
            {check.data && (
              <div role="status" className="space-y-2 rounded-md bg-muted/40 p-3 text-sm">
                <p className="font-medium">{t("mcp_connect.test_ok").replace("{tools}", String(check.data.tools.length)).replace("{collections}", String(check.data.collections.length))}</p>
                {check.data.collections.length > 0 && (
                  <ul className="list-disc pl-5 text-xs text-muted-foreground">
                    {check.data.collections.map((c) => <li key={c.name}>{c.name} · {t(c.access === "write" ? "integrations.read_write" : "integrations.read_only")}</li>)}
                  </ul>
                )}
                <ul aria-label={t("mcp_connect.test_tools")} className="flex flex-wrap gap-1">
                  {check.data.tools.map((name) => <li key={name}><code className="rounded bg-background px-1.5 py-0.5 font-mono text-xs">{name}</code></li>)}
                </ul>
              </div>
            )}
          </form>
        </Step>
      </ol>
    </FormSection>
  );
}

/** One numbered step of the connection guide. */
function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <li className="flex gap-3">
      <span aria-hidden className="flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-medium tabular-nums">{n}</span>
      <div className="min-w-0 flex-1 space-y-1.5">
        <h3 className="font-medium leading-6">{title}</h3>
        {children}
      </div>
    </li>
  );
}

/** REST examples built from a collection the member picks. */
function RestCard() {
  const { t } = useI18n();
  const cols = useQuery({ queryKey: ["collections", "options"], queryFn: ({ signal }) => collections.options(signal), staleTime: 30_000 });
  const mine = (cols.data?.items ?? []).filter((c) => c.member !== false);
  const [picked, setPicked] = useState<string | null>(null);
  const id = picked ?? mine[0]?.id ?? null;
  const collection = useQuery({ queryKey: ["collections", id], queryFn: () => collections.get(id!), enabled: !!id });
  const names = Object.fromEntries(mine.map((c) => [c.id, c.name]));
  return (
    <FormSection id="rest" title={t("integrations.docs")} description={t("integrations.docs_desc")} wide>
      <div className="space-y-4">
        {cols.isPending ? <ListSkeleton rows={1} /> : cols.isError ? <QueryErrorState error={cols.error} onRetry={() => void cols.refetch()} /> : !id ? (
          <p className="text-sm text-muted-foreground">{t("keys.no_collections")}</p>
        ) : (
          <>
            <Select value={id} items={names} onValueChange={(v) => setPicked(String(v))}>
              <SelectTrigger className="w-72 max-w-full" aria-label={t("integrations.docs_collection")}><span className="text-muted-foreground">{t("integrations.docs_collection")}</span><SelectValue /></SelectTrigger>
              <SelectContent>{mine.map((c) => <SelectItem key={c.id} value={c.id}>{c.name}</SelectItem>)}</SelectContent>
            </Select>
            {collection.data ? <RestExamples collection={collection.data} /> : collection.isError ? <QueryErrorState error={collection.error} onRetry={() => void collection.refetch()} /> : <ListSkeleton rows={3} />}
          </>
        )}
      </div>
    </FormSection>
  );
}

export default function ApiAccessPage() {
  const { t } = useI18n();
  // A key's secret shows once: while this page stays open, the examples use it.
  const [secret, setSecret] = useState<string | null>(null);
  return (
    <>
      <PageHeader title={t("nav.api_access")} description={t("api_access.subtitle")} />
      <PageBody>
        <FormSections>
          <ApiKeysCard onCreated={setSecret} />
          <McpConnectCard secret={secret} />
          <RestCard />
        </FormSections>
      </PageBody>
    </>
  );
}
