"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, KeyRound, Plug, Send, Trash2, Webhook as WebhookIcon } from "lucide-react";
import { toast } from "sonner";
import { FormError, FormSection, FormSections, IconAction, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
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
import { WEBHOOK_EVENTS, type Collection, type WebhookEvent } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

function CopyButton({ text }: { text: string }) {
  const { t } = useI18n();
  const [done, setDone] = useState(false);
  return (
    <Button type="button" size="sm" variant="outline" onClick={() => void navigator.clipboard.writeText(text).then(() => { setDone(true); setTimeout(() => setDone(false), 1500); })}>
      {done ? <Check className="size-4" /> : <Copy className="size-4" />}{t(done ? "integrations.copied" : "integrations.copy")}
    </Button>
  );
}

/** A secret shown once, right after it was made: the server keeps only its hash (keys) or sealed (webhooks). */
function Secret({ value, onClose }: { value: string; onClose: () => void }) {
  const { t } = useI18n();
  return (
    <div role="status" className="space-y-2 rounded-md border border-warn/40 bg-warn/10 p-3 text-sm">
      <p className="font-medium">{t("integrations.secret_once")}</p>
      <code className="block break-all rounded bg-background px-2 py-1.5 font-mono text-xs">{value}</code>
      <div className="flex gap-2">
        <CopyButton text={value} />
        <Button type="button" size="sm" variant="ghost" onClick={onClose}>{t("integrations.secret_saved")}</Button>
      </div>
    </div>
  );
}

function Keys({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["keys", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => tools.keys(collection.id) });
  const [name, setName] = useState("");
  const [canWrite, setCanWrite] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<{ id: string; name: string } | null>(null);
  const create = useMutation({
    mutationFn: () => tools.createKey(collection.id, name.trim(), canWrite),
    onSuccess: (k) => { setSecret(k.secret); setName(""); setCanWrite(false); void client.invalidateQueries({ queryKey: key }); },
  });
  const items = list.data?.items ?? [];
  return (
    <FormSection title={t("integrations.keys")} description={t("integrations.keys_desc")}>
      <div className="space-y-3">
        {secret && <Secret value={secret} onClose={() => setSecret(null)} />}
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
                <IconAction label={t("integrations.revoke")} target={k.name} className="text-destructive" onClick={() => setRevoking(k)}><Trash2 className="size-4" /></IconAction>
              </li>
            ))}
          </ul>
        )}
        <form className="flex flex-wrap items-end gap-3" onSubmit={(e) => { e.preventDefault(); if (name.trim()) create.mutate(); }}>
          <div className="flex min-w-48 flex-1 flex-col gap-1.5">
            <Label htmlFor="key-name">{t("integrations.key_name")}</Label>
            <Input id="key-name" maxLength={80} value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <label className="flex h-[var(--control-h)] items-center gap-2 text-sm">
            <Checkbox checked={canWrite} onCheckedChange={(on) => setCanWrite(on === true)} />
            {t("integrations.can_write")}
          </label>
          <Button type="submit" disabled={!name.trim() || create.isPending}><KeyRound className="size-4" />{t("integrations.create_key")}</Button>
        </form>
        <FormError message={create.isError ? errorMessage(create.error, "common.save_failed", t) : null} />
      </div>
      <ConfirmDialog
        open={!!revoking}
        onOpenChange={(o) => !o && setRevoking(null)}
        title={t("integrations.revoke_title").replace("{name}", revoking?.name ?? "")}
        description={t("integrations.revoke_desc")}
        confirmLabel={t("integrations.revoke")}
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
                  <IconAction label={t("common.delete")} target={w.url} className="text-destructive" onClick={() => setRemoving(w)}><Trash2 className="size-4" /></IconAction>
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

function Code({ children }: { children: string }) {
  return (
    <div className="relative">
      <pre className="overflow-x-auto rounded-md border bg-muted/40 p-3 pr-24 font-mono text-xs leading-relaxed">{children}</pre>
      <div className="absolute right-2 top-2"><CopyButton text={children} /></div>
    </div>
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
          <Code>{`curl ${auth} \\\n  "${base}/v1/records?collection_id=${collection.id}&page=1&limit=50&sort=updated_at&order=desc"`}</Code>
          <p className="text-xs text-muted-foreground">{t("integrations.docs_list_hint")}</p>
        </div>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_create")}</h3>
          <Code>{`curl ${auth} -H "Content-Type: application/json" \\\n  -d '${JSON.stringify({ collection_id: collection.id, data: example })}' \\\n  "${base}/v1/records"`}</Code>
        </div>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_update")}</h3>
          <Code>{`curl ${auth} -X PATCH -H "Content-Type: application/json" \\\n  -d '${JSON.stringify({ data: example, version: 1 })}' \\\n  "${base}/v1/records/<record-id>"`}</Code>
          <p className="text-xs text-muted-foreground">{t("integrations.docs_update_hint")}</p>
        </div>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_delete")}</h3>
          <Code>{`curl ${auth} -H "Content-Type: application/json" \\\n  -d '{"ids":["<record-id>"]}' "${base}/v1/records/delete"`}</Code>
        </div>
        <div className="space-y-1.5">
          <h3 className="font-medium">{t("integrations.docs_signature")}</h3>
          <p className="text-muted-foreground">{t("integrations.docs_signature_desc")}</p>
          <Code>{`// Node.js
const [t, v1] = req.headers["x-dataset-signature"].split(",").map((p) => p.split("=")[1]);
const expected = crypto.createHmac("sha256", process.env.DATASET_WEBHOOK_SECRET).update(\`\${t}.\${rawBody}\`).digest("hex");
const ok = crypto.timingSafeEqual(Buffer.from(v1), Buffer.from(expected)) && Date.now() / 1000 - Number(t) < 300;`}</Code>
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
          <Keys collection={collection} />
          <Webhooks collection={collection} />
          <Docs collection={collection} />
        </FormSections>
      ) : <EmptyState icon={Plug} title={t("error.forbidden")} description={t("integrations.owners_only")} />}
    </CollectionPage>
  );
}
