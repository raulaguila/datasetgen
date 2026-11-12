"use client";

// The signed in user, loaded from /v1/me and refreshed on window focus so
// a permission change shows up without a new login. Their saved theme and
// language are applied once per load.
import { createContext, useContext, useEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTheme } from "next-themes";
import { account } from "@/lib/api/endpoints";
import type { Me, Preferences } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";

interface MeCtx {
  user: Me | null;
  /** /me failed and nothing is cached: permissions are unknown, not empty. */
  failed: boolean;
  retry: () => void;
  sessionId: string | null;
  loading: boolean;
  /** Resolves true once stored; on failure reloads the server copy and resolves false. */
  savePreferences: (p: Partial<Preferences>) => Promise<boolean>;
}

const Ctx = createContext<MeCtx | null>(null);

export const ME_KEY = ["me"] as const;

export function MeProvider({ children }: { children: React.ReactNode }) {
  const client = useQueryClient();
  const { setTheme } = useTheme();
  const { setLocale } = useI18n();
  const query = useQuery({ queryKey: ME_KEY, queryFn: account.me, refetchOnWindowFocus: true, staleTime: 60_000 });
  const user = query.data?.user ?? null;

  const applied = useRef<string | null>(null);
  useEffect(() => {
    if (!user || applied.current === user.id) return;
    applied.current = user.id;
    setTheme(user.preferences.theme);
    setLocale(user.preferences.language);
  }, [user, setTheme, setLocale]);

  const savePreferences = async (p: Partial<Preferences>) => {
    if (!user) return false;
    const next = { ...user.preferences, ...p };
    client.setQueryData(ME_KEY, { ...query.data!, user: { ...user, preferences: next } });
    try {
      await account.savePreferences(next);
      return true;
    } catch {
      void client.invalidateQueries({ queryKey: ME_KEY });
      return false;
    }
  };

  return (
    <Ctx.Provider value={{ user, failed: query.isError && !user, retry: () => void query.refetch(), sessionId: query.data?.session_id ?? null, loading: query.isPending, savePreferences }}>
      {children}
    </Ctx.Provider>
  );
}

export function useMe(): MeCtx {
  const c = useContext(Ctx);
  if (!c) throw new Error("useMe must be used within MeProvider");
  return c;
}

/** Same as useMe outside the app area (auth pages), where nobody is signed in. */
export function useOptionalMe(): MeCtx | null {
  return useContext(Ctx);
}
