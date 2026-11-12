"use client";

import { useTheme } from "next-themes";
import { useI18n } from "@/lib/i18n/provider";
import { Moon, Sun } from "lucide-react";
import { useEffect, useState } from "react";
import { useOptionalMe } from "@/lib/context/me";

export function ThemeToggle() {
  const { t } = useI18n();
  const { resolvedTheme, setTheme } = useTheme();
  const me = useOptionalMe();
  const [mounted, setMounted] = useState(false);
  // eslint-disable-next-line react-hooks/set-state-in-effect -- mount flag to avoid SSR hydration mismatch
  useEffect(() => setMounted(true), []);

  const isDark = mounted && resolvedTheme === "dark";
  return (
    <button
      aria-label={`${t("common.theme")}: ${t(isDark ? "profile.theme_light" : "profile.theme_dark")}`}
      type="button"
      title={t(isDark ? "profile.theme_light" : "profile.theme_dark")}
      onClick={() => {
        const next = isDark ? "light" : "dark";
        setTheme(next);
        void me?.savePreferences({ theme: next });
      }}
      className="flex size-11 md:size-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
    >
      {/* Avoid hydration mismatch: render a neutral icon until mounted. */}
      {isDark ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </button>
  );
}
