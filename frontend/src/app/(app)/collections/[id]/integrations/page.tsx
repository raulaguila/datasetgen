"use client";

import { useState, useSyncExternalStore } from "react";
import { Tabs } from "@base-ui/react/tabs";
import Link from "next/link";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, KeyRound, Link2, Pause, Pencil, Play, Plug, Send, Trash2, Webhook as WebhookIcon } from "lucide-react";
import { toast } from "sonner";
import { FormError, FormSection, FormSections, IconAction, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { CodeBlock, CopyButton, Secret } from "@/components/secret";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { tools } from "@/lib/api/endpoints";
import { WEBHOOK_EVENTS, type Collection, type ShareLink, type Webhook, type WebhookEvent } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { McpActivity, McpTools } from "@/components/mcp-tools";
import { CollectionPage } from "../_parts";

/** The keys that reach this collection, to see and cut off: members make and edit their keys on the API access page. */
function Keys({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["keys", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => tools.keys(collection.id) });
  const [revoking, setRevoking] = useState<{ id: string; name: string } | null>(null);
  const items = list.data?.items ?? [];
  return (
    <FormSection id="keys" title={t("integrations.keys")} description={t("integrations.keys_desc")}>
      <div className="space-y-3">
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <EmptyState compact title={t("integrations.keys_empty")} />
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
                    {k.expires_at && new Date(k.expires_at) > new Date() && ` · ${t("keys.expires_at").replace("{at}", format(k.expires_at))}`}
                  </p>
                  {k.expires_at && new Date(k.expires_at) <= new Date() && <Badge variant="destructive">{t("keys.expired")}</Badge>}
                </div>
                <IconAction label={t("integrations.key_drop")} target={k.name} className="text-destructive" onClick={() => setRevoking(k)}><Trash2 className="size-4" /></IconAction>
              </li>
            ))}
          </ul>
        )}
        <Link href={`/api-access?collection=${collection.id}`} className={buttonVariants({ variant: "outline" })}><KeyRound className="size-4" />{t("integrations.key_new_here")}</Link>
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

const SHARE_DAYS = ["7", "30", "90", "0"];

/** Public links: a form anyone can fill, or a read-only list. The link is the access. */
function Shares({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const key = ["shares", collection.id];
  const list = useQuery({ queryKey: key, queryFn: () => tools.shares(collection.id) });
  const [removing, setRemoving] = useState<{ id: string; kind: ShareLink["kind"] } | null>(null);
  const [days, setDays] = useState("30");
  const dayNames: Record<string, string> = Object.fromEntries(SHARE_DAYS.map((d) => [d, d === "0" ? t("keys.expires_never") : t("keys.expires_days").replace("{n}", d)]));
  const create = useMutation({
    mutationFn: (kind: ShareLink["kind"]) => tools.createShare(collection.id, kind, Number(days)),
    onSuccess: () => void client.invalidateQueries({ queryKey: key }),
  });
  const items = list.data?.items ?? [];
  const urlOf = (s: ShareLink) => `${window.location.origin}/${s.kind === "form" ? "f" : "s"}/${s.token}`;
  const kindName = (k: ShareLink["kind"]) => t(k === "form" ? "share.form" : "share.view");
  return (
    <FormSection id="shares" title={t("share.title")} description={`${t("share.desc")} ${t("share.private_note")}`}>
      <div className="space-y-3">
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <EmptyState compact title={t("share.empty")} />
        ) : (
          <ul className="divide-y rounded-md border" aria-label={t("share.title")}>
            {items.map((s) => (
              <li key={s.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-3 py-2">
                <div className="min-w-48 flex-1">
                  <p className="text-sm font-medium">{kindName(s.kind)}</p>
                  <p className="truncate font-mono text-xs text-muted-foreground">{urlOf(s)}</p>
                  <p className="text-xs text-muted-foreground">
                    {s.last_used_at ? t("integrations.used_at").replace("{at}", format(s.last_used_at)) : t("integrations.never_used")}
                    {" · "}
                    {!s.expires_at ? t("share.no_end") : new Date(s.expires_at) < new Date() ? <span className="text-destructive">{t("share.expired")}</span> : t("share.expires_at").replace("{at}", format(s.expires_at))}
                  </p>
                </div>
                <CopyButton text={urlOf(s)} />
                <Button size="sm" variant="ghost" nativeButton={false} render={<a href={urlOf(s)} target="_blank" rel="noreferrer" />}><ExternalLink className="size-4" />{t("share.open")}</Button>
                <IconAction label={t("integrations.revoke")} target={kindName(s.kind)} className="text-destructive" onClick={() => setRemoving(s)}><Trash2 className="size-4" /></IconAction>
              </li>
            ))}
          </ul>
        )}
        <div className="flex flex-wrap gap-2">
          <Select value={days} items={dayNames} onValueChange={(v) => setDays(String(v))}>
            <SelectTrigger className="w-44" aria-label={t("share.validity")}><SelectValue /></SelectTrigger>
            <SelectContent>{SHARE_DAYS.map((d) => <SelectItem key={d} value={d}>{dayNames[d]}</SelectItem>)}</SelectContent>
          </Select>
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
  // "new" opens an empty form; a hook opens it filled, to change its address or events.
  const [editing, setEditing] = useState<Webhook | "new" | null>(null);
  const close = () => { setEditing(null); setUrl(""); setEvents([...WEBHOOK_EVENTS]); };
  const edit = (w: Webhook) => { save.reset(); setEditing(w); setUrl(w.url); setEvents(w.events); };
  const save = useMutation({
    mutationFn: async () => {
      if (editing && editing !== "new") return { ...(await tools.updateWebhook(collection.id, editing.id, url.trim(), events, !editing.paused_at)), secret: null };
      return tools.createWebhook(collection.id, url.trim(), events);
    },
    onSuccess: (w) => { if (w.secret) setSecret(w.secret); else toast.success(t("common.saved")); close(); void client.invalidateQueries({ queryKey: key }); },
  });
  const pause = useMutation({
    mutationFn: (w: Webhook) => tools.updateWebhook(collection.id, w.id, w.url, w.events, !!w.paused_at),
    onSuccess: (w) => { toast.success(t(w.paused_at ? "integrations.hook_paused_done" : "integrations.hook_resumed")); void client.invalidateQueries({ queryKey: key }); },
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const eventName = (e: WebhookEvent) => t(`webhook.event.${e}`);
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
    <FormSection id="webhooks" title={t("integrations.webhooks")} description={t("integrations.webhooks_desc")}>
      <div className="space-y-3">
        {secret && <Secret value={secret} onClose={() => setSecret(null)} />}
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <EmptyState compact title={t("integrations.webhooks_empty")} />
        ) : (
          <ul className="divide-y rounded-md border">
            {items.map((w) => {
              const failed = !!w.last_error;
              return (
                <li key={w.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2">
                  <div className="min-w-48 flex-1">
                    <p className="flex min-w-0 items-center gap-2 text-sm">
                      <span className="truncate font-mono">{w.url}</span>
                      {w.paused_at ? <Badge variant="outline">{t("integrations.hook_paused")}</Badge> : failed && <Badge variant="destructive">{t("integrations.hook_failing")}</Badge>}
                    </p>
                    {w.paused_at && <p className="text-xs text-muted-foreground">{t("integrations.hook_paused_hint").replace("{at}", format(w.paused_at))}</p>}
                    <p className="text-xs text-muted-foreground">
                      {w.events.map(eventName).join(", ")}
                      {w.last_at && (
                        <span className={failed ? "text-destructive" : undefined}>
                          {` · ${format(w.last_at)}: ${failed ? w.last_error : `HTTP ${w.last_status}`}`}
                        </span>
                      )}
                    </p>
                  </div>
                  <Button type="button" size="sm" variant="outline" disabled={test.isPending} onClick={() => test.mutate(w.id)}><Send className="size-4" />{t("integrations.test")}</Button>
                  <Button type="button" size="sm" variant="ghost" aria-expanded={open === w.id} onClick={() => setOpen(open === w.id ? null : w.id)}>{t("deliveries.title")}</Button>
                  <Button type="button" size="sm" variant="ghost" disabled={pause.isPending} onClick={() => pause.mutate(w)}>
                    {w.paused_at ? <><Play className="size-4" />{t("integrations.hook_resume")}</> : <><Pause className="size-4" />{t("integrations.hook_pause")}</>}
                  </Button>
                  <IconAction label={t("common.edit")} target={w.url} onClick={() => edit(w)}><Pencil className="size-4" /></IconAction>
                  <IconAction label={t("common.delete")} target={w.url} className="text-destructive" onClick={() => setRemoving(w)}><Trash2 className="size-4" /></IconAction>
                  {open === w.id && <div className="basis-full"><Deliveries collection={collection} hookId={w.id} /></div>}
                </li>
              );
            })}
          </ul>
        )}
        {editing ? (
          <form className="space-y-3 rounded-md border p-3" aria-label={t(editing === "new" ? "integrations.hook_new" : "integrations.hook_edit")} onSubmit={(e) => { e.preventDefault(); if (url.trim() && events.length) save.mutate(); }}>
            {editing !== "new" && <p className="text-xs text-muted-foreground">{t("integrations.hook_edit_hint")}</p>}
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="hook-url">{t("integrations.hook_url")}</Label>
              <Input id="hook-url" type="url" inputMode="url" placeholder="https://" maxLength={2000} value={url} onChange={(e) => setUrl(e.target.value)} autoFocus />
            </div>
            <fieldset className="flex flex-wrap gap-x-4 gap-y-2">
              <legend className="mb-1.5 text-sm font-medium">{t("integrations.events")}</legend>
              {WEBHOOK_EVENTS.map((e) => (
                <label key={e} className="flex items-center gap-2 text-sm">
                  <Checkbox checked={events.includes(e)} onCheckedChange={(on) => toggle(e, on === true)} />{eventName(e)}
                </label>
              ))}
            </fieldset>
            <FormError message={save.isError ? errorMessage(save.error, "common.save_failed", t) : null} />
            <div className="flex flex-wrap gap-2">
              <Button type="submit" disabled={!url.trim() || !events.length || save.isPending}>
                {editing === "new" ? <><WebhookIcon className="size-4" />{t("integrations.create_hook")}</> : t("common.save")}
              </Button>
              <Button type="button" variant="ghost" onClick={() => { save.reset(); close(); }}>{t("common.cancel")}</Button>
            </div>
          </form>
        ) : (
          <Button variant="outline" onClick={() => { save.reset(); setEditing("new"); }}><WebhookIcon className="size-4" />{t("integrations.hook_new")}</Button>
        )}
        <details className="text-sm">
          <summary className="cursor-pointer font-medium">{t("integrations.docs_signature")}</summary>
          <div className="mt-2 space-y-1.5">
            <p className="text-muted-foreground">{t("integrations.docs_signature_desc")}</p>
            <CodeBlock>{`// Node.js
const [t, v1] = req.headers["x-dataset-signature"].split(",").map((p) => p.split("=")[1]);
const expected = crypto.createHmac("sha256", process.env.DATASET_WEBHOOK_SECRET).update(\`\${t}.\${rawBody}\`).digest("hex");
const ok = crypto.timingSafeEqual(Buffer.from(v1), Buffer.from(expected)) && Date.now() / 1000 - Number(t) < 300;`}</CodeBlock>
          </div>
        </details>
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

const TABS = [["shares", "share.title"], ["keys", "integrations.tab_keys"], ["mcp", "integrations.tab_mcp"], ["webhooks", "integrations.webhooks"]] as const;
type Tab = (typeof TABS)[number][0];

const onHash = (cb: () => void) => { window.addEventListener("hashchange", cb); return () => window.removeEventListener("hashchange", cb); };

/** One tab per kind of integration; the address hash (#keys) opens a tab directly. */
function IntegrationTabs({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const hash = useSyncExternalStore(onHash, () => window.location.hash.slice(1), () => "");
  const tab: Tab = TABS.some(([id]) => id === hash) ? (hash as Tab) : "shares";
  const pick = (v: Tab) => {
    history.replaceState(null, "", `#${v}`);
    window.dispatchEvent(new HashChangeEvent("hashchange"));
  };
  return (
    <Tabs.Root value={tab} onValueChange={(v) => pick(v as Tab)}>
      <Tabs.List aria-label={t("integrations.sections")} className="mb-6 flex gap-1 overflow-x-auto border-b">
        {TABS.map(([id, key]) => (
          <Tabs.Tab key={id} value={id} className="-mb-px shrink-0 whitespace-nowrap border-b-2 border-transparent px-3 py-2 text-sm text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring data-[active]:border-primary data-[active]:font-medium data-[active]:text-foreground">
            {t(key)}
          </Tabs.Tab>
        ))}
      </Tabs.List>
      <Tabs.Panel value="shares" className="outline-none"><FormSections><Shares collection={collection} /></FormSections></Tabs.Panel>
      <Tabs.Panel value="keys" className="outline-none"><FormSections><Keys collection={collection} /></FormSections></Tabs.Panel>
      <Tabs.Panel value="mcp" className="outline-none"><FormSections><McpTools collection={collection} /><McpActivity collection={collection} /></FormSections></Tabs.Panel>
      <Tabs.Panel value="webhooks" className="outline-none"><FormSections><Webhooks collection={collection} /></FormSections></Tabs.Panel>
    </Tabs.Root>
  );
}

export default function CollectionIntegrationsPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.integrations" description="integrations.subtitle">
      {(collection) => roleAllows(collection.role, "owner") ? (
        <>
          <IntegrationTabs collection={collection} />
        </>
      ) : <EmptyState icon={Plug} title={t("error.forbidden")} description={t("integrations.owners_only")} />}
    </CollectionPage>
  );
}
