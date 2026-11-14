"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ChevronDown, CircleCheck, ListChecks, RefreshCw, Trash2, Wrench } from "lucide-react";
import { toast } from "sonner";
import { FormSection, FormSections, useFormatDate } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { EmptyState } from "@/components/empty-state";
import { fieldsOf, recordTitle, type Field } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { collections } from "@/lib/api/endpoints";
import type { Collection, QualityReport, RecordFix } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { useQuality } from "@/lib/hooks/use-quality";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

type Fix = Omit<RecordFix, "dry_run">;

/** A field's value typed the way its type reads: a number box, yes/no, its options, a date. */
function ValueInput({ field, value, onChange }: { field?: Field; value: string; onChange: (v: string) => void }) {
  const { t } = useI18n();
  const props = { id: "fix-value", value, "aria-describedby": "fix-value-hint" };
  const choices = field?.type === "boolean" ? { true: t("common.yes"), false: t("common.no") }
    : field?.type === "enum" ? Object.fromEntries(field.options.split(",").map((o) => o.trim()).filter(Boolean).map((o) => [o, o]))
    : null;
  if (choices) return (
    <Select value={value || null} items={choices} onValueChange={(v) => onChange(String(v ?? ""))}>
      <SelectTrigger id="fix-value" className="w-full" aria-describedby="fix-value-hint"><SelectValue placeholder={t("fix.pick")} /></SelectTrigger>
      <SelectContent>{Object.entries(choices).map(([v, label]) => <SelectItem key={v} value={v}>{label}</SelectItem>)}</SelectContent>
    </Select>
  );
  const type = field?.type === "integer" || field?.type === "number" ? "number" : field?.type === "date" ? "date" : field?.type === "email" ? "email" : field?.type === "url" ? "url" : "text";
  return <Input {...props} type={type} step={field?.type === "integer" ? 1 : "any"} onChange={(e) => onChange(e.target.value)} />;
}

/** The JSON a typed box's text stands for; free fields read JSON and fall back to text. */
function valueOf(field: Field | undefined, text: string): unknown {
  switch (field?.type) {
    case "integer": case "number": return Number(text);
    case "boolean": return text === "true";
    case "string": case "text": case "enum": case "date": case "datetime": case "email": case "url": return text;
  }
  try { return JSON.parse(text) as unknown; } catch { return text; }
}

/** The fix form: one bulk change, counted, confirmed, then the check runs again. */
function BulkFix({ collection, report, onDone, fieldLabel, issueText }: { collection: Collection; report: QualityReport; onDone: () => void; fieldLabel: (key: string) => string; issueText: (keyword: string, message: string) => string }) {
  const { t } = useI18n();
  const client = useQueryClient();
  const [op, setOp] = useState<RecordFix["op"]>("set_default");
  const [field, setField] = useState("");
  const [to, setTo] = useState("");
  const [value, setValue] = useState("");
  const [counting, setCounting] = useState(false);
  // The change waiting for its confirmation, with how many records it touches.
  const [confirm, setConfirm] = useState<{ fix: Fix; count: number } | { delete: true } | null>(null);
  const rows = fieldsOf(collection.schema ?? null).filter((r) => r.field.type !== "group");
  const known = new Set(rows.map((r) => r.key));
  // Keys records still hold that the fields no longer have: renamed or removed ones.
  const old = [...new Set(report.invalid.flatMap((r) => Object.keys(r.data ?? {})))].filter((k) => k && !known.has(k));
  const options: Record<string, string> = Object.fromEntries([
    ...rows.map((r) => [r.key, r.field.name] as const),
    ...old.map((k) => [k, t("fix.old_field").replace("{key}", k)] as const),
  ]);
  const target = rows.find((r) => r.key === field)?.field;
  const ready = field !== "" && (op !== "rename" || (to.trim() !== "" && to.trim() !== field)) && (op !== "set_default" || value !== "");

  // What the check found, one line per field and rule, with the fix that fits when there is one.
  const problems = new Map<string, { keyword: string; key: string; message: string; count: number }>();
  for (const rec of report.invalid) {
    for (const i of rec.issues) {
      const segs = i.path.split("/").slice(1).map((x) => x.replace(/~1/g, "/").replace(/~0/g, "~"));
      // A leftover inside a group or list reads as its path, without the item numbers.
      const key = i.keyword === "additionalProperties" ? segs.filter((x) => !/^\d+$/.test(x)).join(" › ") : (segs[0] ?? "");
      const id = `${i.keyword}|${key}`;
      const p = problems.get(id) ?? { keyword: i.keyword, key, message: i.message, count: 0 };
      p.count++;
      problems.set(id, p);
    }
  }
  const actionFor = (keyword: string, k: string): { label: string; run: () => void } | null => {
    if (keyword === "required" && known.has(k)) {
      const f = rows.find((r) => r.key === k)!.field;
      return {
        label: t("fix.suggest_fill").replace("{field}", f.name),
        run: () => {
          setOp("set_default"); setField(k); setValue(f.default); setManual(true);
          if (f.default !== "") void review({ op: "set_default", field: k, value: valueOf(f, f.default) });
          else setTimeout(() => document.getElementById("fix-value")?.focus());
        },
      };
    }
    if (keyword === "additionalProperties" && k && !k.includes(" › ") && !known.has(k)) return {
      label: t("fix.suggest_remove").replace("{field}", k),
      run: () => { setOp("remove"); setField(k); void review({ op: "remove", field: k }); },
    };
    return null;
  };
  const [manual, setManual] = useState(false);

  const after = (n: number, key: MessageKey) => {
    toast.success(t(key).replace("{n}", String(n)));
    void client.invalidateQueries({ queryKey: ["records", collection.id] });
    void client.invalidateQueries({ queryKey: ["collections"] });
    onDone();
  };
  // Counts the records first, so the confirmation says how many change.
  const review = async (fix: Fix) => {
    setCounting(true);
    try {
      const { count } = await collections.fix(collection.id, { ...fix, dry_run: true });
      if (count === 0) toast.info(t("fix.none"));
      else setConfirm({ fix, count });
    } catch (err) {
      toast.error(errorMessage(err, "common.save_failed", t));
    } finally {
      setCounting(false);
    }
  };
  const canDelete = report.status === "done" && report.version === report.current_version && report.invalid_count > 0;
  const ops: Record<RecordFix["op"], string> = { set_default: t("fix.op.set_default"), rename: t("fix.op.rename"), remove: t("fix.op.remove") };
  const pending = confirm && "fix" in confirm ? confirm : null;

  return (
    <FormSections>
      <FormSection title={t("fix.problems")} description={t(report.invalid_count > report.invalid.length ? "fix.problems_first" : "fix.problems_desc").replace("{n}", String(report.invalid.length))}>
        <ul className="divide-y rounded-lg border" aria-label={t("fix.problems")}>
          {[...problems.values()].sort((a, b) => b.count - a.count).map((p) => {
            const action = actionFor(p.keyword, p.key);
            return (
              <li key={`${p.keyword}|${p.key}`} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-3 py-2.5">
                <div className="min-w-0 flex-1">
                  <p className="text-sm"><span className="font-medium">{fieldLabel(p.key)}</span> · <span className="text-destructive">{issueText(p.keyword, p.message)}</span></p>
                  <p className="text-xs text-muted-foreground">{t(p.count === 1 ? "fix.records_one" : "fix.records").replace("{n}", String(p.count))}</p>
                </div>
                {action
                  ? <Button type="button" size="sm" variant="outline" disabled={counting} onClick={action.run}><Wrench className="size-4" />{action.label}</Button>
                  : <span className="text-xs text-muted-foreground">{t("fix.by_hand")}</span>}
              </li>
            );
          })}
        </ul>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button type="button" variant="ghost" className="-ml-2" aria-expanded={manual} onClick={() => setManual((m) => !m)}>
            {t("fix.other")}<ChevronDown className={manual ? "size-4 rotate-180" : "size-4"} />
          </Button>
          {canDelete && (
            <Button type="button" variant="ghost" className="ml-auto text-destructive" onClick={() => setConfirm({ delete: true })}>
              <Trash2 className="size-4" />{t("fix.delete_invalid").replace("{n}", String(report.invalid_count))}
            </Button>
          )}
        </div>
        {manual && <form className="mt-4 space-y-4 rounded-lg border p-4" aria-label={t("fix.other")} onSubmit={(e) => {
          e.preventDefault();
          if (ready) void review({ op, field, ...(op === "rename" ? { to: to.trim() } : {}), ...(op === "set_default" ? { value: valueOf(target, value) } : {}) });
        }}>
          <p className="text-sm text-muted-foreground">{t("fix.desc")}</p>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="fix-op">{t("fix.op")}</Label>
            <Select value={op} items={ops} onValueChange={(v) => setOp(v as RecordFix["op"])}>
              <SelectTrigger id="fix-op" className="w-full"><SelectValue /></SelectTrigger>
              <SelectContent>
                {Object.entries(ops).map(([value, label]) => <SelectItem key={value} value={value}>{label}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="fix-field">{t(op === "rename" ? "fix.from" : "fix.field")}</Label>
            <Select value={field || null} items={options} onValueChange={(v) => { setField(String(v ?? "")); setValue(""); }}>
              <SelectTrigger id="fix-field" className="w-full"><SelectValue placeholder={t("fix.pick")} /></SelectTrigger>
              <SelectContent>{Object.entries(options).map(([k, label]) => <SelectItem key={k} value={k}>{label}</SelectItem>)}</SelectContent>
            </Select>
          </div>
          {op === "rename" && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="fix-to">{t("fix.to")}</Label>
              <Input id="fix-to" list="fix-keys" maxLength={200} value={to} onChange={(e) => setTo(e.target.value)} />
              <datalist id="fix-keys">{rows.map((r) => <option key={r.key} value={r.key}>{r.field.name}</option>)}</datalist>
            </div>
          )}
          {op === "set_default" && (
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="fix-value">{t("fix.value")}</Label>
              <ValueInput key={field} field={target} value={value} onChange={setValue} />
              <p id="fix-value-hint" className="text-xs text-muted-foreground">{t(target ? "fix.value_typed_hint" : "fix.value_hint")}</p>
            </div>
          )}
          <p className="text-xs text-muted-foreground">{t("fix.versions_hint")}</p>
          <Button type="submit" disabled={!ready || counting}><Wrench className="size-4" />{t("fix.apply")}</Button>
        </form>}
      </FormSection>
      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={t(pending ? "fix.confirm_title" : "fix.delete_title")}
        description={pending ? t(pending.count === 1 ? "fix.confirm_one" : "fix.confirm").replace("{n}", String(pending.count)) : t("fix.delete_confirm").replace("{n}", String(report.invalid_count))}
        destructive={!pending || pending.fix.op === "remove"}
        confirmLabel={t(pending ? "fix.apply" : "common.delete")}
        onConfirm={async () => {
          try {
            if (pending) after((await collections.fix(collection.id, pending.fix)).count, "fix.changed");
            else after((await collections.deleteInvalid(collection.id)).count, "fix.deleted");
          } catch (err) {
            toast.error(errorMessage(err, "common.save_failed", t));
          }
          setConfirm(null);
        }}
      />
    </FormSections>
  );
}

/** Records that a change of fields left out of format, each with the rules it breaks. */
function Quality({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const { q, start, stale } = useQuality(collection);
  // Records change without the fields changing: opening this page checks them again, once.
  // shortcut: re-scans the whole collection per visit; compare against the latest record change if checks get slow.
  const refreshed = useRef(false);
  useEffect(() => {
    if (refreshed.current || q.data?.status !== "done" || stale) return;
    refreshed.current = true;
    start.mutate();
  }, [q.data?.status, stale, start]);
  // Issues name the field by its key; show its label, as the form does.
  const schema = collection.schema ?? null;
  const labels = new Map(fieldsOf(schema).map((r) => [r.key, r.field.name]));
  // A group's subfield reads "Endereço › Rua".
  const field = (path: string) => {
    const [key = "", sub] = path.split("/").slice(1).map((p) => p.replace(/~1/g, "/").replace(/~0/g, "~"));
    if (!key) return t("quality.root");
    const name = labels.get(key) ?? key;
    if (!sub) return name;
    const group = (schema?.properties as Record<string, Record<string, unknown>> | undefined)?.[key];
    return `${name} › ${fieldsOf(group ?? null).find((r) => r.key === sub)?.field.name ?? sub}`;
  };
  const issue = (keyword: string, message: string) => {
    const text = t(`quality.issue.${keyword}` as MessageKey);
    return text.startsWith("quality.issue.") ? message : text;
  };
  const preview = (id: string, data: Record<string, unknown>) => recordTitle(collection.schema ?? null, data) ?? `${t("records.record")} ${id.slice(0, 8)}`;

  if (q.isPending) return <ListSkeleton rows={3} />;
  if (q.isError) return <QueryErrorState error={q.error} onRetry={() => void q.refetch()} />;
  const r = q.data;
  const running = r.status === "running" || start.isPending;
  const recheck = (
    <Button variant="outline" disabled={running} onClick={() => start.mutate()}>
      <RefreshCw className={running ? "size-4 animate-spin" : "size-4"} />{t(running ? "quality.running" : "quality.recheck")}
    </Button>
  );
  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-3">
        <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
          {running ? t("quality.running_desc")
            : r.status === "failed" ? t("quality.failed")
            : stale ? t("quality.stale")
            : r.status === "done" && r.invalid_count === 0 ? <>
              <CircleCheck className="size-4 shrink-0 text-emerald-600 dark:text-emerald-400" aria-hidden />
              <span>
                <span className="font-medium text-foreground">{t(r.checked === 1 ? "quality.all_valid_one" : "quality.all_valid").replace("{n}", String(r.checked))}</span>
                {r.finished_at && ` · ${format(r.finished_at)}`}
                {r.truncated && ` ${t("quality.truncated")}`}
              </span>
            </>
            : <>
              {t("quality.checked").replace("{n}", String(r.checked))}
              {r.finished_at && ` · ${format(r.finished_at)}`}
              {r.truncated && ` ${t("quality.truncated")}`}
            </>}
        </p>
        <div className="ml-auto">{recheck}</div>
      </div>
      {r.status === "done" && !stale && r.invalid_count > 0 && (
        <BulkFix key={String(r.finished_at)} collection={collection} report={r} onDone={() => start.mutate()} fieldLabel={(k) => (k ? labels.get(k) ?? k : t("quality.root"))} issueText={issue} />
      )}
      {r.status === "done" && !stale && r.invalid_count > 0 && (
        <Card className="gap-0 overflow-hidden p-0">
          <div className="border-b px-4 py-3">
            <h2 className="font-heading text-base font-semibold">{t("quality.invalid").replace("{n}", String(r.invalid_count))}</h2>
            <p className="text-sm text-muted-foreground">
              {t("quality.invalid_desc")}
              {r.invalid_count > r.invalid.length && ` ${t("quality.first").replace("{n}", String(r.invalid.length))}`}
            </p>
          </div>
          <ul aria-label={t("quality.invalid").replace("{n}", String(r.invalid_count))} className="divide-y">
            {r.invalid.map((rec) => (
              <li key={rec.id} className="flex flex-wrap items-start gap-x-4 gap-y-1 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <Link href={`/records/${rec.id}`} className="block truncate text-sm font-medium hover:underline">{preview(rec.id, rec.data)}</Link>
                  <ul className="mt-1 space-y-0.5 text-xs">
                    {rec.issues.map((i, n) => (
                      <li key={n}><span className="font-medium">{field(i.path)}</span>: <span className="text-destructive">{issue(i.keyword, i.message)}</span></li>
                    ))}
                  </ul>
                </div>
                <span className="text-xs text-muted-foreground">{format(rec.updated_at)}</span>
              </li>
            ))}
          </ul>
        </Card>
      )}
    </div>
  );
}

export default function CollectionQualityPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.quality" description="quality.subtitle">
      {(collection) => roleAllows(collection.role, "editor")
        ? <Quality collection={collection} />
        : <EmptyState icon={ListChecks} title={t("error.forbidden")} description={t("quality.editors_only")} />}
    </CollectionPage>
  );
}
