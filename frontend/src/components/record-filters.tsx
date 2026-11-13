"use client";

import { useState } from "react";
import { Popover } from "@base-ui/react/popover";
import { ListFilter, X } from "lucide-react";
import { type FieldRow } from "@/components/field-builder";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useI18n } from "@/lib/i18n/provider";

export type FieldFilters = Record<string, string | number | boolean>;

/** The "any value" choice; real values are never this. */
const ANY = "\u0000any";

const FILTERABLE = new Set(["string", "text", "number", "integer", "boolean", "enum", "date", "email", "url"]);

/** The fields a list can filter on: plain top-level values, matched exactly. */
export function filterableFields(rows: FieldRow[]) {
  return rows.filter((r) => !r.advanced && FILTERABLE.has(r.field.type));
}

/** A "Filters" button with one control per field, and the active filters as removable chips. */
export function RecordFilters({ fields, value, onChange }: { fields: FieldRow[]; value: FieldFilters; onChange: (v: FieldFilters) => void }) {
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const active = Object.keys(value).length;
  const label = (key: string) => fields.find((f) => f.key === key)?.field.name ?? key;
  const show = (v: string | number | boolean) => (typeof v === "boolean" ? t(v ? "common.yes" : "common.no") : String(v));

  const openWith = (o: boolean) => {
    if (o) setDraft(Object.fromEntries(Object.entries(value).map(([k, v]) => [k, String(v)])));
    setOpen(o);
  };
  const apply = () => {
    const next: FieldFilters = {};
    for (const r of fields) {
      const raw = draft[r.key]?.trim();
      if (!raw) continue;
      if (r.field.type === "boolean") next[r.key] = raw === "true";
      else if (r.field.type === "number" || r.field.type === "integer") {
        if (Number.isFinite(Number(raw))) next[r.key] = Number(raw);
      } else next[r.key] = raw;
    }
    onChange(next);
    setOpen(false);
  };
  const remove = (key: string) => {
    const next = { ...value };
    delete next[key];
    onChange(next);
  };
  const control = (r: FieldRow) => {
    const id = `flt-${r.key}`;
    const v = draft[r.key] ?? "";
    const set = (x: string) => setDraft((d) => ({ ...d, [r.key]: x }));
    if (r.field.type === "boolean" || r.field.type === "enum") {
      const options = r.field.type === "boolean"
        ? [["true", t("common.yes")], ["false", t("common.no")]]
        : r.field.options.split(",").map((o) => o.trim()).filter(Boolean).map((o) => [o, o]);
      const items: Record<string, string> = { [ANY]: t("filters.any"), ...Object.fromEntries(options) };
      return (
        <Select value={v || ANY} items={items} onValueChange={(x) => set(x === ANY ? "" : String(x))}>
          <SelectTrigger id={id} className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent>
            {Object.entries(items).map(([val, text]) => <SelectItem key={val} value={val}>{text}</SelectItem>)}
          </SelectContent>
        </Select>
      );
    }
    const numeric = r.field.type === "number" || r.field.type === "integer";
    return <Input id={id} type={numeric ? "number" : "text"} value={v} placeholder={t("filters.exact")} onChange={(e) => set(e.target.value)} />;
  };

  if (fields.length === 0) return null;
  return (
    <>
      <Popover.Root open={open} onOpenChange={openWith}>
        <Popover.Trigger render={<Button variant="outline" />}>
          <ListFilter className="size-4" />{t("filters.title")}{active > 0 && <span className="rounded-full bg-primary/15 px-1.5 text-xs text-primary">{active}</span>}
        </Popover.Trigger>
        <Popover.Portal>
          <Popover.Positioner sideOffset={4} align="start" collisionPadding={16} className="z-50">
            <Popover.Popup aria-label={t("filters.title")} className="w-80 max-w-[calc(100vw-32px)] space-y-3 rounded-lg bg-popover p-3 text-popover-foreground shadow-md ring-1 ring-foreground/10">
              <form className="space-y-3" onSubmit={(e) => { e.preventDefault(); apply(); }}>
                <div className="max-h-80 space-y-3 overflow-y-auto">
                  {fields.map((r) => (
                    <div key={r.key} className="space-y-1.5">
                      <Label htmlFor={`flt-${r.key}`}>{r.field.name}</Label>
                      {control(r)}
                    </div>
                  ))}
                </div>
                <div className="flex justify-between gap-2">
                  <Button type="button" variant="ghost" onClick={() => { setDraft({}); onChange({}); setOpen(false); }}>{t("filters.clear")}</Button>
                  <Button type="submit">{t("filters.apply")}</Button>
                </div>
              </form>
            </Popover.Popup>
          </Popover.Positioner>
        </Popover.Portal>
      </Popover.Root>
      {active > 0 && (
        <ul className="flex basis-full flex-wrap gap-1.5" aria-label={t("filters.active")}>
          {Object.entries(value).map(([k, v]) => (
            <li key={k} className="flex items-center gap-1 rounded-full border bg-muted/40 py-0.5 pl-2.5 pr-0.5 text-xs">
              <span><span className="text-muted-foreground">{label(k)}:</span> {show(v)}</span>
              <button type="button" className="grid size-6 place-items-center rounded-full hover:bg-accent" aria-label={`${t("filters.remove")}: ${label(k)}`} onClick={() => remove(k)}><X className="size-3" /></button>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
