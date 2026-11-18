"use client";

import { FormError } from "@/components/bits";
import { useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { FileUp, Loader2 } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { fieldsOf } from "@/components/field-builder";
import { records as records_ } from "@/lib/api/endpoints";
import type { ImportResult } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { Cancelled, eachBatch, importId, loadProgress, readHeader, saveProgress, withRetry, type RetryWait, type SendProgress } from "@/lib/import-client";
import { isTable, type Batch, type LineProblem } from "@/lib/import-stream";
import { guessColumns } from "@/lib/infer-schema";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";

const SHOWN_ERRORS = 20;
/** Problems kept to list; past it they are only counted. */
const KEPT_PROBLEMS = 200;
/** Select value for "no field" (a skipped column, or plain create without upsert). */
const NONE = "-";

interface Table {
  /** The file's header row. */
  header: string[];
  /** The field key each column fills; NONE skips it. */
  columns: string[];
}

interface Checked {
  valid: number;
  /** All invalid records and unreadable lines, counted. */
  invalid: number;
  /** The first of them, by line, with the first message. */
  problems: { line: number; message: string }[];
  /** Records an earlier send of this file already saved. */
  saved: number;
}

/** What runs now: a label, the share of the file read, and a wait for the request limit. */
interface Busy {
  label: string;
  read?: number;
  records?: number;
  wait?: RetryWait | null;
}

/**
 * Imports a JSON Lines, JSON array, CSV or Excel file of any size in two passes,
 * read in the import worker (lib/import.worker.ts) so the page keeps answering:
 * every batch is checked first (dry run), then the file is read again and sent.
 * Only one batch is in memory at a time. A table first maps its columns to
 * fields; picking a unique field updates the records already holding a row's
 * value instead of creating copies. Batches commit one by one; each carries a
 * key (the file's id and its position), so a request repeated after a lost
 * answer saves nothing twice, and failures that may pass are tried again. Where a
 * send stopped is kept in this browser: the same file continues from there.
 */
export function RecordImportDialog({ collectionId, schema, open, onOpenChange, initialFile }: {
  collectionId: string;
  schema?: Record<string, unknown>;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** A file already chosen (on the new-collection page): its check starts when the dialog opens. */
  initialFile?: File | null;
}) {
  const { t, locale } = useI18n();
  const num = (n: number) => n.toLocaleString(locale);
  const client = useQueryClient();
  const [file, setFile] = useState<File | null>(null);
  const [checked, setChecked] = useState<Checked | null>(null);
  const [table, setTable] = useState<Table | null>(null);
  const [upsert, setUpsert] = useState(NONE);
  const fields = useMemo(() => fieldsOf(schema ?? null), [schema]);
  const uniques = fields.filter((r) => r.field.unique);
  const upsertKey = upsert === NONE ? undefined : upsert;
  const [skipInvalid, setSkipInvalid] = useState(false);
  const [busy, setBusy] = useState<Busy | null>(null);
  const [error, setError] = useState<string | null>(null);
  /** An earlier, stopped send of this file into this collection. */
  const [resumed, setResumed] = useState<SendProgress | null>(null);
  const running = useRef<AbortController | null>(null);

  const reset = () => {
    running.current?.abort();
    setFile(null);
    setChecked(null);
    setTable(null);
    setSkipInvalid(false);
    setError(null);
    setResumed(null);
  };
  const close = (o: boolean) => {
    if (busy) return;
    if (!o) reset();
    onOpenChange(o);
  };
  const stop = () => running.current?.abort();

  const lineMessage = (p: LineProblem) =>
    p.code === "parse" ? t("import.not_json") : p.code === "too_large" ? t("import.too_large") : t("import.duplicate").replace("{field}", fields.find((f) => f.key === p.field)?.field.name ?? p.field ?? "");

  /**
   * Reads the whole file once more, batch by batch: send false checks, true saves.
   * The first skip batches were saved by an earlier send of this file: they are
   * read (the batches after them must come out the same) but not sent.
   */
  const pass = async (f: File, cols: string[] | undefined, send: boolean, progress: SendProgress | null, selectedKey: string | undefined, onBatch: (b: Batch, res: ImportResult | null, n: number) => void) => {
    const ctl = new AbortController();
    running.current = ctl;
    const label = t(send ? "import.sending" : "import.checking");
    const checkpointId = send && progress ? importId(collectionId, schema, f, cols, selectedKey) : null;
    let records = 0;
    let n = 0;
    setBusy({ label, read: 0, records: 0 });
    await eachBatch(f, { schema: schema ?? null, columns: cols, upsertKey: selectedKey, legacyDuplicates: !!progress && !progress.version }, async (b) => {
      const i = n;
      if (b.records.length) n++;
      if (checkpointId && progress && b.records.length && i >= progress.sent) {
        progress.pending = i;
        saveProgress(checkpointId, progress);
      }
      const res = b.records.length && i >= (progress?.sent ?? 0) && (send || i !== progress?.pending)
        ? await withRetry(
          () => records_.importBatch({
            collection_id: collectionId, records: b.records.map((r) => r.data), upsert_key: selectedKey,
            ...(b.records.some((r) => r.sourceJSON) ? { sourceRecords: b.records.map((r) => r.sourceJSON ?? JSON.stringify(r.data)) } : {}),
            ...(send ? { skip_invalid: true, batch_key: `${progress!.fid}:${i}` } : { dry_run: true }),
          }),
          (wait) => setBusy((x) => x && { ...x, wait }),
          ctl.signal)
        : null;
      records += b.records.length + b.problems.length;
      onBatch(b, res, i);
      setBusy((x) => x && { ...x, records });
    }, (read) => setBusy((x) => x && { ...x, read }), ctl.signal);
  };

  const check = async (f: File, cols?: string[], keySelection: string = upsertKey ?? NONE) => {
    const selectedKey = keySelection === NONE ? undefined : keySelection;
    setChecked(null);
    setError(null);
    const out: Checked = { valid: 0, invalid: 0, problems: [], saved: 0 };
    const keep = (line: number, message: string) => { out.invalid++; if (out.problems.length < KEPT_PROBLEMS) out.problems.push({ line, message }); };
    // Batches an earlier send saved are not checked again: their unique values would now read as taken.
    const before = loadProgress(importId(collectionId, schema, f, cols, selectedKey));
    out.saved = (before?.created ?? 0) + (before?.updated ?? 0);
    setResumed(before);
    try {
      await pass(f, cols, false, before, selectedKey, (b, res, i) => {
        if (before && i < before.sent) return;
        for (const p of b.problems) keep(p.line, lineMessage(p));
        if (before?.pending === i && b.records.length) {
          // A lost acknowledgement is resolved by replaying the same batch key.
          out.valid += b.records.length;
          return;
        }
        if (!res) return;
        out.valid += b.records.length - res.invalid;
        for (const e of res.errors) keep(b.records[e.index].line, e.issues.map((i) => `${i.path || "/"}: ${i.message}`).join("; "));
        // Beyond the first 100 per batch the server only counts them.
        out.invalid += res.invalid - res.errors.length;
      });
      out.problems.sort((a, b) => a.line - b.line);
      setChecked(out);
    } catch (err) {
      if (!(err instanceof Cancelled)) setError(errorMessage(err, "common.save_failed", t));
    } finally {
      setBusy(null);
    }
  };

  const pick = async (f: File) => {
    reset();
    setFile(f);
    try {
      if (!isTable(f)) return void check(f);
      setBusy({ label: t("import.reading") });
      const header = await readHeader(f);
      setBusy(null);
      // A header no field matches starts skipped, and the mapping shows for the person to fix; else the check runs at once.
      const columns = guessColumns(header, schema ?? null).map((k) => (fields.some((x) => x.key === k) ? k : NONE));
      setTable({ header, columns });
      if (!columns.includes(NONE)) void check(f, columns);
    } catch (err) {
      setBusy(null);
      setError(err instanceof Error && err.message.startsWith("xlsx:") ? t("import.bad_xlsx") : errorMessage(err, "common.save_failed", t));
    }
  };

  const mapped = () => table?.columns.map((c) => (c === NONE ? "" : c));
  const recheck = () => { if (file) void check(file, mapped()); };

  const send = async () => {
    if (!file || !checked) return;
    const cols = mapped();
    const id = importId(collectionId, schema, file, cols, upsertKey);
    const progress: SendProgress = loadProgress(id) ?? { version: 2, fid: crypto.randomUUID(), sent: 0, created: 0, updated: 0, at: Date.now() };
    const skip = progress.sent;
    setError(null);
    try {
      await pass(file, cols, true, progress, upsertKey, (b, res, i) => {
        if (!b.records.length || i < skip) return;
        progress.created += res?.created ?? 0;
        progress.updated += res?.updated ?? 0;
        progress.sent = i + 1;
        delete progress.pending;
        saveProgress(id, progress);
      });
      saveProgress(id, null);
      const { created, updated } = progress;
      toast.success(upsertKey ? t("import.done_upsert").replace("{n}", num(created)).replace("{u}", num(updated)) : plural(t("import.done"), created, num(created)));
      // The check counted what should go in; less means records changed between the check and the send.
      const missing = checked.valid + checked.saved - created - updated;
      if (missing > 0) toast.warning(plural(t("import.short"), missing, num(missing)), { duration: 15_000 });
      void client.invalidateQueries({ queryKey: ["records", collectionId] });
      void client.invalidateQueries({ queryKey: ["collections"] });
      setBusy(null);
      close(false);
    } catch (err) {
      const why = err instanceof Cancelled ? t("import.stopped") : errorMessage(err, "common.save_failed", t);
      setError(`${why} ${plural(t("import.partial"), progress.created + progress.updated, num(progress.created + progress.updated))} ${t("import.resume_later")}`);
      setResumed(loadProgress(id));
      void client.invalidateQueries({ queryKey: ["records", collectionId] });
      setBusy(null);
    }
  };

  /** Forgets where an earlier send stopped: the next send starts from the first line. */
  const startOver = () => {
    if (!file) return;
    saveProgress(importId(collectionId, schema, file, mapped(), upsertKey), null);
    void check(file, mapped());
  };

  const invalid = checked?.invalid ?? 0;
  const pct = busy?.read !== undefined && file?.size ? Math.min(100, Math.round((busy.read / file.size) * 100)) : null;
  const canSend = !!checked && checked.valid > 0 && (invalid === 0 || skipInvalid) && !busy;

  // Once: the ref also keeps React's dev double-run from starting a second check.
  const started = useRef(false);
  useEffect(() => {
    if (!open || !initialFile || started.current) return;
    started.current = true;
    void pick(initialFile);
  }, [open, initialFile]); // eslint-disable-line react-hooks/exhaustive-deps -- pick is recreated each render; the ref runs it once

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
            <span>{file?.name ?? t("import.pick")}</span>
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
                onValueChange={(v) => { setUpsert(String(v)); setChecked(null); if (file && !table) void check(file, undefined, String(v)); }}>
                <SelectTrigger id="import-upsert" className="w-full" disabled={!!busy}><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>{t("import.upsert_none")}</SelectItem>
                  {uniques.map((u) => <SelectItem key={u.key} value={u.key}>{u.field.name}</SelectItem>)}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">{t("import.upsert_hint")}</p>
            </div>
          )}
          {table && !checked && !busy && (
            <fieldset className="space-y-2" disabled={!!busy}>
              <legend className="text-sm font-medium">{t("import.columns")}</legend>
              <ul className="max-h-56 space-y-1.5 overflow-auto">
                {table.header.map((h, i) => {
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
              <Button variant="outline" size="sm" onClick={recheck}>{t("import.check")}</Button>
            </fieldset>
          )}
          {busy && (
            <div role="status" className="space-y-1.5 text-sm text-muted-foreground">
              <p className="flex items-center gap-2"><Loader2 className="size-4 animate-spin" />{busy.label}{pct !== null && ` ${pct}%`}</p>
              {pct !== null && (
                <div role="progressbar" aria-label={busy.label} aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct} className="h-1.5 overflow-hidden rounded-full bg-muted">
                  <div className="h-full bg-primary transition-[width]" style={{ width: `${pct}%` }} />
                </div>
              )}
              {!!busy.records && <p className="text-xs">{plural(t("import.records_read"), busy.records, num(busy.records))}</p>}
              {busy.wait && <p className="text-xs">{t(busy.wait.reason === "limit" ? "import.waiting" : "import.retrying").replace("{s}", String(busy.wait.seconds))}</p>}
            </div>
          )}
          {checked && !busy && (<>
            <Button variant="link" size="sm" className="h-auto p-0" onClick={() => setChecked(null)} hidden={!table}>{t("import.remap")}</Button>
            <div role="status" className="space-y-2 text-sm">
              <p>
                {plural(t("import.summary"), checked.valid, num(checked.valid)).replace("{invalid}", num(invalid))}
              </p>
              {resumed && (
                <p className="rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground">
                  {plural(t("import.resume_note"), checked.saved, num(checked.saved))}{" "}
                  <Button variant="link" size="sm" className="h-auto p-0 text-xs" onClick={startOver}>{t("import.start_over")}</Button>
                </p>
              )}
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
          {busy ? <Button variant="ghost" onClick={stop}>{t("import.stop")}</Button> : <Button variant="ghost" onClick={() => close(false)}>{t("common.cancel")}</Button>}
          <Button disabled={!canSend} onClick={() => void send()}>
            
            {t(resumed ? "import.resume" : "import.send").replace("{n}", num(checked?.valid ?? 0))}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
