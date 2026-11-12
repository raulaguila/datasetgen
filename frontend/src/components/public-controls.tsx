"use client";

import { ThemeToggle } from "@/components/theme-toggle";
import { LanguageSwitcher } from "@/components/language-switcher";
import { cn } from "@/lib/utils";

// Theme / language controls for screens without the app topbar
// (auth pages, the workspace picker). Both providers wrap the whole app,
// so the controls work pre-login too; the choices persist via cookie/localStorage.
export function PublicControls({ className }: { className?: string }) {
  return (
    <div className={cn("flex items-center gap-1", className)}>
      <ThemeToggle />
      <LanguageSwitcher />
    </div>
  );
}
