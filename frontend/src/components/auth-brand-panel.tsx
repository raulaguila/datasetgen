"use client";

import { BrandMark } from "@/components/brand-mark";
import { PublicControls } from "@/components/public-controls";
import { useI18n } from "@/lib/i18n/provider";

/** Desktop brand column follows the height of the authentication form. */
export function AuthBrandPanel() {
  const { t } = useI18n();
  return (
    <div className="relative hidden flex-col overflow-hidden bg-primary text-(--brand-foreground) lg:flex [background-image:var(--brand-panel)]">
      <div className="relative z-20 flex justify-end p-6">
        <PublicControls className="text-(--brand-foreground) [&_button]:text-(--brand-foreground) [&_button:hover]:bg-white/10" />
      </div>
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-0 top-0 bottom-1/2 flex items-center justify-center overflow-hidden"
      >
        {/* Upper half only: behind the headline it fought the text. */}
        <BrandMark size={200} className="opacity-[0.14]" />
      </div>
      <div className="relative z-10 mt-auto max-w-md space-y-3 p-10 pb-12 pr-12">
        <p className="text-sm font-medium uppercase tracking-[0.2em] text-(--brand-foreground)">
          {t("auth.brand_eyebrow")}
        </p>
        <h2 className="font-heading text-3xl font-semibold leading-tight text-(--brand-foreground)">
          {t("auth.brand_tagline")}
        </h2>
        <p className="text-sm leading-relaxed text-(--brand-foreground)">
          {t("auth.brand_body")}
        </p>
      </div>
    </div>
  );
}
