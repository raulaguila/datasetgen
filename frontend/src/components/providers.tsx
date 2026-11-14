"use client";

import { useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ThemeProvider } from "next-themes";
import { NativeValidationMessages } from "@/components/native-validation";
import { ApiError } from "@/lib/api/client";
import type { Locale } from "@/lib/i18n/messages";
import { I18nProvider } from "@/lib/i18n/provider";
import { AppearanceProvider } from "@/lib/theme/appearance";

// A 4xx answer will not change on retry; one more try covers a network blip.
function retry(failures: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status < 500) return false;
  return failures < 1;
}

export function Providers({ children, initialLocale }: { children: React.ReactNode; initialLocale: Locale }) {
  const [client] = useState(
    () => {
      const client = new QueryClient({ defaultOptions: { queries: { staleTime: 30_000, retry, refetchOnWindowFocus: false } } });
      // Structural sharing keeps the cached object when only the key order changed, and a
      // schema's key order is its field order: a saved reorder would read back as the old one.
      for (const key of [["collections", "detail"], ["schema-versions"]]) client.setQueryDefaults(key, { structuralSharing: false });
      return client;
    },
  );
  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem disableTransitionOnChange>
      <I18nProvider initial={initialLocale}>
        <AppearanceProvider>
          <QueryClientProvider client={client}>
            {children}
            <NativeValidationMessages />
          </QueryClientProvider>
        </AppearanceProvider>
      </I18nProvider>
    </ThemeProvider>
  );
}
