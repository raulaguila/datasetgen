"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

/**
 * The records the list showed last, in its order: the record page steps through
 * them (previous / next) without asking the server again. Kept for the tab, so a
 * reload of the record page still knows its neighbours.
 */
export const useSequenceStore = create<{ ids: string[]; set: (ids: string[]) => void }>()(
  persist((set) => ({ ids: [], set: (ids) => set({ ids }) }), { name: "record-sequence", storage: createJSONStorage(() => sessionStorage), partialize: (s) => ({ ids: s.ids }) }),
);

/** The neighbours of `id` in the last list, when it was in it. */
export function neighbours(ids: string[], id: string): { prev?: string; next?: string; index: number } {
  const index = ids.indexOf(id);
  if (index < 0) return { index };
  return { prev: ids[index - 1], next: ids[index + 1], index };
}
