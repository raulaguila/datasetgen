"use client";

import { useState } from "react";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, Link2, Plug, Send, Trash2, Webhook as WebhookIcon } from "lucide-react";
import { toast } from "sonner";
import { FormError, FormSection, FormSections, IconAction, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { CodeBlock, CopyButton, Secret } from "@/components/secret";
import { EmptyState } from "@/components/empty-state";
import { fieldsOf } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { tools } from "@/lib/api/endpoints";
import { WEBHOOK_EVENTS, type Collection, type ShareLink, type WebhookEvent } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { McpActivity, McpTools } from "@/components/mcp-tools";
import { CollectionPage } from "../_parts";

/** The keys that reach this collection: members make them on their profile; an owner can take the collection out of one. */
function Keys({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["keys", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => tools.keys(collection.id) });
  const [revoking, setRevoking] = useState<{ id: string; name: string } | null>(null);
  const items = list.data?.items ?? [];
  return (
    <FormSection title={t("integrations.keys")} description={t("integrations.keys_desc")}>
      <div className="space-y-3">
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <p className="rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">{t("integrations.keys_empty")}</p>
        ) : (
          <ul className="divide-y rounded-md border">
            {items.map((k) => (
              <li key={k.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
                <div className="min-w-40 flex-1">
                  <p className="flex items-center gap-2 text-sm font-medium">{k.name}<Badge variant="outline">{t(k.can_write ? "integrations.read_write" : "integrations.read_only")}</Badge></p>
                  <p className="text-xs text-muted-foreground">
                    <span className="font-mono">{k.prefix}…</span>
                    {k.creator_name && ` · ${k.creator_name}`}
                    {` · ${k.last_used_at ? t("integrations.used_at").replace("{at}", format(k.last_used_at)) : t("integrations.never_used")}`}
                  </p>
                </div>
                <IconAction label={t("integrations.key_drop")} target={k.name} className="text-destructive" onClick={() => setRevoking(k)}><Trash2 className="size-4" /></IconAction>
              </li>
            ))}
          </ul>
        )}
        <p className="text-sm text-muted-foreground">
          {t("integrations.keys_profile")}{" "}
          <Link href="/api-access" className="text-primary underline-offset-2 hover:underline">{t("integrations.keys_profile_link")}</Link>
        </p>
      </div>
      <ConfirmDialog
        open={!!revoking}
        onOpenChange={(o) => !o && setRevoking(null)}
        title={t("integrations.key_drop_title").replace("{name}", revoking?.name ?? "")}
        description={t("integrations.key_drop_desc")}
        confirmLabel={t("integrations.key_drop")}
        destructive
        onConfirm={async () => {
          if (!revoking) return;
          await tools.deleteKey(collection.id, revoking.id).then(() => client.invalidateQueries({ queryKey: key }), (err) => toast.error(errorMessage(err, "common.save_failed", t)));
          setRevoking(null);
        }}
      />
    </FormSection>
  );
}

/** Public links: a form anyone can fill, or a read-only list. The link is the access. */
function Shares({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["shares", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => tools.shares(collection.id) });
  const [removing, setRemoving] = useState<{ id: string; kind: ShareLink["kind"] } | null>(null);
  const create = useMutation({
    mutationFn: (kind: ShareLink["kind"]) => tools.createShare(collection.id, kind),
    onSuccess: () => void client.invalidateQueries({ queryKey: key }),
  });
  const items = list.data?.items ?? [];
  const urlOf = (s: ShareLink) => `${window.location.origin}/${s.kind === "form" ? "f" : "s"}/${s.token}`;
  const kindName = (k: ShareLink["kind"]) => t(k === "form" ? "share.form" : "share.view");
  return (
    <FormSection title={t("share.title")} description={`${t("share.desc")} ${t("share.private_note")}`}>
      <div className="space-y-3">
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <p className="rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">{t("share.empty")}</p>
        ) : (
          <ul className="divide-y rounded-md border" aria-label={t("share.title")}>
            {items.map((s) => (
              <li key={s.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2">
                <div className="min-w-48 flex-1">
                  <p className="text-sm font-medium">{kindName(s.kind)}</p>
                  <p className="truncate font-mono text-xs text-muted-foreground">{urlOf(s)}</p>
                  <p className="text-xs text-muted-foreground">{s.last_used_at ? t("integrations.used_at").replace("{at}", format(s.last_used_at)) : t("integrations.never_used")}</p>
                </div>
                <CopyButton text={urlOf(s)} />
                <Button size="sm" variant="ghost" nativeButton={false} render={<a href={urlOf(s)} target="_blank" rel="noreferrer" />}><ExternalLink className="size-4" />{t("share.open")}</Button>
                <IconAction label={t("integrations.revoke")} target={kindName(s.kind)} className="text-destructive" onClick={() => setRemoving(s)}><Trash2 className="size-4" /></IconAction>
              </li>
            ))}
          </ul>
        )}
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" disabled={create.isPending} onClick={() => create.mutate("form")}><Link2 className="size-4" />{t("share.create_form")}</Button>
          <Button variant="outline" disabled={create.isPending} onClick={() => create.mutate("view")}><Link2 className="size-4" />{t("share.create_view")}</Button>
        </div>
        <FormError message={create.isError ? errorMessage(create.error, "common.save_failed", t) : null} />
      </div>
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("share.revoke_title").replace("{name}", removing ? kindName(removing.kind) : "")}
        description={t("share.revoke_desc")}
        confirmLabel={t("integrations.revoke")}
        destructive
        onConfirm={async () => {
          if (!removing) return;
          await tools.deleteShare(collection.id, removing.id).then(() => client.invalidateQueries({ queryKey: key }), (err) => toast.error(errorMessage(err, "common.save_failed", t)));
          setRemoving(null);
        }}
      />
    </FormSection>
  );
}

/** A hook's newest calls; a failed one can be sent again with the same body. */
function Deliveries({ collection, hookId }: { collection: Collection; hookId: string }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["webhooks", collection.id, hookId, "deliveries"];
  const list = useQuery({ queryKey: key, queryFn: () => tools.deliveries(collection.id, hookId) });
  const again = useMutation({
    mutationFn: (deliveryId: string) => tools.redeliver(collection.id, hookId, deliveryId),
    onSuccess: (d) => {
      if (d.error) toast.error(t("integrations.test_failed").replace("{error}", d.error));
      else toast.success(t("integrations.test_ok").replace("{status}", String(d.status ?? "")));
      void client.invalidateQueries({ queryKey: ["webhooks", collection.id] });
    },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  if (list.isPending) return <ListSkeleton rows={1} />;
  const items = list.data?.items ?? [];
  if (!items.length) return <p className="text-xs text-muted-foreground">{t("deliveries.empty")}</p>;
  return (
    <ul className="max-h-64 divide-y overflow-auto rounded border text-xs" aria-label={t("deliveries.title")}>
      {items.map((d) => {
        const ok = !d.error && d.status !== undefined && d.status < 300;
        return (
          <li key={d.id} className="flex items-center gap-2 px-2 py-1.5">
            <span className={ok ? "text-success" : "text-destructive"}>{d.status ? `HTTP ${d.status}` : t("deliveries.no_answer")}</span>
            <span className="font-mono">{d.event}</span>
            <span className="text-muted-foreground">{format(d.created_at)}</span>
            {d.error && <span className="min-w-0 flex-1 truncate text-destructive" title={d.error}>{d.error}</span>}
            <Button size="sm" variant="ghost" className="ml-auto" disabled={again.isPending} onClick={() => again.mutate(d.id)}>{t("deliveries.redeliver")}</Button>
          </li>
        );
      })}
    </ul>
  );
}

function Webhooks({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["webhooks", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => tools.webhooks(collection.id) });
  const [url, setUrl] = useState("");
  const [events, setEvents] = useState<WebhookEvent[]>([...WEBHOOK_EVENTS]);
  const [secret, setSecret] = useState<string | null>(null);
  const [removing, setRemoving] = useState<{ id: string; url: string } | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const create = useMutation({
    mutationFn: () => tools.createWebhook(collection.id, url.trim(), events),
    onSuccess: (w) => { setSecret(w.secret); setUrl(""); setEvents([...WEBHOOK_EVENTS]); void client.invalidateQueries({ queryKey: key }); },
  });
  const test = useMutation({
    mutationFn: (hookId: string) => tools.testWebhook(collection.id, hookId),
    onSuccess: (w) => {
      if (w.last_error) toast.error(t("integrations.test_failed").replace("{error}", w.last_error));
      else toast.success(t("integrations.test_ok").replace("{status}", String(w.last_status ?? "")));
      void client.invalidateQueries({ queryKey: key });
    },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const items = list.data?.items ?? [];
  const toggle = (e: WebhookEvent, on: boolean) => setEvents((cur) => (on ? [...cur, e] : cur.filter((x) => x !== e)));
  return (
    <FormSection title={t("integrations.webhooks")} description={t("integrations.webhooks_desc")}>
      <div className="space-y-3">
        {secret && <Secret value={secret} onClose={() => setSecret(null)} />}
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <p className="rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">{t("integrations.webhooks_empty")}</p>
        ) : (
          <ul className="divide-y rounded-md border">
            {items.map((w) => {
              const failed = !!w.last_error;
              return (
                <li key={w.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
                  <div className="min-w-48 flex-1">
                    <p className="truncate font-mono text-sm">{w.url}</p>
                    <p className="text-xs text-muted-foreground">
                      {w.events.join(", ")}
                      {w.last_at && (
                        <span className={failed ? "text-destructive" : undefined}>
                          {` · ${format(w.last_at)}: ${failed ? w.last_error : `HTTP ${w.last_status}`}`}
                        </span>
                      )}
                    </p>
                  </div>
                  <Button type="button" size="sm" variant="outline" disabled={test.isPending} onClick={() => test.mutate(w.id)}><Send className="size-4" />{t("integrations.test")}</Button>
                  <Button type="button" size="sm" variant="ghost" aria-expanded={open === w.id} onClick={() => setOpen(open === w.id ? null : w.id)}>{t("deliveries.title")}</Button>
                  <IconAction label={t("common.delete")} target={w.url} className="text-destructive" onClick={() => setRemoving(w)}><Trash2 className="size-4" /></IconAction>
                  {open === w.id && <div className="basis-full"><Deliveries collection={collection} hookId={w.id} /></div>}
                </li>
              );
            })}
          </ul>
        )}
        <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); if (url.trim() && events.length) create.mutate(); }}>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="hook-url">{t("integrations.hook_url")}</Label>
            <Input id="hook-url" type="url" inputMode="url" placeholder="https://" maxLength={2000} value={url} onChange={(e) => setUrl(e.target.value)} />
          </div>
          <fieldset className="flex flex-wrap gap-x-4 gap-y-2">
            <legend className="mb-1.5 text-sm font-medium">{t("integrations.events")}</legend>
            {WEBHOOK_EVENTS.map((e) => (
              <label key={e} className="flex items-center gap-2 font-mono text-xs">
                <Checkbox checked={events.includes(e)} onCheckedChange={(on) => toggle(e, on === true)} />{e}
              </label>
            ))}
          </fieldset>
          <Button type="submit" disabled={!url.trim() || !events.length || create.isPending}><WebhookIcon className="size-4" />{t("integrations.create_hook")}</Button>
          <FormError message={create.isError ? errorMessage(create.error, "common.save_failed", t) : null} />
        </form>
      </div>
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("integrations.hook_remove_title")}
        description={removing?.url}
        confirmLabel={t("common.delete")}
        destructive
        onConfirm={async () => {
          if (!removing) return;
          await tools.deleteWebhook(collection.id, removing.id).then(() => client.invalidateQueries({ queryKey: key }), (err) => toast.error(errorMessage(err, "common.save_failed", t)));
          setRemoving(null);
        }}
      />
    </FormSection>
  );
}

/** How a program reads and writes this collection, with examples built from its fields. */
function Docs({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  // The web address answers the API too: the proxy passes API keys through.
  // Drawn only after the collection loads in the browser, so window is there.
  const base = typeof window === "undefined" ? "/api/proxy" : `${window.location.origin}/api/proxy`;
  const example = Object.fromEntries(fieldsOf(collection.schema ?? null).filter((r) => !r.advanced).slice(0, 3).map((r) => [
    r.key,
    r.field.type === "number" || r.field.type === "integer" ? 1 : r.field.type === "boolean" ? true : r.field.type === "strings" ? ["a"] : r.field.type === "enum" ? r.field.options.split(",")[0]?.trim() ?? "" : r.field.type === "date" ? "2026-01-31" : "…",
  ]));
  const auth = `-H "Authorization: Bearer $DATASET_KEY"`;
  return (
    <FormSection title={t("integrations.docs")} description={t("integrations.docs_desc")} wide>
      <div className="space-y-4 text-sm">
        <p>{t("integrations.docs_base")} <code className="font-mono text-xs">{base}</code></p>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_list")}</h3>
          <CodeBlock>{`curl ${auth} \\\n  "${base}/v1/records?collection_id=${collection.id}&page=1&limit=50&sort=updated_at&order=desc"`}</CodeBlock>
          <p className="text-xs text-muted-foreground">{t("integrations.docs_list_hint")}</p>
        </div>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_create")}</h3>
          <CodeBlock>{`curl ${auth} -H "Content-Type: application/json" \\\n  -d '${JSON.stringify({ collection_id: collection.id, data: example })}' \\\n  "${base}/v1/records"`}</CodeBlock>
        </div>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_update")}</h3>
          <CodeBlock>{`curl ${auth} -X PATCH -H "Content-Type: application/json" \\\n  -d '${JSON.stringify({ data: example, version: 1 })}' \\\n  "${base}/v1/records/<record-id>"`}</CodeBlock>
          <p className="text-xs text-muted-foreground">{t("integrations.docs_update_hint")}</p>
        </div>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_delete")}</h3>
          <CodeBlock>{`curl ${auth} -H "Content-Type: application/json" \\\n  -d '{"ids":["<record-id>"]}' "${base}/v1/records/delete"`}</CodeBlock>
        </div>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_signature")}</h3>
          <p className="text-muted-foreground">{t("integrations.docs_signature_desc")}</p>
          <CodeBlock>{`// Node.js
const [t, v1] = req.headers["x-dataset-signature"].split(",").map((p) => p.split("=")[1]);
const expected = crypto.createHmac("sha256", process.env.DATASET_WEBHOOK_SECRET).update(\`\${t}.\${rawBody}\`).digest("hex");
const ok = crypto.timingSafeEqual(Buffer.from(v1), Buffer.from(expected)) && Date.now() / 1000 - Number(t) < 300;`}</CodeBlock>
        </div>
      </div>
    </FormSection>
  );
}

export default function CollectionIntegrationsPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.integrations" description="integrations.subtitle">
      {(collection) => roleAllows(collection.role, "owner") ? (
        <FormSections>
          <Shares collection={collection} />
          <Keys collection={collection} />
          <McpTools collection={collection} />
          <McpActivity collection={collection} />
          <Webhooks collection={collection} />
          <Docs collection={collection} />
        </FormSections>
      ) : <EmptyState icon={Plug} title={t("error.forbidden")} description={t("integrations.owners_only")} />}
    </CollectionPage>
  );
}
