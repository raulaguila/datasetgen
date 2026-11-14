"use client";

import { FormError } from "@/components/bits";
import { useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { FileUp, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { fieldsOf } from "@/components/field-builder";
import { records } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/errors";
import { batches, parseImport, type ParsedRecord } from "@/lib/import-file";
import { guessColumns, parseCsv, tableRecords } from "@/lib/infer-schema";
import { parseXlsx } from "@/lib/xlsx";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";

const SHOWN_ERRORS = 20;
/** Select value for "no field" (a skipped column, or plain create without upsert). */
const NONE = "-";

interface Table {
  name: string;
  rows: string[][];
  /** The field key each column fills; NONE skips it. */
  columns: string[];
}

interface Checked {
  name: string;
  batches: ParsedRecord[][];
  valid: number;
  /** Invalid records and unreadable lines, by line, with the first message. */
  problems: { line: number; message: string }[];
}

/**
 * Imports a JSON Lines, JSON array, CSV or Excel file in two steps: every batch
 * is checked first (dry run), then sent. A table first maps its columns to
 * fields; picking a unique field updates the records already holding a row's
 * value instead of creating copies. Batches commit one by one, so a failure midway
 * leaves the earlier batches saved; the result says how many were created.
 */
export function RecordImportDialog({ collectionId, schema, open, onOpenChange }: { collectionId: string; schema?: Record<string, unknown>; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const [checked, setChecked] = useState<Checked | null>(null);
  const [table, setTable] = useState<Table | null>(null);
  const [upsert, setUpsert] = useState(NONE);
  const fields = useMemo(() => fieldsOf(schema ?? null), [schema]);
  const uniques = fields.filter((r) => r.field.unique);
  const upsertKey = upsert === NONE ? undefined : upsert;
  const [skipInvalid, setSkipInvalid] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setChecked(null);
    setTable(null);
    setSkipInvalid(false);
    setError(null);
  };
  const close = (o: boolean) => {
    if (busy) return;
    if (!o) reset();
    onOpenChange(o);
  };

  const pick = async (file: File) => {
    reset();
    try {
      const xlsx = /\.xlsx$/i.test(file.name);
      if (!xlsx && !/\.csv$/i.test(file.name)) return void check(file.name, parseImport(await file.text()));
      const rows = xlsx ? await parseXlsx(await file.arrayBuffer()) : parseCsv(await file.text());
      // A header no field matches starts skipped, and the mapping shows for the person to fix; else the check runs at once.
      const columns = guessColumns(rows[0] ?? [], schema ?? null).map((k) => (fields.some((f) => f.key === k) ? k : NONE));
      setTable({ name: file.name, rows, columns });
      if (!columns.includes(NONE)) void check(file.name, tableRecords(rows, schema ?? null, columns));
    } catch (err) {
      setError(err instanceof Error && err.message.startsWith("xlsx:") ? t("import.bad_xlsx") : errorMessage(err, "common.save_failed", t));
    }
  };

  const checkTable = () => {
    if (table) void check(table.name, tableRecords(table.rows, schema ?? null, table.columns.map((c) => (c === NONE ? "" : c))));
  };

  const check = async (name: string, parsed: ReturnType<typeof parseImport>) => {
    setChecked(null);
    setError(null);
    setBusy(t("import.checking"));
    try {
      const split = batches(parsed.records);
      const problems = [
        ...parsed.errors.map((e) => ({ line: e.line, message: t("import.not_json") })),
        ...split.tooLarge.map((line) => ({ line, message: t("import.too_large") })),
      ];
      let valid = 0;
      for (const [n, batch] of split.batches.entries()) {
        setBusy(t("import.checking_n").replace("{n}", String(n + 1)).replace("{total}", String(split.batches.length)));
        const res = await records.importBatch({ collection_id: collectionId, records: batch.map((r) => r.data), dry_run: true, upsert_key: upsertKey });
        valid += batch.length - res.invalid;
        for (const e of res.errors) problems.push({ line: batch[e.index].line, message: e.issues.map((i) => `${i.path || "/"}: ${i.message}`).join("; ") });
        // Beyond the first 100 per batch the server only counts them.
        for (let i = res.errors.length; i < res.invalid; i++) problems.push({ line: 0, message: "" });
      }
      problems.sort((a, b) => a.line - b.line);
      setChecked({ name, batches: split.batches, valid, problems });
    } catch (err) {
      setError(errorMessage(err, "common.save_failed", t));
    } finally {
      setBusy(null);
    }
  };

  const send = async () => {
    if (!checked) return;
    let created = 0;
    let updated = 0;
    setError(null);
    try {
      for (const [n, batch] of checked.batches.entries()) {
        setBusy(t("import.sending_n").replace("{n}", String(n + 1)).replace("{total}", String(checked.batches.length)));
        const res = await records.importBatch({ collection_id: collectionId, records: batch.map((r) => r.data), skip_invalid: true, upsert_key: upsertKey });
        created += res.created;
        updated += res.updated ?? 0;
      }
      toast.success(upsertKey ? t("import.done_upsert").replace("{n}", String(created)).replace("{u}", String(updated)) : plural(t("import.done"), created));
      void client.invalidateQueries({ queryKey: ["records", collectionId] });
      void client.invalidateQueries({ queryKey: ["collections"] });
      setBusy(null);
      close(false);
    } catch (err) {
      setError(`${errorMessage(err, "common.save_failed", t)} ${plural(t("import.partial"), created)}`);
      void client.invalidateQueries({ queryKey: ["records", collectionId] });
      setBusy(null);
    }
  };

  const invalid = checked?.problems.length ?? 0;
  const canSend = !!checked && checked.valid > 0 && (invalid === 0 || skipInvalid) && !busy;

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-lg" aria-busy={!!busy}>
        <DialogHeader>
          <DialogTitle>{t("import.title")}</DialogTitle>
          <DialogDescription>{t("import.desc")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Label htmlFor="import-file" className="flex cursor-pointer flex-col items-center gap-2 rounded-lg border border-dashed p-6 text-center text-sm hover:bg-muted/40">
            <FileUp className="size-5 text-muted-foreground" aria-hidden />
            <span>{checked?.name ?? table?.name ?? t("import.pick")}</span>
            <span className="text-xs font-normal text-muted-foreground">{t("import.formats")}</span>
          </Label>
          <input
            id="import-file"
            type="file"
            accept=".jsonl,.ndjson,.json,.csv,.xlsx,application/json,application/x-ndjson,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
            className="sr-only"
            disabled={!!busy}
            onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ""; if (f) void pick(f); }}
          />
          {uniques.length > 0 && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="import-upsert">{t("import.upsert")}</Label>
              <Select value={upsert} items={{ [NONE]: t("import.upsert_none"), ...Object.fromEntries(uniques.map((u) => [u.key, u.field.name])) }}
                onValueChange={(v) => { setUpsert(String(v)); setChecked(null); }}>
                <SelectTrigger id="import-upsert" className="w-full" disabled={!!busy}><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>{t("import.upsert_none")}</SelectItem>
                  {uniques.map((u) => <SelectItem key={u.key} value={u.key}>{u.field.name}</SelectItem>)}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t("import.upsert_hint")}</p>
            </div>
          )}
          {table && !checked && (
            <fieldset className="space-y-2" disabled={!!busy}>
              <legend className="text-sm font-medium">{t("import.columns")}</legend>
              <ul className="max-h-56 space-y-1.5 overflow-auto">
                {(table.rows[0] ?? []).map((h, i) => {
                  const col = table.columns[i] ?? NONE;
                  const items: Record<string, string> = { [NONE]: t("import.column_skip"), ...Object.fromEntries(fields.map((f) => [f.key, f.field.name])) };
                  return (
                    <li key={i} className="grid grid-cols-2 items-center gap-2 text-sm">
                      <span className="truncate" title={h}>{h || `#${i + 1}`}</span>
                      <Select value={col} items={items} onValueChange={(v) => v != null && setTable({ ...table, columns: table.columns.map((c, j) => (j === i ? String(v) : c)) })}>
                        <SelectTrigger aria-label={`${t("import.column_for")} ${h || i + 1}`} className="w-full"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          {Object.entries(items).map(([k, v]) => <SelectItem key={k} value={k}>{v}</SelectItem>)}
                        </SelectContent>
                      </Select>
                    </li>
                  );
                })}
              </ul>
              <Button variant="outline" size="sm" onClick={checkTable}>{t("import.check")}</Button>
            </fieldset>
          )}
          {busy && <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground"><Loader2 className="size-4 animate-spin" />{busy}</p>}
          {checked && !busy && (<>
            <Button variant="link" size="sm" className="h-auto p-0" onClick={() => setChecked(null)} hidden={!table}>{t("import.remap")}</Button>
            <div role="status" className="space-y-2 text-sm">
              <p>
                {plural(t("import.summary"), checked.valid).replace("{invalid}", String(invalid))}
              </p>
              {invalid > 0 && (
                <>
                  <ul className="max-h-40 space-y-1 overflow-auto rounded-md border bg-muted/30 p-2 text-xs" aria-label={t("import.problems")}>
                    {checked.problems.filter((p) => p.line > 0).slice(0, SHOWN_ERRORS).map((p, i) => (
                      <li key={i}><span className="font-medium">{t("import.line")} {p.line}</span>: <span className="text-destructive">{p.message}</span></li>
                    ))}
                  </ul>
                  {checked.valid > 0 && (
                    <label className="flex items-center gap-2">
                      <Checkbox checked={skipInvalid} onCheckedChange={(v) => setSkipInvalid(v === true)} />
                      {t("import.skip_invalid")}
                    </label>
                  )}
                </>
              )}
            </div>
          </>)}
          <FormError message={error} />
        </div>
        <DialogFooter>
          <Button variant="ghost" disabled={!!busy} onClick={() => close(false)}>{t("common.cancel")}</Button>
          <Button disabled={!canSend} onClick={() => void send()}>
            {busy && checked && <Loader2 className="size-4 animate-spin" />}
            {t("import.send").replace("{n}", String(checked?.valid ?? 0))}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
