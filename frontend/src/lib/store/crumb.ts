"use client";

import { useEffect } from "react";
import { create } from "zustand";

/** Last breadcrumb segment, set by a detail page (a role or user name). */
export const useCrumbStore = create<{ label: string | null; set: (label: string | null) => void }>()((set) => ({
  label: null,
  set: (label) => set({ label }),
}));

export function useCrumb(label: string | undefined) {
  const set = useCrumbStore((s) => s.set);
  useEffect(() => {
    set(label ?? null);
    return () => set(null);
  }, [label, set]);
}
