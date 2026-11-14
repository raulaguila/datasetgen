"use client";

import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowRight, GitCompare, History, RotateCcw } from "lucide-react";
import { toast } from "sonner";
import { useFormatDate } from "@/components/bits";
import { ConfirmDialog, ConfirmationError } from "@/components/confirm-dialog";
import { Chip, Value, isEmpty } from "@/components/record-view";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { ApiError } from "@/lib/api/client";
import { records } from "@/lib/api/endpoints";
import type { Collection, DataRecord } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";
import { plural } from "@/lib/i18n/plural";
import { cn } from "@/lib/utils";

type S = Record<string, unknown>;
const obj = (v: unknown): S => (v && typeof v === "object" && !Array.isArray(v) ? (v as S) : {});
const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

export type Snapshot = { version: number; saved_at: string; data: unknown };

/**
 * Top-level fields whose value differs between two versions. A list on both
 * sides also says which items left and which arrived, so a long list reads as
 * its few changes.
 */
export type Change = { key: string; before: unknown; after: unknown; removed?: unknown[]; added?: unknown[] };

export function changes(before: unknown, after: unknown): Change[] {
  const a = obj(before);
  const b = obj(after);
  return [...new Set([...Object.keys(a), ...Object.keys(b)])]
    .filter((k) => !same(a[k], b[k]))
    .map((key) => {
      const [x, y] = [a[key], b[key]];
      if (!(Array.isArray(x) || x === undefined) || !(Array.isArray(y) || y === undefined)) return { key, before: x, after: y };
      const xs = (x as unknown[] | undefined) ?? [];
      const ys = (y as unknown[] | undefined) ?? [];
      return { key, before: x, after: y, removed: xs.filter((i) => !ys.some((j) => same(i, j))), added: ys.filter((j) => !xs.some((i) => same(i, j))) };
    });
}

/** The record's versions, newest first, the current one on top. */
export function useSnapshots(record: DataRecord) {
  const list = useQuery({
    queryKey: ["records", "versions", record.id, record.version],
    queryFn: ({ signal }) => records.versions(record.id, signal),
  });
  const current: Snapshot = { version: record.version, saved_at: record.updated_at, data: record.data };
  return { pending: list.isPending, items: [current, ...(list.data?.items ?? []).filter((v) => v.version !== record.version)] };
}

/** One item of a list, short: an object by its first text value. */
function itemText(v: unknown): string {
  if (v && typeof v === "object" && !Array.isArray(v)) {
    const first = Object.values(v as S).find((x) => typeof x === "string" && x.trim());
    return typeof first === "string" ? first : JSON.stringify(v);
  }
  return String(v);
}

/** Two versions side by side, field by field: what left struck through, what arrived after it. */
export function VersionCompare({ collection, from, to, onClose }: { collection: Collection; from: Snapshot; to: Snapshot; onClose: () => void }) {
  const { t } = useI18n();
  const props = obj((collection.schema as S | null)?.properties);
  const diff = changes(from.data, to.data);
  const label = (k: string) => { const s = obj(props[k]); return typeof s.title === "string" && s.title ? s.title : k; };
  // The pick happens in the side list; bring the result into view where it opens.
  const box = useRef<HTMLElement>(null);
  useEffect(() => { box.current?.scrollIntoView({ behavior: "smooth", block: "start" }); }, [from.version, to.version]);
  return (
    <section ref={box} aria-labelledby="compare-title" className="scroll-mt-4 rounded-xl border border-primary/30 bg-card p-4">
      <div className="flex flex-wrap items-center gap-2">
        <GitCompare className="size-4 text-primary" aria-hidden />
        <h2 id="compare-title" className="text-sm font-semibold">
          {t("history.version").replace("{n}", String(from.version))} <ArrowRight className="inline size-3.5" aria-label={t("records.compare_to")} /> {t("history.version").replace("{n}", String(to.version))}
        </h2>
        <Chip>{diff.length ? plural(t("records.changed_fields"), diff.length) : t("records.no_changes")}</Chip>
        <Button size="sm" variant="ghost" className="ml-auto" onClick={onClose}>{t("common.close")}</Button>
      </div>
      {diff.length > 0 && (
        <dl className="mt-3 grid gap-x-4 gap-y-3 text-sm sm:grid-cols-[minmax(7rem,auto)_minmax(0,1fr)]">
          {diff.map((c) => (
            <div key={c.key} className="contents">
              <dt className="font-medium text-muted-foreground">{label(c.key)}</dt>
              <dd className="min-w-0">
                {c.added || c.removed ? (
                  <ul className="flex flex-wrap gap-1.5">
                    {c.removed?.map((v, i) => <li key={`r${i}`} className="rounded-md bg-destructive/10 px-1.5 py-0.5 text-destructive line-through decoration-destructive/60">− {itemText(v)}</li>)}
                    {c.added?.map((v, i) => <li key={`a${i}`} className="rounded-md bg-success/10 px-1.5 py-0.5 text-success">+ {itemText(v)}</li>)}
                  </ul>
                ) : (
                  <div className="space-y-1.5">
                    <div className={cn("rounded-md bg-destructive/10 px-2 py-1 line-through decoration-destructive/60", isEmpty(c.before) && "no-underline")}>
                      {isEmpty(c.before) ? <span className="text-muted-foreground">{t("history.absent")}</span> : <Value schema={obj(props[c.key])} value={c.before} />}
                    </div>
                    <div className="rounded-md bg-success/10 px-2 py-1">
                      {isEmpty(c.after) ? <span className="text-muted-foreground">{t("history.absent")}</span> : <Value schema={obj(props[c.key])} value={c.after} />}
                    </div>
                  </div>
                )}
              </dd>
            </div>
          ))}
        </dl>
      )}
    </section>
  );
}

/** The versions list: pick two to compare; editors restore an earlier one. */
export function RecordVersions({ record, snapshots, pending, picked, onPick, editable }: {
  record: DataRecord;
  snapshots: Snapshot[];
  pending: boolean;
  picked: number[];
  onPick: (versions: number[]) => void;
  editable: boolean;
}) {
  const { t } = useI18n();
  const format = useFormatDate();
  const client = useQueryClient();
  const [restoring, setRestoring] = useState<Snapshot | null>(null);
  // Ticking a third drops the oldest pick, so two stay chosen.
  const toggle = (v: number) => onPick(picked.includes(v) ? picked.filter((x) => x !== v) : [...picked, v].slice(-2));

  return (
    <section aria-labelledby="versions-title" className="rounded-xl border bg-card p-4">
      <h2 id="versions-title" className="flex items-center gap-2 text-sm font-semibold"><History className="size-4" aria-hidden />{t("history.title")}</h2>
      <p className="mt-0.5 text-xs text-muted-foreground">{t("records.compare_hint")}</p>
      {pending ? (
        <div className="mt-3"><ListSkeleton rows={2} /></div>
      ) : snapshots.length < 2 ? (
        <p className="mt-3 text-sm text-muted-foreground">{t("history.empty")}</p>
      ) : (
        <ol className="mt-3 divide-y" aria-label={t("history.title")}>
          {snapshots.map((v) => {
            const id = `ver-${v.version}`;
            const current = v.version === record.version;
            return (
              <li key={v.version} className="flex items-center gap-2 py-2">
                <Checkbox id={id} checked={picked.includes(v.version)} onCheckedChange={() => toggle(v.version)} />
                <label htmlFor={id} className="min-w-0 flex-1 cursor-pointer">
                  <span className="block text-sm font-medium">
                    {t("history.version").replace("{n}", String(v.version))}
                    {current && <span className="ml-1.5 text-xs font-normal text-primary">{t("records.current")}</span>}
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">{format(v.saved_at)}</span>
                </label>
                {editable && !current && (
                  <Button size="icon-sm" variant="ghost" aria-label={`${t("history.restore")} ${v.version}`} title={t("history.restore")} onClick={() => setRestoring(v)}>
                    <RotateCcw className="size-3.5" />
                  </Button>
                )}
              </li>
            );
          })}
        </ol>
      )}
      <ConfirmDialog
        open={!!restoring}
        onOpenChange={(o) => !o && setRestoring(null)}
        title={t("history.restore")}
        description={t("history.restore_confirm").replace("{n}", String(restoring?.version ?? ""))}
        confirmLabel={t("history.restore")}
        onConfirm={async () => {
          if (!restoring) return;
          try {
            const saved = await records.restore(record.id, restoring.version, record.version);
            client.setQueryData(["records", "detail", saved.id], saved);
            void client.invalidateQueries({ queryKey: ["records", record.collection_id] });
            toast.success(t("history.restored"));
          } catch (err) {
            // Old data that breaks today's fields cannot come back as it was.
            if (err instanceof ApiError && err.code === "VALIDATION_FAILED") throw new ConfirmationError(t("history.invalid"));
            throw err;
          }
          onPick([]);
          setRestoring(null);
        }}
      />
    </section>
  );
}
