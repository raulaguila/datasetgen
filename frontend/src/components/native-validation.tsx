"use client";

import { useEffect } from "react";
import { useI18n } from "@/lib/i18n/provider";

/**
 * Native form checks speak the browser's language, not the app's. This puts the
 * app's message on every invalid field, so the browser bubble reads in it.
 */
export function NativeValidationMessages() {
  const { t } = useI18n();
  useEffect(() => {
    const onInvalid = (e: Event) => {
      const el = e.target;
      if (!(el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement)) return;
      el.setCustomValidity("");
      const v = el.validity;
      if (v.valid) return;
      const n = (k: number) => String(k);
      el.setCustomValidity(
        v.valueMissing ? t("validation.required")
          : v.typeMismatch && el.type === "email" ? t("validation.email")
          : v.tooShort && "minLength" in el ? t("validation.too_short").replace("{n}", n(el.minLength))
          : v.tooLong && "maxLength" in el ? t("validation.too_long").replace("{n}", n(el.maxLength))
          : t("validation.invalid"),
      );
    };
    // A custom message keeps the field invalid until cleared; any edit clears it.
    const onInput = (e: Event) => {
      const el = e.target;
      if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement) el.setCustomValidity("");
    };
    document.addEventListener("invalid", onInvalid, true);
    document.addEventListener("input", onInput, true);
    return () => {
      document.removeEventListener("invalid", onInvalid, true);
      document.removeEventListener("input", onInput, true);
    };
  }, [t]);
  return null;
}
