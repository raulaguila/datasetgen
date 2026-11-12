"use client";

import { create } from "zustand";
import { persist } from "zustand/middleware";

interface UIState {
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
  setSidebar: (collapsed: boolean) => void;
  /** The collection the sidebar shows, the last one opened. */
  collectionId: string | null;
  setCollection: (id: string | null) => void;
  /** Collections pinned to the top of the sidebar, on this browser only. */
  pinned: string[];
  togglePin: (id: string) => void;
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setSidebar: (collapsed) => set({ sidebarCollapsed: collapsed }),
      collectionId: null,
      setCollection: (id) => set({ collectionId: id }),
      pinned: [],
      togglePin: (id) => set((s) => ({ pinned: s.pinned.includes(id) ? s.pinned.filter((p) => p !== id) : [...s.pinned, id] })),
    }),
    { name: "ui" },
  ),
);
