"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Loader2 } from "lucide-react";
import { AuthCard } from "@/components/auth-card";
import { FormError } from "@/components/bits";
import { CapsLockHint } from "@/components/caps-lock-hint";
import { PasswordInput } from "@/components/password-input";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authPost } from "@/lib/api/client";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { safeNext } from "@/lib/utils";

function LoginForm() {
  const { t } = useI18n();
  const params = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [caps, setCaps] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setPending(true);
    setError(null);
    try {
      await authPost("login", { email, password });
      // A full load drops any cached data from a previous user.
      window.location.assign(safeNext(params.get("next")));
    } catch (err) {
      setError(errorMessage(err, "auth.login_failed", t));
      setPending(false);
    }
  };

  return (
    <AuthCard
      title={t("auth.login_title")}
      subtitle={t("auth.login_subtitle")}
      footer={<>{t("auth.no_account")} <Link href="/register" className="font-medium text-primary hover:underline">{t("auth.create_account")}</Link></>}
    >
      <form onSubmit={submit} className="space-y-5">
        <FormError message={error} />
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
        <Button type="submit" className="w-full" disabled={pending}>
          {pending && <Loader2 className="size-4 animate-spin" />}
          {t("auth.login_action")}
        </Button>
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
