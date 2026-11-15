"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { KeyRound, Pencil, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { FormError, FormSection, IconAction, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { QueryErrorState } from "@/components/query-error-state";
import { Secret } from "@/components/secret";
import { ListSkeleton } from "@/components/skeletons";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { account, collections } from "@/lib/api/endpoints";
import type { ApiKey, KeyScope } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";

type Access = "none" | "read" | "write";
/** The key being made (no id) or changed: its name and each collection's access. */
type Draft = { id?: string; name: string; access: Record<string, Access> };

const KEYS = ["me", "keys"] as const;

const draftOf = (k?: ApiKey): Draft => ({
  id: k?.id,
  name: k?.name ?? "",
  access: Object.fromEntries((k?.scopes ?? []).map((s) => [s.collection_id, s.can_write ? "write" : "read"])),
});

const scopesOf = (d: Draft): KeyScope[] =>
  Object.entries(d.access).filter(([, a]) => a !== "none").map(([collection_id, a]) => ({ collection_id, can_write: a === "write" }));

/** The member's own API keys: each reaches the collections picked, read only or read and write. */
export function ApiKeysCard() {
  const { t } = useI18n();
  const client = useQueryClient();
  const format = useFormatDate();
  const list = useQuery({ queryKey: KEYS, queryFn: account.keys });
  const cols = useQuery({ queryKey: ["collections", "options"], queryFn: ({ signal }) => collections.options(signal), staleTime: 30_000 });
  const mine = (cols.data?.items ?? []).filter((c) => c.member !== false);
  const [draft, setDraft] = useState<Draft | null>(null);
  const [secret, setSecret] = useState<string | null>(null);
  const [revoking, setRevoking] = useState<ApiKey | null>(null);
  const save = useMutation({
    mutationFn: (d: Draft) => (d.id ? account.updateKey(d.id, d.name.trim(), scopesOf(d)) : account.createKey(d.name.trim(), scopesOf(d))),
    onSuccess: (k: ApiKey & { secret?: string }) => {
      if (k.secret) setSecret(k.secret);
      else toast.success(t("common.saved"));
      setDraft(null);
      void client.invalidateQueries({ queryKey: KEYS });
    },
  });
  const items = list.data?.items ?? [];
  const accessName: Record<Access, string> = { none: t("keys.access_none"), read: t("integrations.read_only"), write: t("integrations.read_write") };
  const picked = draft ? scopesOf(draft).length : 0;

  return (
    <FormSection id="keys" title={t("keys.title")} description={t("keys.desc")}>
      <div className="space-y-3">
        {secret && <Secret value={secret} onClose={() => setSecret(null)} />}
        {list.isPending ? <ListSkeleton rows={1} /> : list.isError ? <QueryErrorState error={list.error} onRetry={() => void list.refetch()} /> : items.length === 0 ? (
          <p className="rounded-md border border-dashed px-3 py-4 text-center text-sm text-muted-foreground">{t("integrations.keys_empty")}</p>
        ) : (
          <ul className="divide-y rounded-md border">
            {items.map((k) => (
              <li key={k.id} className="flex items-start gap-3 px-3 py-2">
                <div className="min-w-0 flex-1 space-y-1.5">
                  <p className="text-sm font-medium">{k.name}</p>
                  <p className="text-xs text-muted-foreground">
                    <span className="font-mono">{k.prefix}…</span>
                    {` · ${k.last_used_at ? t("integrations.used_at").replace("{at}", format(k.last_used_at)) : t("integrations.never_used")}`}
                  </p>
                  {k.scopes?.length ? (
                    <ul className="flex flex-wrap gap-1.5" aria-label={t("keys.collections")}>
                      {k.scopes.map((s) => (
                        <li key={s.collection_id} className="max-w-full"><Badge variant="outline" className="h-auto max-w-full justify-start whitespace-normal break-words text-left">{s.collection_name} · {t(s.can_write ? "integrations.read_write" : "integrations.read_only")}</Badge></li>
                      ))}
                    </ul>
                  ) : <p className="text-xs text-muted-foreground">{t("keys.no_collections")}</p>}
                </div>
                <div className="flex shrink-0">
                  <IconAction label={t("common.edit")} target={k.name} onClick={() => { save.reset(); setDraft(draftOf(k)); }}><Pencil className="size-4" /></IconAction>
                  <IconAction label={t("integrations.revoke")} target={k.name} className="text-destructive" onClick={() => setRevoking(k)}><Trash2 className="size-4" /></IconAction>
                </div>
              </li>
            ))}
          </ul>
        )}

        {draft ? (
          <form className="space-y-3 rounded-md border p-3" onSubmit={(e) => { e.preventDefault(); if (draft.name.trim() && picked > 0) save.mutate(draft); }}>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="key-name">{t("integrations.key_name")}</Label>
              <Input id="key-name" maxLength={80} value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} autoFocus />
            </div>
            <fieldset className="space-y-1.5">
              <legend className="text-sm font-medium">{t("keys.collections")}</legend>
              <p className="text-xs text-muted-foreground">{t("keys.collections_hint")}</p>
              {cols.isPending ? <ListSkeleton rows={2} /> : cols.isError ? <QueryErrorState error={cols.error} onRetry={() => void cols.refetch()} /> : (
                <ul className="max-h-80 divide-y overflow-auto rounded-md border">
                  {mine.map((c) => {
                    const value = draft.access[c.id] ?? "none";
                    const label = `key-access-${c.id}`;
                    return (
                      <li key={c.id} className="flex items-center gap-3 px-3 py-1.5">
                        <span id={label} className="min-w-0 flex-1 truncate text-sm">{c.name}</span>
                        <Select value={value} items={accessName} onValueChange={(v) => setDraft({ ...draft, access: { ...draft.access, [c.id]: v as Access } })}>
                          <SelectTrigger size="sm" className="w-44" aria-labelledby={label}><SelectValue /></SelectTrigger>
                          <SelectContent>
                            <SelectItem value="none">{accessName.none}</SelectItem>
                            <SelectItem value="read">{accessName.read}</SelectItem>
                            {/* Writing needs the editor role; a key never goes past its maker. */}
                            <SelectItem value="write" disabled={!roleAllows(c.role, "editor")}>{accessName.write}</SelectItem>
                          </SelectContent>
                        </Select>
                      </li>
                    );
                  })}
                </ul>
              )}
            </fieldset>
            <FormError message={save.isError ? errorMessage(save.error, "common.save_failed", t) : null} />
            <div className="flex flex-wrap items-center gap-2">
              <Button type="submit" disabled={!draft.name.trim() || picked === 0 || save.isPending}>{draft.id ? t("common.save") : <><KeyRound className="size-4" />{t("integrations.create_key")}</>}</Button>
              <Button type="button" variant="ghost" onClick={() => setDraft(null)}>{t("common.cancel")}</Button>
              <span className="text-xs text-muted-foreground">{t("keys.picked").replace("{n}", String(picked))}</span>
            </div>
          </form>
        ) : (
          <Button variant="outline" onClick={() => { save.reset(); setDraft(draftOf()); }}><KeyRound className="size-4" />{t("keys.new")}</Button>
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
