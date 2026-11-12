"use client";

import { createElement, useId, useMemo, useRef, useState } from "react";
import { Ban, ChevronDown } from "lucide-react";
import { Popover } from "@base-ui/react/popover";
import { COLLECTION_ICON_GROUPS, COLLECTION_ICONS } from "@/lib/collection-icons";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";

const SLUGS = Object.keys(COLLECTION_ICONS);
const COLUMNS = 6;

/**
 * A field-sized trigger that opens the curated icons in a popover, with a
 * filter and categories; "" is no icon (the initials). Ported from thoth.
 */
export function IconPicker({ value, onChange, label, noneLabel, disabled }: {
  value: string;
  onChange: (slug: string) => void;
  label: string;
  noneLabel: string;
  disabled?: boolean;
}) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const [category, setCategory] = useState("all");
  const [focusedSlug, setFocusedSlug] = useState<string | null>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const id = useId();
  const selected = COLLECTION_ICONS[value];

  const slugs = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const base = category === "all" ? SLUGS : COLLECTION_ICON_GROUPS.find((g) => g.id === category)?.slugs ?? SLUGS;
    return q ? base.filter((s) => s.includes(q)) : base;
  }, [filter, category]);

  const close = () => {
    setOpen(false);
    setFilter("");
    triggerRef.current?.focus();
  };
  const pick = (slug: string) => {
    onChange(slug);
    close();
  };
  const focusOption = (index: number) => {
    const options = listRef.current?.querySelectorAll<HTMLButtonElement>("[role=option]");
    if (!options?.length) return;
    const next = Math.max(0, Math.min(options.length - 1, index));
    setFocusedSlug(slugs[next]);
    options[next].focus();
    options[next].scrollIntoView?.({ block: "nearest" });
  };
  // One option in the tab order: the focused one, else the selected one, else the first.
  const tabSlug = slugs.includes(focusedSlug ?? "") ? focusedSlug : slugs.includes(value) ? value : slugs[0];
  const chip = (key: string, text: string) => (
    <button
      key={key}
      type="button"
      aria-pressed={category === key}
      onClick={() => setCategory(key)}
      className={cn("rounded-md px-2 py-0.5 text-[11px] transition-colors",
        category === key ? "bg-primary/15 text-primary" : "text-muted-foreground hover:bg-muted hover:text-foreground")}
    >
      {text}
    </button>
  );

  return (
    <Popover.Root open={open} onOpenChange={(next) => { setOpen(next); if (!next) setFilter(""); }}>
      <div className="flex flex-col gap-1.5">
        <label htmlFor={`${id}-trigger`} className="text-sm font-medium">{label}</label>
        <Popover.Trigger
          ref={triggerRef}
          id={`${id}-trigger`}
          type="button"
          disabled={disabled}
          aria-label={`${label}: ${value || noneLabel}`}
          className="flex h-9 items-center gap-2 rounded-lg border border-input bg-card px-2.5 text-sm hover:bg-muted/40 disabled:pointer-events-none disabled:opacity-60 dark:bg-transparent"
        >
          {selected ? createElement(selected, { className: "size-4 text-primary" }) : <Ban className="size-4 text-muted-foreground/60" />}
          <span className="flex-1 truncate text-left font-mono text-xs text-muted-foreground">{value || noneLabel}</span>
          <ChevronDown className={cn("size-3.5 text-muted-foreground transition-transform", open && "rotate-180")} />
        </Popover.Trigger>
      </div>
      <Popover.Portal>
        <Popover.Positioner sideOffset={4} align="start" collisionPadding={16} positionMethod="fixed" className="z-50">
          <Popover.Popup
            aria-label={label}
            className="max-h-[var(--available-height)] w-[var(--anchor-width)] min-w-72 max-w-[calc(100vw-32px)] overflow-y-auto overscroll-contain rounded-lg bg-popover p-2 text-popover-foreground shadow-md ring-1 ring-foreground/10"
          >
            <input
              autoFocus
              aria-label={t("icon.filter")}
              placeholder={t("icon.filter")}
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "ArrowDown") { e.preventDefault(); focusOption(Math.max(0, slugs.indexOf(tabSlug ?? ""))); }
              }}
              className="mb-2 h-8 w-full rounded-md border border-input bg-card px-2 text-xs outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring dark:bg-transparent"
            />
            <div className="mb-2 flex flex-wrap gap-1">
              {chip("all", t("icon.group.all"))}
              {COLLECTION_ICON_GROUPS.map((g) => chip(g.id, t(`icon.group.${g.id}`)))}
            </div>
            <div className="max-h-56 overflow-y-auto">
              <div className="sticky top-0 z-[1] mb-1 bg-popover pb-1">
                <button
                  type="button"
                  onClick={() => pick("")}
                  className={cn("flex h-9 w-full items-center justify-center gap-2 rounded-md border text-xs transition-colors",
                    !value ? "border-primary bg-primary/10 text-primary" : "border-border text-muted-foreground hover:bg-muted hover:text-foreground")}
                >
                  <Ban className="size-4" />{noneLabel}
                </button>
              </div>
              <div ref={listRef} role="listbox" aria-label={label} className="grid grid-cols-6 gap-1">
                {slugs.map((slug, index) => (
                  <button
                    key={slug}
                    type="button"
                    role="option"
                    title={slug}
                    aria-label={slug}
                    aria-selected={value === slug}
                    tabIndex={slug === tabSlug ? 0 : -1}
                    onFocus={() => setFocusedSlug(slug)}
                    onKeyDown={(e) => {
                      const next = ({ ArrowRight: index + 1, ArrowLeft: index - 1, ArrowDown: index + COLUMNS, ArrowUp: index - COLUMNS, Home: 0, End: slugs.length - 1 } as Record<string, number>)[e.key];
                      if (next !== undefined) { e.preventDefault(); focusOption(next); }
                    }}
                    onClick={() => pick(slug)}
                    className={cn("flex h-9 w-full items-center justify-center rounded-md border transition-colors",
                      value === slug ? "border-primary bg-primary/10 text-primary" : "border-transparent text-foreground/80 hover:bg-muted hover:text-foreground")}
                  >
                    {createElement(COLLECTION_ICONS[slug], { className: "size-4" })}
                  </button>
                ))}
              </div>
              {slugs.length === 0 && <p className="py-2 text-center text-[11px] text-muted-foreground">{t("icon.empty")}</p>}
            </div>
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}
