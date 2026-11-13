"use client";

import { useState } from "react";
import { Popover } from "@base-ui/react/popover";
import { useQueryClient } from "@tanstack/react-query";
import { Columns3 } from "lucide-react";
import { toast } from "sonner";
import { type FieldRow, type FieldType } from "@/components/field-builder";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { records } from "@/lib/api/endpoints";
import { ApiError } from "@/lib/api/client";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";

/** Types a list cell edits in place; the rest open the record. */
const INLINE: FieldType[] = ["string", "email", "url", "number", "integer", "boolean", "enum", "date"];
export const editsInline = (r: FieldRow) => !r.advanced && INLINE.includes(r.field.type);

/** Fields a list can show as a column (their value fits a cell). */
export const showable = (rows: FieldRow[]) => rows.filter((r) => !r.advanced && !["group", "json"].includes(r.field.type));

/** The columns shown: those in cols (in field order), or the first four. */
export function pickColumns(rows: FieldRow[], cols: string): FieldRow[] {
  const all = showable(rows);
  if (!cols) return all.filter((r) => r.field.type !== "strings" && r.field.type !== "text").slice(0, 4);
  const keys = cols.split(",");
  return all.filter((r) => keys.includes(r.key));
}

/** Chooses the list's columns; kept in the URL, so a saved view keeps them too. */
export function ColumnPicker({ fields, value, onChange }: { fields: FieldRow[]; value: FieldRow[]; onChange: (keys: string[]) => void }) {
  const { t } = useI18n();
  const all = showable(fields);
  const on = new Set(value.map((r) => r.key));
  const toggle = (key: string, checked: boolean) => {
    const next = all.filter((r) => (r.key === key ? checked : on.has(r.key))).map((r) => r.key);
    if (next.length) onChange(next);
  };
  return (
    <Popover.Root>
      <Popover.Trigger render={<Button variant="outline" />}><Columns3 className="size-4" />{t("grid.columns")}</Popover.Trigger>
      <Popover.Portal>
        <Popover.Positioner sideOffset={4} align="start" collisionPadding={16} className="z-50">
          <Popover.Popup aria-label={t("grid.columns")} className="max-h-96 w-64 space-y-1 overflow-y-auto rounded-lg bg-popover p-2 text-popover-foreground shadow-md ring-1 ring-foreground/10">
            {all.map((r) => (
              <label key={r.key} className="flex items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted">
                <Checkbox checked={on.has(r.key)} disabled={on.has(r.key) && on.size === 1} onCheckedChange={(c) => toggle(r.key, c === true)} />
                <span className="truncate">{r.field.name}</span>
              </label>
            ))}
          </Popover.Popup>
        </Popover.Positioner>
      </Popover.Portal>
    </Popover.Root>
  );
}

/** Saves one field of one record (the other fields stay as they are, unseen). */
export function useSaveField(collectionId: string) {
  const { t } = useI18n();
  const client = useQueryClient();
  return async (recordId: string, key: string, value: unknown) => {
    try {
      await records.bulkUpdate([recordId], { [key]: value === undefined || value === "" ? null : value });
      void client.invalidateQueries({ queryKey: ["records", collectionId] });
      return true;
    } catch (err) {
      const issue = err instanceof ApiError ? err.message : null;
      toast.error(issue ?? errorMessage(err, "common.save_failed", t));
      return false;
    }
  };
}

/**
 * A cell that turns into an input on double click (or Enter when focused):
 * Enter or leaving saves, Escape cancels.
 */
export function EditableCell({ row, value, text, recordId, save, className }: {
  row: FieldRow;
  value: unknown;
  text: string;
  recordId: string;
  save: (recordId: string, key: string, value: unknown) => Promise<boolean>;
  className?: string;
}) {
  const { t } = useI18n();
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const type = row.field.type;
  const start = () => {
    // The list carries text cut to 120 characters: longer text is edited in the record, never saved cut.
    if (typeof value === "string" && value.length >= 120) return void toast.info(t("grid.too_long"));
    setDraft(value === undefined || value === null ? "" : String(value));
    setEditing(true);
  };
  const commit = async (raw: string) => {
    if (busy) return;
    const next = type === "number" || type === "integer" ? (raw.trim() === "" ? undefined : Number(raw)) : type === "boolean" ? raw === "true" : raw;
    if (next === value || (next === undefined && (value === undefined || value === null)) || (typeof next === "number" && !Number.isFinite(next))) { setEditing(false); return; }
    setBusy(true);
    const ok = await save(recordId, row.key, next);
    setBusy(false);
    if (ok) setEditing(false);
  };
  const label = `${t("grid.edit")}: ${row.field.name}`;
  if (!editing) {
    return (
      <span role="button" tabIndex={0} aria-label={`${label}, ${text}`} title={t("grid.edit_hint")} className={`block min-h-5 truncate rounded px-1 -mx-1 hover:bg-muted ${className ?? ""}`}
        onClick={(e) => e.stopPropagation()} onDoubleClick={(e) => { e.stopPropagation(); start(); }}
        onKeyDown={(e) => { if (e.key === "Enter" || e.key === "F2") { e.preventDefault(); e.stopPropagation(); start(); } }}>
        {text}
      </span>
    );
  }
  const keys = (e: React.KeyboardEvent) => {
    e.stopPropagation();
    if (e.key === "Escape") setEditing(false);
    if (e.key === "Enter") { e.preventDefault(); void commit((e.target as HTMLInputElement | HTMLSelectElement).value); }
  };
  if (type === "boolean" || type === "enum") {
    const options = type === "boolean" ? [["true", t("common.yes")], ["false", t("common.no")]] : row.field.options.split(",").map((o) => o.trim()).filter(Boolean).map((o) => [o, o]);
    return (
      <select autoFocus aria-label={label} disabled={busy} className="h-7 w-full rounded border bg-background px-1 text-sm" defaultValue={draft}
        onClick={(e) => e.stopPropagation()} onKeyDown={keys} onChange={(e) => void commit(e.target.value)} onBlur={() => setEditing(false)}>
        {!row.field.required && <option value="">—</option>}
        {options.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
      </select>
    );
  }
  return (
    <input autoFocus aria-label={label} disabled={busy} className="h-7 w-full rounded border bg-background px-1 text-sm"
      type={type === "number" || type === "integer" ? "number" : type === "date" ? "date" : type === "email" ? "email" : "text"}
      step={type === "integer" ? 1 : "any"} value={draft} onChange={(e) => setDraft(e.target.value)}
      onClick={(e) => e.stopPropagation()} onKeyDown={keys} onBlur={() => void commit(draft)} />
  );
}
