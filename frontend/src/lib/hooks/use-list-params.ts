"use client";

import { useEffect } from "react";
import { useSearchParams } from "next/navigation";

type Values = Record<string, string | number>;

/** Reads list state from a query string; a missing or bad value falls back to its default. */
export function readListParams<T extends Values>(defaults: T, search: URLSearchParams): T {
  const out: Values = {};
  for (const [key, fallback] of Object.entries(defaults)) {
    const raw = search.get(key);
    if (raw === null) out[key] = fallback;
    else if (typeof fallback === "number") out[key] = /^[1-9]\d{0,6}$/.test(raw) ? Number(raw) : fallback;
    else out[key] = raw;
  }
  return out as T;
}

/**
 * List state (page, search, filters, sort) kept in the URL, so back, reload and
 * a shared link keep it. Defaults stay out of the URL.
 */
export function useListParams<T extends Values>(defaults: T) {
  const values = readListParams(defaults, useSearchParams());
  const set = (patch: Partial<T>) => {
    // The live URL, not the render's snapshot: two calls in one event must both land.
    const next = new URLSearchParams(window.location.search);
    for (const [key, value] of Object.entries(patch)) {
      if (value === undefined || value === "" || value === defaults[key]) next.delete(key);
      else next.set(key, String(value));
    }
    const qs = next.toString();
    // null state: Next syncs useSearchParams only for a state it did not write itself.
    window.history.replaceState(null, "", qs ? `?${qs}` : window.location.pathname);
  };
  return [values, set] as const;
}

/** Moves to page when it is set: the URL changes after render, never during it. */
export function useGoToPage(page: number | null, set: (patch: { page: number }) => void) {
  // eslint-disable-next-line react-hooks/exhaustive-deps -- set is rebuilt every render; page alone decides
  useEffect(() => { if (page) set({ page }); }, [page]);
}
