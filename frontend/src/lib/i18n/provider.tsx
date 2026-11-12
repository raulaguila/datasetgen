"use client";

import { createContext, useCallback, useContext, useState } from "react";
import { messages, type Locale, type MessageKey } from "./messages";

interface I18nContext {
  locale: Locale;
  setLocale: (l: Locale) => void;
  t: (key: MessageKey) => string;
}

const Ctx = createContext<I18nContext | null>(null);

export const LOCALE_COOKIE = "locale";

// The server reads the cookie for the first render, so there is no flash.
export function I18nProvider({ children, initial }: { children: React.ReactNode; initial: Locale }) {
  const [locale, setLocaleState] = useState<Locale>(initial);

  const setLocale = useCallback((l: Locale) => {
    setLocaleState(l);
    document.cookie = `${LOCALE_COOKIE}=${l}; path=/; max-age=${60 * 60 * 24 * 365}; samesite=lax`;
    document.documentElement.lang = l;
  }, []);

  const t = useCallback((key: MessageKey) => messages[locale][key] ?? key, [locale]);

  return <Ctx.Provider value={{ locale, setLocale, t }}>{children}</Ctx.Provider>;
}

export function useI18n(): I18nContext {
  const c = useContext(Ctx);
  if (!c) throw new Error("useI18n must be used within I18nProvider");
  return c;
}
