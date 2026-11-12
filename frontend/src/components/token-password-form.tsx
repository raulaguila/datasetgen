"use client";

import { Suspense, useState, useEffect } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Loader2 } from "lucide-react";
import { AuthCard } from "@/components/auth-card";
import { FieldNote, FormError } from "@/components/bits";
import { CapsLockHint } from "@/components/caps-lock-hint";
import { PasswordInput } from "@/components/password-input";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authPost } from "@/lib/api/client";
import { errorMessage } from "@/lib/errors";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { newPassword, ready, sameAs } from "@/lib/validation";

// The mail carries a plain token, so the field accepts a paste; a #token= link (or an old ?token= one) fills it.
function Form({ action, title, subtitle, submitLabel, doneTitle, doneBody }: {
  action: "activate" | "password-reset-confirm";
  title: MessageKey;
  subtitle: MessageKey;
  submitLabel: MessageKey;
  doneTitle: MessageKey;
  doneBody: MessageKey;
}) {
  const { t } = useI18n();
  const params = useSearchParams();
  const [token, setToken] = useState(params.get("token") ?? "");
  // Email links carry the token in the fragment, which no server sees; take it and clean the address bar.
  useEffect(() => {
    const fromLink = new URLSearchParams(window.location.hash.slice(1)).get("token");
    if (!fromLink) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect -- the fragment exists only in the browser, after hydration
    setToken(fromLink);
    window.history.replaceState(null, "", window.location.pathname);
  }, []);
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [caps, setCaps] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [done, setDone] = useState(false);
  const confirmErr = sameAs(password)(confirm);

  const footer = <Link href="/login" className="inline-block py-1 font-medium text-primary hover:underline">{t("auth.back_to_login")}</Link>;

  if (done) {
    return (
      <AuthCard title={t(doneTitle)} subtitle={t(doneBody)}>
        <Button className="w-full" nativeButton={false} render={<Link href="/login" />}>{t("auth.login_action")}</Button>
      </AuthCard>
    );
  }

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setPending(true);
    setError(null);
    try {
      await authPost(action, { token: token.trim(), password });
      setDone(true);
    } catch (err) {
      setError(errorMessage(err, "auth.token_failed", t));
    } finally {
      setPending(false);
    }
  };

  return (
    <AuthCard title={t(title)} subtitle={t(subtitle)} footer={footer}>
      <form onSubmit={submit} className="space-y-5">
        <FormError message={error} />
        <div className="space-y-2">
          <Label htmlFor="token">{t("auth.token")}</Label>
          <Input id="token" required autoComplete="one-time-code" spellCheck={false} value={token} onChange={(e) => setToken(e.target.value)} className="font-mono" />
        </div>
        <div className="space-y-2">
          <Label htmlFor="password">{t("auth.new_password")}</Label>
          <PasswordInput id="password" autoComplete="new-password" required maxLength={128} aria-invalid={!!newPassword(password)} aria-describedby="password-note" value={password} onChange={(e) => setPassword(e.target.value)} onCapsLock={setCaps} />
          <FieldNote id="password-note" hint={t("auth.password_rule")} error={newPassword(password) && t("auth.password_rule")} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="confirm">{t("auth.confirm_password")}</Label>
          <PasswordInput id="confirm" autoComplete="new-password" required aria-invalid={!!confirmErr} aria-describedby="confirm-note" value={confirm} onChange={(e) => setConfirm(e.target.value)} onCapsLock={setCaps} />
          <FieldNote id="confirm-note" error={confirmErr && t(confirmErr)} />
          <CapsLockHint on={caps} />
        </div>
        <Button type="submit" className="w-full" disabled={pending || !ready([token], [password, newPassword], [confirm, sameAs(password)])}>
          {pending && <Loader2 className="size-4 animate-spin" />}
          {t(submitLabel)}
        </Button>
      </form>
    </AuthCard>
  );
}

export function TokenPasswordForm(props: React.ComponentProps<typeof Form>) {
  return (
    <Suspense>
      <Form {...props} />
    </Suspense>
  );
}
