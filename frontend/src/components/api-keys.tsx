"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, KeyRound, MoreHorizontal, Pencil, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { FormError, FormSection, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { Secret } from "@/components/secret";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { account, collections } from "@/lib/api/endpoints";
import type { ApiKey, KeyScope } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";

export type Access = "none" | "read" | "write";
/** The key being made (no id) or changed: its name and each collection's access. */
/** days: how long the key lasts from now, "0" for no end; "keep" leaves a saved key's end as it is. */
type Draft = { id?: string; name: string; access: Record<string, Access>; days: string };

const KEYS = ["me", "keys"] as const;

const draftOf = (k?: ApiKey): Draft => ({
  id: k?.id,
  name: k?.name ?? "",
  access: Object.fromEntries((k?.scopes ?? []).map((s) => [s.collection_id, s.can_write ? "write" : "read"])),
  days: k ? "keep" : "90",
});

export const DAYS = ["30", "90", "365", "0"];
/** The server's cap per member (maxKeys in the integration use case). */
const MAX_KEYS = 20;
/** A key ending within this many days shows a warning and a renew button. */
const WARN_DAYS = 14;
const DAY_MS = 24 * 60 * 60 * 1000;

export const scopesOf = (d: Pick<Draft, "access">): KeyScope[] =>
  Object.entries(d.access).filter(([, a]) => a !== "none").map(([collection_id, a]) => ({ collection_id, can_write: a === "write" }));

/** The member's own API keys: each reaches the collections picked, read only or read and write.
 * onCreated gets a new key's secret, shown this once, so the page can fill its examples with it. */
export function ApiKeysCard({ onCreated, prefill }: { onCreated?: (secret: string) => void; prefill?: string | null } = {}) {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const list = useQuery({ queryKey: KEYS, queryFn: account.keys });
  const cols = useQuery({ queryKey: ["collections", "options"], queryFn: ({ signal }) => collections.options(signal), staleTime: 30_000 });
  const mine = (cols.data?.items ?? []).filter((c) => c.member !== false);
  // Only collections the member holds count: a stale ?collection= link adds nothing.
  const scopesFor = (d: Draft) => scopesOf(d).filter((s) => mine.some((c) => c.id === s.collection_id));
  const [usage, setUsage] = useState<string | null>(null);
  // Days left are counted from when the page opened: a render stays pure.
  const [now] = useState(() => Date.now());
  // A link from a collection's Integrations opens a new key that reads it.
  const [draft, setDraft] = useState<Draft | null>(() => (prefill ? { ...draftOf(), access: { [prefill]: "read" } } : null));
  const [secret, setSecret] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<ApiKey | null>(null);
  const save = useMutation({
    mutationFn: (d: Draft) => (d.id
      ? account.updateKey(d.id, d.name.trim(), scopesFor(d), d.days === "keep" ? undefined : Number(d.days))
      : account.createKey(d.name.trim(), scopesFor(d), Number(d.days))),
    onSuccess: (k: ApiKey & { secret?: string }) => {
      if (k.secret) {
        setSecret(k.secret);
        onCreated?.(k.secret);
      }
      else toast.success(t("common.saved"));
      setDraft(null);
      void client.invalidateQueries({ queryKey: KEYS });
    },
  });
  const all = list.data?.items ?? [];
  // A long list gets a search: by the key's name or a collection it reaches.
  const [query, setQuery] = useState("");
  const q = query.trim().toLowerCase();
  const items = q ? all.filter((k) => k.name.toLowerCase().includes(q) || k.scopes?.some((s) => s.collection_name?.toLowerCase().includes(q))) : all;
  const picked = draft ? scopesFor(draft).length : 0;
  const full = all.length >= MAX_KEYS;
  const daysName: Record<string, string> = {
    keep: t("keys.renew_keep"),
    ...Object.fromEntries(DAYS.map((d) => [d, d === "0" ? t("keys.expires_never") : t("keys.expires_days").replace("{n}", d)])),
  };

  return (
    <FormSection id="keys" title={t("keys.title")} description={t("keys.desc")} action={!draft && <>
      {full && <span className="text-xs text-muted-foreground">{t("keys.full").replace("{n}", String(MAX_KEYS))}</span>}
      <Button variant="outline" disabled={full} onClick={() => { save.reset(); setDraft(draftOf()); }}><KeyRound className="size-4" />{t("keys.new")}</Button>
    </>}>
      <div className="space-y-3">
        {secret && <Secret value={secret} onClose={() => setSecret(null)} />}
        {secret && onCreated && <a href="#mcp" className="inline-block text-sm font-medium text-primary underline-offset-2 hover:underline">{t("keys.next_step")} ↓</a>}
        {draft && (
          <form className="space-y-3 rounded-md border p-3" onSubmit={(e) => { e.preventDefault(); if (draft.name.trim() && picked > 0) save.mutate(draft); }}>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="key-name">{t("integrations.key_name")}</Label>
              <Input id="key-name" maxLength={80} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} autoFocus />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label id="key-days">{t("keys.expires")}</Label>
              <Select value={draft.days} items={daysName} onValueChange={(v) => setDraft({ ...draft, days: v as string })}>
                <SelectTrigger className="w-56" aria-labelledby="key-days"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {draft.id && <SelectItem value="keep">{daysName.keep}</SelectItem>}
                  {DAYS.map((d) => <SelectItem key={d} value={d}>{daysName[d]}</SelectItem>)}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t(draft.id ? "keys.renew_hint" : "keys.expires_hint")}</p>
            </div>
            <CollectionAccess access={draft.access} onChange={(access) => setDraft({ ...draft, access })} />
            <FormError message={save.isError ? errorMessage(save.error, "common.save_failed", t) : null} />
            <div className="flex flex-wrap items-center gap-2">
              <Button type="submit" disabled={!draft.name.trim() || picked === 0 || save.isPending}>{draft.id ? t("common.save") : <><KeyRound className="size-4" />{t("integrations.create_key")}</>}</Button>
              <Button type="button" variant="ghost" onClick={() => setDraft(null)}>{t("common.cancel")}</Button>
              <span className="text-xs text-muted-foreground">{t("keys.picked").replace("{n}", String(picked))}</span>
            </div>
          </form>
        )}
        {all.length > 10 && (
          <div className="relative max-w-sm">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden />
            <Input type="search" className="pl-8" placeholder={t("keys.find")} aria-label={t("keys.find")} value={query} onChange={(e) => setQuery(e.target.value)} />
          </div>
        )}
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <EmptyState compact title={t(q ? "keys.search_empty" : "integrations.keys_empty")} />
        ) : (
          <ul className="divide-y rounded-md border">
            {items.map((k) => {
              const left = k.expires_at ? Math.ceil((new Date(k.expires_at).getTime() - now) / DAY_MS) : null;
              return (
              <li key={k.id} className="flex flex-wrap items-start gap-2 px-3 py-2">
                <div className="min-w-0 flex-1 space-y-1.5">
                  <p className="break-words text-sm font-medium">{k.name}</p>
                  <p className="text-xs text-muted-foreground">
                    <span className="font-mono">{k.prefix}…</span>
                    {` · ${k.last_used_at ? t("integrations.used_at").replace("{at}", format(k.last_used_at)) : t("integrations.never_used")}`}
                    {k.expires_at && new Date(k.expires_at) > new Date() && ` · ${t("keys.expires_at").replace("{at}", format(k.expires_at))}`}
                  </p>
                  {left !== null && left <= WARN_DAYS && (
                    <p className="flex flex-wrap items-center gap-2">
                      {left <= 0 ? <Badge variant="destructive">{t("keys.expired")}</Badge> : <Badge variant="outline">{t("keys.ends_in").replace("{n}", String(left))}</Badge>}
                      <Button type="button" size="sm" variant="link" className="h-auto p-0" onClick={() => { save.reset(); setDraft({ ...draftOf(k), days: "90" }); }}>{t("keys.renew")}</Button>
                    </p>
                  )}
                  {k.scopes?.length ? (
                    <ul className="flex flex-wrap gap-1.5" aria-label={t("keys.collections")}>
                      {k.scopes.map((s) => (
                        <li key={s.collection_id} className="max-w-full"><Badge variant="outline" className="h-auto max-w-full justify-start whitespace-normal break-words text-left">{s.collection_name} · {t(s.can_write ? "integrations.read_write" : "integrations.read_only")}</Badge></li>
                      ))}
                    </ul>
                  ) : <p className="text-xs text-muted-foreground">{t("keys.no_collections")}</p>}
                </div>
                <DropdownMenu>
                  <DropdownMenuTrigger render={<Button type="button" variant="ghost" size="icon-sm" className="-mr-1 shrink-0" aria-label={`${t("common.actions")}: ${k.name}`} />}><MoreHorizontal className="size-4" /></DropdownMenuTrigger>
                  <DropdownMenuContent align="end">
                    <DropdownMenuItem onClick={() => setUsage(usage === k.id ? null : k.id)}><Activity className="size-4" />{t(usage === k.id ? "keys.usage_hide" : "keys.usage")}</DropdownMenuItem>
                    <DropdownMenuItem onClick={() => { save.reset(); setDraft(draftOf(k)); }}><Pencil className="size-4" />{t("common.edit")}</DropdownMenuItem>
                    <DropdownMenuSeparator />
                    <DropdownMenuItem variant="destructive" onClick={() => setRevoking(k)}><Trash2 className="size-4" />{t("integrations.revoke")}</DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
                {usage === k.id && <div className="basis-full"><KeyUsage keyId={k.id} /></div>}
              </li>
              );
            })}
          </ul>
        )}

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
          await account.deleteKey(revoking.id).then(() => client.invalidateQueries({ queryKey: KEYS }), (err) => toast.error(errorMessage(err, "common.save_failed", t)));
          setRevoking(null);
        }}
      />
    </FormSection>
  );
}

/** What one key did through MCP: its newest calls of the last 30 days. */
function KeyUsage({ keyId }: { keyId: string }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const list = useQuery({ queryKey: [...KEYS, keyId, "calls"], queryFn: () => account.keyCalls(keyId) });
  if (list.isPending) return <ListSkeleton rows={1} />;
  if (list.isError) return <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />;
  const items = list.data.items;
  if (!items.length) return <p className="text-xs text-muted-foreground">{t("keys.usage_empty")}</p>;
  return (
    <ul className="max-h-64 divide-y overflow-auto rounded border text-xs" aria-label={t("keys.usage")}>
      {items.map((c, i) => (
        <li key={i} className="flex flex-wrap items-center gap-x-2 gap-y-0.5 px-2 py-1.5">
          <span className="font-mono font-medium">{c.tool}</span>
          <span className={c.ok ? "text-muted-foreground" : "text-destructive"}>{t(c.ok ? "mcp_activity.ok" : "mcp_activity.failed")}</span>
          <span className="text-muted-foreground">{c.collection_name} · {format(c.created_at)}</span>
          {c.error && <span className="basis-full break-words text-destructive">{c.error}</span>}
        </li>
      ))}
    </ul>
  );
}

/** Each of the member's collections with the access a key gets to it: none, read, or read and write (editors only). */
export function CollectionAccess({ access, onChange }: { access: Record<string, Access>; onChange: (access: Record<string, Access>) => void }) {
  const { t } = useI18n();
  const cols = useQuery({ queryKey: ["collections", "options"], queryFn: ({ signal }) => collections.options(signal), staleTime: 30_000 });
  const mine = (cols.data?.items ?? []).filter((c) => c.member !== false);
  const [filter, setFilter] = useState("");
  const shown = filter.trim() ? mine.filter((c) => c.name.toLowerCase().includes(filter.trim().toLowerCase())) : mine;
  const setAll = (a: Access) => onChange({ ...access, ...Object.fromEntries(shown.map((c) => [c.id, a])) });
  const accessName: Record<Access, string> = { none: t("keys.access_none"), read: t("integrations.read_only"), write: t("integrations.read_write") };
  return (
    <fieldset className="space-y-1.5">
      <legend className="text-sm font-medium">{t("keys.collections")}</legend>
      <p className="text-xs text-muted-foreground">{t("keys.collections_hint")}</p>
      {cols.isPending ? <ListSkeleton rows={2} /> : cols.isError ? <QueryErrorState error={cols.error} onRetry={() => void cols.refetch()} /> : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            {mine.length > 8 && (
              <Input type="search" aria-label={t("keys.search")} placeholder={t("keys.search")} className="h-8 w-56 max-w-full" value={filter} onChange={(e) => setFilter(e.target.value)} />
            )}
            <Button type="button" size="sm" variant="ghost" onClick={() => setAll("read")}>{t("keys.all_read")}</Button>
            <Button type="button" size="sm" variant="ghost" onClick={() => setAll("none")}>{t("keys.clear")}</Button>
          </div>
          <ul className="max-h-80 divide-y overflow-auto rounded-md border">
            {shown.map((c) => {
              const value = access[c.id] ?? "none";
              const label = `key-access-${c.id}`;
              return (
                <li key={c.id} className="flex items-center gap-3 px-3 py-1.5">
                  <span id={label} className="min-w-0 flex-1 truncate text-sm">{c.name}</span>
                  <Select value={value} items={accessName} onValueChange={(v) => onChange({ ...access, [c.id]: v as Access })}>
                    <SelectTrigger size="sm" className="w-44" aria-labelledby={label}><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="none">{accessName.none}</SelectItem>
                      <SelectItem value="read">{accessName.read}</SelectItem>
                      {/* Writing needs the editor role; a key never goes past its maker. */}
                      <SelectItem value="write" disabled={!roleAllows(c.role, "editor")}>
                        {roleAllows(c.role, "editor") ? accessName.write : `${accessName.write} (${t("keys.needs_editor")})`}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </li>
              );
            })}
          </ul>
        </>
      )}
    </fieldset>
  );
}
