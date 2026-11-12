"use client";

import { AuthBrandPanel } from "@/components/auth-brand-panel";
import { BrandWordmark } from "@/components/brand-wordmark";
import { PublicControls } from "@/components/public-controls";
import { useDocumentTitle } from "@/components/bits";

export function AuthCard({
  title,
  subtitle,
  children,
  footer,
}: {
  title: string;
  subtitle?: string;
  children: React.ReactNode;
  footer?: React.ReactNode;
}) {
  useDocumentTitle(title);
  return (
    <div data-ui-auth-card className="relative bg-card dark:bg-background">
      {/* Mobile brand strip */}
      <div
        aria-hidden
        data-ui-auth-brand-strip
      >
        <BrandWordmark height={36} />
      </div>

      <div data-ui-auth-form className="relative flex flex-col justify-center">
        <PublicControls className="mb-4 justify-end text-foreground lg:hidden" />
        <div className="mx-auto w-full max-w-sm space-y-8">
          <div className="hidden w-full justify-center lg:flex">
            <BrandWordmark height={44} />
          </div>

          <div>
            <h1 className="font-heading text-xl font-semibold tracking-tight">{title}</h1>
            {subtitle && (
              <p className="mt-1.5 text-sm text-muted-foreground">{subtitle}</p>
            )}
            {/* Auth field / button sizing — Dolphin-like controls */}
            <div className="mt-6 space-y-5 [&_[data-slot=button]]:h-11 [&_[data-slot=button]]:rounded-md [&_[data-slot=input]]:h-11 [&_[data-slot=input]]:rounded-md [&_[data-slot=input]]:px-3 [&_[data-slot=input]]:text-base [&_[data-slot=input]]:md:text-sm [&_[data-slot=input]]:shadow-none [&_[data-slot=input]]:focus-visible:ring-1 [&_[data-slot=input]]:focus-visible:ring-ring/40">
              {children}
            </div>
          </div>

          {footer && <div className="text-center text-sm text-muted-foreground">{footer}</div>}
        </div>
      </div>

      <AuthBrandPanel />
    </div>
  );
}
