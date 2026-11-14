"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Pencil } from "lucide-react";
import { PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { recordTitle } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { RecordComments } from "@/components/record-comments";
import { RecordVersions, VersionCompare, useSnapshots } from "@/components/record-history";
import { RecordLinks } from "@/components/record-links";
import { Chip, RecordView, isChip } from "@/components/record-view";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { ApiError } from "@/lib/api/client";
import { collections, records } from "@/lib/api/endpoints";
import type { Collection, DataRecord } from "@/lib/api/types";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { useCrumb } from "@/lib/store/crumb";
import { neighbours, useSequenceStore } from "@/lib/store/sequence";
import { useUIStore } from "@/lib/store/ui";
import { cn } from "@/lib/utils";
import RecordEditPage from "./edit/page";

type S = Record<string, unknown>;

/** True when a key press belongs to a field or an open menu, not to the page's shortcuts. */
const typing = () => {
  const el = document.activeElement as HTMLElement | null;
  return !!el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA" || el.tagName === "SELECT" || el.isContentEditable || !!el.closest("[role=listbox],[role=dialog],[role=menu]"));
};

/** The record to read: its fields laid out, links, comments and versions to compare; Edit opens the form. */
function RecordReader({ collection, record }: { collection: Collection; record: DataRecord }) {
  const editable = roleAllows(collection.role, "editor");
  const { pending, items } = useSnapshots(record);
  const [picked, setPicked] = useState<number[]>([]);
  const pair = picked.map((v) => items.find((s) => s.version === v)).filter((s) => !!s).sort((a, b) => a.version - b.version);
  const schema = (collection.schema ?? {}) as S;
  const props = (schema.properties ?? {}) as Record<string, S>;
  const data = (record.data ?? {}) as S;
  const titleKey = Object.keys(props).find((k) => data[k] === recordTitle(collection.schema ?? null, record.data));
  const chips = Object.keys(props).filter((k) => k !== titleKey && isChip(props[k], data[k]));
  const skip = new Set([...(titleKey ? [titleKey] : []), ...chips]);
  const history = pending || items.length > 1;

  return (
    <div className="space-y-4">
      {chips.length > 0 && (
        <ul className="flex flex-wrap gap-1.5" aria-label={collection.name}>
          {chips.map((k) => (
            <li key={k}><Chip mono={props[k]["x-unique"] === true} strong={Array.isArray(props[k].enum)} title={typeof props[k].title === "string" ? props[k].title : k}>{String(data[k])}</Chip></li>
          ))}
        </ul>
      )}
      {/* With one version there is nothing to compare: the record gets the whole width. */}
      <div className={cn("grid items-start gap-4", history && "lg:grid-cols-[minmax(0,1fr)_17rem]")}>
        <div className="min-w-0 space-y-4">
          {pair.length === 2 && <VersionCompare collection={collection} from={pair[0]} to={pair[1]} onClose={() => setPicked([])} />}
          <RecordView collection={collection} record={record} skip={skip} />
          <RecordLinks recordId={record.id} />
          <RecordComments recordId={record.id} owner={roleAllows(collection.role, "owner")} />
        </div>
        {history && (
          <div className="lg:sticky lg:top-4">
            <RecordVersions record={record} snapshots={items} pending={pending} picked={picked} onPick={setPicked} editable={editable} />
          </div>
        )}
      </div>
    </div>
  );
}

function RecordViewPage({ id }: { id: string }) {
  const { t } = useI18n();
  const format = useFormatDate();
  const { loading } = useMe();
  const detail = useQuery({ queryKey: ["records", "detail", id], queryFn: () => records.get(id) });
  const record = detail.data;
  const collectionId = record?.collection_id;
  const parent = useQuery({ queryKey: ["collections", "detail", collectionId], queryFn: () => collections.get(collectionId!), enabled: !!collectionId });
  const collection = parent.data;
  const setCollection = useUIStore((st) => st.setCollection);
  useEffect(() => { if (collection) setCollection(collection.id); }, [collection, setCollection]);
  const editable = roleAllows(collection?.role, "editor");
  const title = record ? recordTitle(collection?.schema ?? null, record.data) ?? `${t("records.record")} ${record.id.slice(0, 8)}` : t("records.record");
  useCrumb(title, collection && { label: collection.name, href: `/collections/${collection.id}` });
  const failed = detail.error ?? parent.error;
  const sequence = useSequenceStore((st) => st.ids);
  const { prev, next, index } = neighbours(sequence, id);
  // j / k step through the list, e edits, as in mail apps; typing in a field is left alone.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || typing()) return;
      const target = e.key === "j" ? "next" : e.key === "k" ? "prev" : e.key === "e" ? "edit" : null;
      if (target) document.querySelector<HTMLAnchorElement>(`[data-step=${target}]`)?.click();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
  const step = (to: string | undefined, dir: "prev" | "next") => {
    const label = t(dir === "prev" ? "records.prev" : "records.next");
    const Icon = dir === "prev" ? ChevronLeft : ChevronRight;
    return to
      ? <Button variant="outline" size="icon" nativeButton={false} aria-label={label} title={`${label} (${dir === "prev" ? "k" : "j"})`} render={<Link href={`/records/${to}`} data-step={dir} />}><Icon className="size-4" /></Button>
      : <Button variant="outline" size="icon" disabled aria-label={label}><Icon className="size-4" /></Button>;
  };

  return (
    <>
      <PageHeader
        title={title}
        description={record && `${t("history.version").replace("{n}", String(record.version))} · ${t("common.updated_at")} ${format(record.updated_at)}`}
        action={record && (
          <>
            {index >= 0 && sequence.length > 1 && (
              <div className="flex items-center gap-2" role="group" aria-label={t("records.steps")}>
                {step(prev, "prev")}
                <span className="text-sm tabular-nums text-muted-foreground">
                  {/* A phone keeps "3 / 25" so the steps and Edit share one row. */}
                  <span className="max-sm:hidden">{t("records.step_of").replace("{i}", String(index + 1)).replace("{n}", String(sequence.length))}</span>
                  <span className="sm:hidden" title={t("records.step_of").replace("{i}", String(index + 1)).replace("{n}", String(sequence.length))}>{index + 1} / {sequence.length}</span>
                </span>
                {step(next, "next")}
              </div>
            )}
            {editable && (
              <Button nativeButton={false} title={`${t("common.edit")} (e)`} render={<Link href={`/records/${record.id}/edit`} data-step="edit" />}>
                <Pencil className="size-4" />{t("common.edit")}
              </Button>
            )}
          </>
        )}
      />
      <PageBody>
        {loading || detail.isPending || (!!collectionId && parent.isPending) ? (
          <ListSkeleton rows={3} />
        ) : failed instanceof ApiError && failed.code === "NOT_FOUND" ? (
          <EmptyState title={t("error.not_found")} description={t("error.record_gone")} action={<Button variant="outline" nativeButton={false} render={<Link href="/collections" />}>{t("common.back_to_list")}</Button>} />
        ) : failed || !collection || !record ? (
          <QueryErrorState error={failed} onRetry={() => { void detail.refetch(); void parent.refetch(); }} />
        ) : (
          <RecordReader key={record.id} collection={collection} record={record} />
        )}
      </PageBody>
    </>
  );
}

/** /records/{id} reads the record; /records/new is the form for a new one. */
export default function RecordPage() {
  const { id } = useParams<{ id: string }>();
  return id === "new" ? <RecordEditPage /> : <RecordViewPage id={id} />;
}
