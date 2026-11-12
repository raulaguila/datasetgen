"use client";

import { useEffect } from "react";
import { create } from "zustand";

type Parent = { label: string; href: string };

/**
 * Last breadcrumb segment, set by a detail page (a role or user name). Pages
 * inside a collection also give the collection as parent: "Wired > Fields".
 */
export const useCrumbStore = create<{ label: string | null; parent: Parent | null; set: (label: string | null, parent?: Parent | null) => void }>()((set) => ({
  label: null,
  parent: null,
  set: (label, parent = null) => set({ label, parent }),
}));

export function useCrumb(label: string | undefined, parent?: Parent) {
  const set = useCrumbStore((s) => s.set);
  const parentLabel = parent?.label;
  const parentHref = parent?.href;
  useEffect(() => {
    set(label ?? null, parentLabel && parentHref ? { label: parentLabel, href: parentHref } : null);
    return () => set(null);
  }, [label, parentLabel, parentHref, set]);
}
