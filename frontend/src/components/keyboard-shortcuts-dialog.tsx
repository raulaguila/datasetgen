"use client";

import { useEffect, useState } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useI18n } from "@/lib/i18n/provider";
import { COLLECTION_SECTIONS } from "@/lib/navigation";

/**
 * "?" opens the list of keyboard shortcuts. The shortcuts live where they are
 * handled (the rail's "g" navigation, the command palette); this only lists them.
 */
export function KeyboardShortcutsDialog() {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey) return;
      const el = document.activeElement as HTMLElement | null;
      if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable)) return;
      if (e.key === "?") setOpen(true);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const groups: { title: string; rows: [string, string][] }[] = [
    { title: t("shortcuts.global"), rows: [["⌘K / Ctrl+K", t("shortcuts.palette")], ["?", t("shortcuts.this")]] },
    { title: t("records.record"), rows: [["j", t("records.next")], ["k", t("records.prev")], ["e", t("common.edit")], ["Ctrl+Enter", t("records.shortcut_save")]] },
    { title: t("shortcuts.goto"), rows: COLLECTION_SECTIONS.filter((s) => s.shortcut).map((s) => [`g ${s.shortcut}`, s.key === "collections.tab.general" ? `${t("collections.tab.settings")} · ${t(s.key)}` : t(s.key)]) },
  ];

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader><DialogTitle>{t("shortcuts.title")}</DialogTitle></DialogHeader>
        <div className="space-y-4">
          {groups.map((g) => (
            <section key={g.title} className="space-y-2">
              <h3 className="text-xs font-medium text-muted-foreground">{g.title}</h3>
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
                {g.rows.map(([keys, label]) => (
                  <div key={keys} className="contents">
                    <dt><kbd className="rounded border bg-muted px-1.5 py-0.5 font-mono text-xs">{keys}</kbd></dt>
                    <dd>{label}</dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
