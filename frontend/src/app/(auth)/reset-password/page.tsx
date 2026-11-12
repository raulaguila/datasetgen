"use client";

import { TokenPasswordForm } from "@/components/token-password-form";

export default function ResetPasswordPage() {
  return (
    <TokenPasswordForm
      action="password-reset-confirm"
      title="auth.reset_title"
      subtitle="auth.reset_subtitle"
      submitLabel="auth.reset_action"
      doneTitle="auth.reset_done_title"
      doneBody="auth.reset_done_body"
    />
  );
}
