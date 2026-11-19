"use client";

import { useEffect } from "react";
import { create } from "zustand";

type Parent = { label: string; href: string };

/**
 * Last breadcrumb segment, set by a detail page (a role or user name). Pages
 * inside a collection also give the collection as parent: "Wired > Fields",
 * and a record adds the section it lives in: "Wired > Records > Item".
 */
export const useCrumbStore = create<{ label: string | null; parent: Parent | null; section: Parent | null; set: (label: string | null, parent?: Parent | null, section?: Parent | null) => void }>()((set) => ({
  label: null,
  parent: null,
  section: null,
  set: (label, parent = null, section = null) => set({ label, parent, section }),
}));

export function useCrumb(label: string | undefined, parent?: Parent, section?: Parent) {
  const set = useCrumbStore((s) => s.set);
  const parentLabel = parent?.label;
  const parentHref = parent?.href;
  const sectionLabel = section?.label;
  const sectionHref = section?.href;
  useEffect(() => {
    set(
      label ?? null,
      parentLabel && parentHref ? { label: parentLabel, href: parentHref } : null,
      sectionLabel && sectionHref ? { label: sectionLabel, href: sectionHref } : null,
    );
    return () => set(null);
  }, [label, parentLabel, parentHref, sectionLabel, sectionHref, set]);
}
