"use client";

import { BUCKETS, FieldStats, ViewConditions, barFields, isDate, isNumeric, useStats, isPlain } from "@/components/collection-stats";
import { fieldsOf } from "@/components/field-builder";
import { useNumberFormat } from "@/components/record-groups";
import { RecordRules, parseWhere, whereText, type Where } from "@/components/record-rules";
import { ListSkeleton } from "@/components/skeletons";
import { QueryErrorState } from "@/components/query-error-state";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import type { Collection, StatsBucket } from "@/lib/api/types";
import { useListParams } from "@/lib/hooks/use-list-params";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { CollectionPage } from "../_parts";

/**
 * The collection's fields measured over its records, like a schema browser:
 * how full each one is, its numbers, dates over time or common values, all in
 * one request. Conditions narrow the records, as on the list.
 */
function Analysis({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const num = useNumberFormat();
  const [params, set] = useListParams({ where: "", bucket: "month" });
  const where = parseWhere(params.where);
  const text = whereText(where);
  const bucket = (BUCKETS.includes(params.bucket as StatsBucket) ? params.bucket : "month") as StatsBucket;
  const rows = fieldsOf(collection.schema ?? null).filter((r) => !r.advanced && r.field.type !== "group");
  const dates = rows.filter(isDate);
  const st = useStats(collection.id, where, { filled: rows, numbers: rows.filter(isNumeric), dates, values: barFields(rows), bucket });
  const n = st.data?.total ?? 0;
  const buckets = Object.fromEntries(BUCKETS.map((b) => [b, t(`analysis.bucket.${b}` as MessageKey)]));
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <RecordRules fields={fieldsOf(collection.schema ?? null)} value={where} onChange={(w: Where) => set({ where: whereText(w) })} />
        <ViewConditions collectionId={collection.id} onPick={(w) => set({ where: whereText(w) })} />
        {dates.length > 0 && (
          <Select value={bucket} items={buckets} onValueChange={(v) => set({ bucket: String(v) })}>
            <SelectTrigger size="sm" aria-label={t("analysis.bucket")}><span className="text-muted-foreground">{t("analysis.bucket")}</span><SelectValue /></SelectTrigger>
            <SelectContent>{BUCKETS.map((b) => <SelectItem key={b} value={b}>{buckets[b]}</SelectItem>)}</SelectContent>
          </Select>
        )}
        <p className="ml-auto text-sm text-muted-foreground" role="status">
          {st.isPending ? "…" : t(text ? "analysis.records_matching" : "analysis.records").replace("{n}", num(n))}
        </p>
      </div>
      {st.isError ? <QueryErrorState error={st.error} onRetry={() => void st.refetch()} /> : st.isPending ? <ListSkeleton rows={4} /> : (
        <>
          {/* Fields with figures or bars first; those with only a fill rate share smaller cards after them. */}
          <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
            {rows.filter((r) => !isPlain(st.data.fields[r.key])).map((r) => <FieldStats key={r.key} row={r} profile={st.data.fields[r.key]} total={n} bucket={bucket} />)}
          </div>
          <div className="grid items-start gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
            {rows.filter((r) => isPlain(st.data.fields[r.key])).map((r) => <FieldStats key={r.key} row={r} profile={st.data.fields[r.key]} total={n} bucket={bucket} />)}
          </div>
        </>
      )}
    </div>
  );
}

export default function CollectionAnalysisPage() {
  return (
    <CollectionPage section="collections.tab.analysis" description="analysis.subtitle">
      {(collection) => <Analysis collection={collection} />}
    </CollectionPage>
  );
}
