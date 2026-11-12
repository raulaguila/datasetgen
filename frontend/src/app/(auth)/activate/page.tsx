"use client";

import { TokenPasswordForm } from "@/components/token-password-form";

export default function ActivatePage() {
  return (
    <TokenPasswordForm
      action="activate"
      title="auth.activate_title"
      subtitle="auth.activate_subtitle"
      submitLabel="auth.activate_action"
      doneTitle="auth.activated_title"
      doneBody="auth.activated_body"
    />
  );
}
