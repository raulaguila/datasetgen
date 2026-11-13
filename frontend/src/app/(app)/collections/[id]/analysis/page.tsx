"use client";

import { useQuery } from "@tanstack/react-query";
import { FieldStats } from "@/components/collection-stats";
import { fieldsOf } from "@/components/field-builder";
import { useNumberFormat } from "@/components/record-groups";
import { RecordRules, parseWhere, whereText, type Where } from "@/components/record-rules";
import { ListSkeleton } from "@/components/skeletons";
import { records } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { useListParams } from "@/lib/hooks/use-list-params";
import { useI18n } from "@/lib/i18n/provider";
import { CollectionPage } from "../_parts";

/**
 * The collection's fields measured over its records, like a schema browser:
 * how full each one is, its numbers, dates or common values. Conditions narrow
 * the records, as on the list.
 */
function Analysis({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const num = useNumberFormat();
  const [params, set] = useListParams({ where: "" });
  const where = parseWhere(params.where);
  const text = whereText(where) || undefined;
  const rows = fieldsOf(collection.schema ?? null).filter((r) => !r.advanced && r.field.type !== "group");
  const total = useQuery({
    queryKey: ["records", collection.id, "aggregate", text, undefined, undefined],
    queryFn: ({ signal }) => records.aggregate({ collection_id: collection.id, where: text }, signal),
  });
  const n = total.data?.items[0]?.count ?? 0;
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <RecordRules fields={fieldsOf(collection.schema ?? null)} value={where} onChange={(w: Where) => set({ where: whereText(w) })} />
        <p className="ml-auto text-sm text-muted-foreground" role="status">
          {total.isPending ? "…" : t(text ? "analysis.records_matching" : "analysis.records").replace("{n}", num(n))}
        </p>
      </div>
      {total.isPending ? <ListSkeleton rows={4} /> : (
        <div className="grid gap-4 lg:grid-cols-2 2xl:grid-cols-3">
          {rows.map((r) => <FieldStats key={r.key} collection={collection} row={r} where={where} total={n} />)}
        </div>
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
