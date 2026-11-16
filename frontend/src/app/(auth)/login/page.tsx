"use client";

import { Suspense, useEffect, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Loader2 } from "lucide-react";
import { AuthCard } from "@/components/auth-card";
import { FormError } from "@/components/bits";
import { CapsLockHint } from "@/components/caps-lock-hint";
import { PasswordInput } from "@/components/password-input";
import { Button, buttonVariants } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError, authPost } from "@/lib/api/client";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { cn, safeNext } from "@/lib/utils";

function LoginForm() {
  const { t } = useI18n();
  const params = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [caps, setCaps] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  // Two-step login: the backend answers MFA_REQUIRED to a right password.
  const [askCode, setAskCode] = useState(false);
  const [code, setCode] = useState("");
  const [sso, setSso] = useState<string | null>(null);
  const ssoError = params.get("sso_error");

  useEffect(() => {
    fetch("/api/auth/sso")
      .then((r) => r.json() as Promise<{ enabled: boolean; name?: string }>)
      .then((r) => setSso(r.enabled ? (r.name ?? "SSO") : null))
      .catch(() => setSso(null));
  }, []);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setPending(true);
    setError(null);
    try {
      await authPost("login", askCode ? { email, password, code } : { email, password });
      // A full load drops any cached data from a previous user.
      window.location.assign(safeNext(params.get("next")));
    } catch (err) {
      setPending(false);
      if (err instanceof ApiError && err.code === "MFA_REQUIRED") {
        setAskCode(true);
        return;
      }
      setError(askCode && err instanceof ApiError && err.code === "INVALID_CREDENTIALS" ? t("auth.mfa_wrong") : errorMessage(err, "auth.login_failed", t));
    }
  };

  return (
    <AuthCard
      title={t("auth.login_title")}
      subtitle={t("auth.login_subtitle")}
      footer={<>{t("auth.no_account")} <Link href="/register" className="font-medium text-primary hover:underline">{t("auth.create_account")}</Link></>}
    >
      <form onSubmit={submit} className="space-y-5">
        <FormError message={error ?? (ssoError && t(ssoError === "no_account" ? "auth.sso_no_account" : "auth.sso_failed"))} />
        {askCode ? (
          <div className="space-y-2">
            <Label htmlFor="code">{t("auth.mfa_code")}</Label>
            <Input id="code" autoComplete="one-time-code" inputMode="text" required autoFocus maxLength={64} value={code} onChange={(e) => setCode(e.target.value)} aria-describedby="code-hint" />
            <p id="code-hint" className="text-xs text-muted-foreground">{t("auth.mfa_hint")}</p>
          </div>
        ) : (
          <>
            <div className="space-y-2">
              <Label htmlFor="email">{t("auth.email")}</Label>
              <Input id="email" type="email" autoComplete="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} />
            </div>
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <Label htmlFor="password">{t("auth.password")}</Label>
                <Link href="/forgot-password" className="text-xs text-primary hover:underline">{t("auth.forgot_link")}</Link>
              </div>
              <PasswordInput id="password" autoComplete="current-password" required value={password} onChange={(e) => setPassword(e.target.value)} onCapsLock={setCaps} />
              <CapsLockHint on={caps} />
            </div>
          </>
        )}
        <Button type="submit" className="w-full" disabled={pending}>
          {pending && <Loader2 className="size-4 animate-spin" />}
          {t("auth.login_action")}
        </Button>
        {sso && !askCode && (
          // A full navigation: the start route is an API route that redirects to the provider.
          // eslint-disable-next-line @next/next/no-html-link-for-pages
          <a href="/api/auth/sso/start" className={cn(buttonVariants({ variant: "outline" }), "w-full")}>{t("auth.sso_login").replace("{name}", sso)}</a>
        )}
      </form>
    </AuthCard>
  );
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
