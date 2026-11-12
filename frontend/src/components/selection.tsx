"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { TableCell, TableHead } from "@/components/ui/table";
import { useI18n } from "@/lib/i18n/provider";

/** Rows ticked on the current page; a new page or a refetch with other rows starts empty. */
export function useSelection(pageIds: string[]) {
  const key = pageIds.join(",");
  const [state, setState] = useState({ key, ids: new Set<string>() });
  const ids = state.key === key ? state.ids : new Set<string>();
  const set = (next: Set<string>) => setState({ key, ids: next });
  return {
    ids: [...ids],
    has: (id: string) => ids.has(id),
    toggle: (id: string, on: boolean) => {
      const next = new Set(ids);
      if (on) next.add(id);
      else next.delete(id);
      set(next);
    },
    togglePage: (on: boolean) => set(on ? new Set(pageIds) : new Set()),
    clear: () => set(new Set()),
    all: pageIds.length > 0 && pageIds.every((id) => ids.has(id)),
  };
}

type Selection = ReturnType<typeof useSelection>;

export function SelectAllHead({ selection }: { selection: Selection }) {
  const { t } = useI18n();
  return (
    <TableHead className="w-10 [&:has([role=checkbox])]:pr-0">
      <Checkbox
        aria-label={t("common.select_all")}
        checked={selection.all}
        indeterminate={!selection.all && selection.ids.length > 0}
        onCheckedChange={() => selection.togglePage(!selection.all)}
      />
    </TableHead>
  );
}

/** Its clicks stay here, so a clickable row does not open. Pass only selectable ids to useSelection. */
export function SelectCell({ selection, id, label, disabled }: { selection: Selection; id: string; label: string; disabled?: boolean }) {
  return (
    <TableCell className="w-10 [&:has([role=checkbox])]:pr-0" onClick={(e) => e.stopPropagation()}>
      <Checkbox aria-label={label} disabled={disabled} checked={selection.has(id)} onCheckedChange={(on) => selection.toggle(id, on === true)} />
    </TableCell>
  );
}

/** Sticky bar with the count and the bulk actions while something is ticked. */
export function SelectionBar({ selection, children }: { selection: Selection; children: React.ReactNode }) {
  const { t } = useI18n();
  const n = selection.ids.length;
  if (n === 0) return null;
  return (
    <div role="region" aria-label={t("common.selection")} className="sticky bottom-4 z-10 flex flex-wrap items-center gap-2 rounded-xl border bg-card px-4 py-3 shadow-lg">
      <span className="flex-1 text-sm font-medium">{n} {t(n === 1 ? "common.selected_one" : "common.selected_many")}</span>
      <Button variant="ghost" onClick={selection.clear}>{t("common.clear_selection")}</Button>
      {children}
    </div>
  );
}
