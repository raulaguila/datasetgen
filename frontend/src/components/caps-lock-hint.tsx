"use client";

import { useI18n } from "@/lib/i18n/provider";

export function CapsLockHint({ on }: { on: boolean }) {
  const { t } = useI18n();
  if (!on) return null;
  return <p className="text-xs text-warn">{t("auth.caps_lock")}</p>;
}
