"use client";

import { useState } from "react";
import Link from "next/link";
import { Loader2 } from "lucide-react";
import { AuthCard } from "@/components/auth-card";
import { FieldNote, FormError } from "@/components/bits";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { authPost } from "@/lib/api/client";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { email as emailRule, ready } from "@/lib/validation";

export default function ForgotPasswordPage() {
  const { t } = useI18n();
  const [email, setEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [sent, setSent] = useState(false);
  const emailErr = emailRule(email);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setPending(true);
    setError(null);
    try {
      // The server answers the same for known and unknown addresses.
      await authPost("password-reset-request", { email });
      setSent(true);
    } catch (err) {
      setError(errorMessage(err, "auth.forgot_failed", t));
    } finally {
      setPending(false);
    }
  };

  const footer = <Link href="/login" className="inline-block py-1 font-medium text-primary hover:underline">{t("auth.back_to_login")}</Link>;

  if (sent) {
    return (
      <AuthCard title={t("auth.check_email_title")} subtitle={t("auth.check_email_reset")} footer={footer}>
        <Button className="w-full" nativeButton={false} render={<Link href="/reset-password" />}>{t("auth.have_token")}</Button>
      </AuthCard>
    );
  }

  return (
    <AuthCard title={t("auth.forgot_title")} subtitle={t("auth.forgot_subtitle")} footer={footer}>
      <form onSubmit={submit} className="space-y-5">
        <FormError message={error} />
        <div className="space-y-2">
          <Label htmlFor="email">{t("auth.email")}</Label>
          <Input id="email" type="email" autoComplete="email" required autoFocus aria-invalid={!!emailErr} aria-describedby="email-note" value={email} onChange={(e) => setEmail(e.target.value)} />
          <FieldNote id="email-note" error={emailErr && t(emailErr)} />
        </div>
        <Button type="submit" className="w-full" disabled={pending || !ready([email, emailRule])}>
          {pending && <Loader2 className="size-4 animate-spin" />}
          {t("auth.forgot_action")}
        </Button>
      </form>
    </AuthCard>
  );
}
