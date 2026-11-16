"use client";

import { Suspense, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useMutation, useQuery } from "@tanstack/react-query";
import { PlugZap } from "lucide-react";
import { FormError, FormSection, FormSections, PageBody, PageHeader } from "@/components/bits";
import { CollectionAccess, DAYS, scopesOf, type Access } from "@/components/api-keys";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { oauth } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";

export default function OAuthAuthorizePage() {
  const { t } = useI18n();
  return (
    <>
      <PageHeader title={t("oauth.title")} description={t("oauth.subtitle")} />
      <PageBody>
        <Suspense>
          <Consent />
        </Suspense>
      </PageBody>
    </>
  );
}

/** An assistant asks for a key: the member picks its collections and validity, or refuses. */
function Consent() {
  const { t } = useI18n();
  const q = useSearchParams();
  const clientId = q.get("client_id") ?? "";
  const redirectUri = q.get("redirect_uri") ?? "";
  const challenge = q.get("code_challenge") ?? "";
  const method = q.get("code_challenge_method") ?? "";
  const state = q.get("state");
  const valid = q.get("response_type") === "code" && !!clientId && !!redirectUri && !!challenge;
  // The redirect is checked against the client's registration before anything goes back to it.
  const client = useQuery({ queryKey: ["oauth-client", clientId, redirectUri], queryFn: () => oauth.client(clientId, redirectUri), enabled: valid, retry: false });
  const [name, setName] = useState<string | null>(null);
  const [access, setAccess] = useState<Record<string, Access>>({});
  const [days, setDays] = useState("90");
  const back = (params: Record<string, string>) => {
    const url = new URL(redirectUri);
    for (const [k, v] of Object.entries(params)) url.searchParams.set(k, v);
    if (state) url.searchParams.set("state", state);
    window.location.assign(url.toString());
  };
  const keyName = (name ?? client.data?.client_name ?? "").trim();
  const scopes = scopesOf({ access });
  const approve = useMutation({
    mutationFn: () => oauth.approve({ client_id: clientId, redirect_uri: redirectUri, code_challenge: challenge, code_challenge_method: method, name: keyName, scopes, expires_in_days: Number(days) }),
    onSuccess: ({ code }) => back({ code }),
  });
  const daysName: Record<string, string> = Object.fromEntries(DAYS.map((d) => [d, d === "0" ? t("keys.expires_never") : t("keys.expires_days").replace("{n}", d)]));

  if (!valid) return <p role="alert" className="text-sm text-destructive">{t("oauth.bad_request")}</p>;
  if (client.isPending) return <ListSkeleton rows={2} />;
  if (client.isError) return <QueryErrorState error={client.error} onRetry={() => void client.refetch()} />;
  return (
    <FormSections>
      <FormSection title={t("oauth.asks").replace("{client}", client.data.client_name)} description={t("oauth.asks_desc").replace("{host}", new URL(redirectUri).host)}>
        <form className="space-y-4" aria-label={t("oauth.title")} onSubmit={(e) => { e.preventDefault(); if (keyName && scopes.length) approve.mutate(); }}>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="oauth-name">{t("integrations.key_name")}</Label>
            <Input id="oauth-name" maxLength={80} value={name ?? client.data.client_name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label id="oauth-days">{t("keys.expires")}</Label>
            <Select value={days} items={daysName} onValueChange={(v) => setDays(String(v))}>
              <SelectTrigger className="w-56" aria-labelledby="oauth-days"><SelectValue /></SelectTrigger>
              <SelectContent>{DAYS.map((d) => <SelectItem key={d} value={d}>{daysName[d]}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          <CollectionAccess access={access} onChange={setAccess} />
          <p className="text-xs text-muted-foreground">{t("oauth.revoke_hint")}</p>
          <FormError message={approve.isError ? errorMessage(approve.error, "common.save_failed", t) : null} />
          <div className="flex flex-wrap gap-2">
            <Button type="submit" disabled={!keyName || !scopes.length || approve.isPending}><PlugZap className="size-4" />{t("oauth.allow")}</Button>
            <Button type="button" variant="ghost" onClick={() => back({ error: "access_denied" })}>{t("oauth.deny")}</Button>
          </div>
        </form>
      </FormSection>
    </FormSections>
  );
}
