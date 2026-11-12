"use client";

import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { FileUp } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { records } from "@/lib/api/endpoints";
import { errorMessage } from "@/lib/errors";
import { batches, parseImport, type ParsedRecord } from "@/lib/import-file";
import { useI18n } from "@/lib/i18n/provider";

const SHOWN_ERRORS = 20;

interface Checked {
  name: string;
  batches: ParsedRecord[][];
  valid: number;
  /** Invalid records and unreadable lines, by line, with the first message. */
  problems: { line: number; message: string }[];
}

/**
 * Imports a JSON Lines or JSON array file in two steps: every batch is checked
 * first (dry run), then sent. Batches commit one by one, so a failure midway
 * leaves the earlier batches saved; the result says how many were created.
 */
export function RecordImportDialog({ collectionId, open, onOpenChange }: { collectionId: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const [checked, setChecked] = useState<Checked | null>(null);
  const [skipInvalid, setSkipInvalid] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setChecked(null);
    setSkipInvalid(false);
    setError(null);
  };
  const close = (o: boolean) => {
    if (busy) return;
    if (!o) reset();
    onOpenChange(o);
  };

  const check = async (file: File) => {
    reset();
    setBusy(t("import.checking"));
    try {
      const parsed = parseImport(await file.text());
      const split = batches(parsed.records);
      const problems = [
        ...parsed.errors.map((e) => ({ line: e.line, message: t("import.not_json") })),
        ...split.tooLarge.map((line) => ({ line, message: t("import.too_large") })),
      ];
      let valid = 0;
      for (const [n, batch] of split.batches.entries()) {
        setBusy(t("import.checking_n").replace("{n}", String(n + 1)).replace("{total}", String(split.batches.length)));
        const res = await records.importBatch({ collection_id: collectionId, records: batch.map((r) => r.data), dry_run: true });
        valid += batch.length - res.invalid;
        for (const e of res.errors) problems.push({ line: batch[e.index].line, message: e.issues.map((i) => `${i.path || "/"}: ${i.message}`).join("; ") });
        // Beyond the first 100 per batch the server only counts them.
        for (let i = res.errors.length; i < res.invalid; i++) problems.push({ line: 0, message: "" });
      }
      problems.sort((a, b) => a.line - b.line);
      setChecked({ name: file.name, batches: split.batches, valid, problems });
    } catch (err) {
      setError(errorMessage(err, "common.save_failed", t));
    } finally {
      setBusy(null);
    }
  };

  const send = async () => {
    if (!checked) return;
    let created = 0;
    setError(null);
    try {
      for (const [n, batch] of checked.batches.entries()) {
        setBusy(t("import.sending_n").replace("{n}", String(n + 1)).replace("{total}", String(checked.batches.length)));
        const res = await records.importBatch({ collection_id: collectionId, records: batch.map((r) => r.data), skip_invalid: true });
        created += res.created;
      }
      toast.success(t("import.done").replace("{n}", String(created)));
      void client.invalidateQueries({ queryKey: ["records", collectionId] });
      void client.invalidateQueries({ queryKey: ["collections"] });
      setBusy(null);
      close(false);
    } catch (err) {
      setError(`${errorMessage(err, "common.save_failed", t)} ${t("import.partial").replace("{n}", String(created))}`);
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
            <span>{checked ? checked.name : t("import.pick")}</span>
            <span className="text-xs font-normal text-muted-foreground">{t("import.formats")}</span>
          </Label>
          <input
            id="import-file"
            type="file"
            accept=".jsonl,.ndjson,.json,application/json,application/x-ndjson"
            className="sr-only"
            disabled={!!busy}
            onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ""; if (f) void check(f); }}
          />
          {busy && <p role="status" className="text-sm text-muted-foreground">{busy}</p>}
          {checked && !busy && (
            <div role="status" className="space-y-2 text-sm">
              <p>
                {t("import.summary").replace("{valid}", String(checked.valid)).replace("{invalid}", String(invalid))}
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
          )}
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        </div>
        <DialogFooter>
          <Button variant="ghost" disabled={!!busy} onClick={() => close(false)}>{t("common.cancel")}</Button>
          <Button disabled={!canSend} onClick={() => void send()}>
            {t("import.send").replace("{n}", String(checked?.valid ?? 0))}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
