"use client";

import { AppShell } from "@/components/app-shell";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { MeProvider } from "@/lib/context/me";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    // Toasts and tooltips live only in the signed-in app: the sign-in pages load neither.
    <MeProvider>
      <TooltipProvider>
        <AppShell>{children}</AppShell>
      </TooltipProvider>
      <Toaster richColors position="bottom-right" />
    </MeProvider>
  );
}
