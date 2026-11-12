"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { CircleCheck, ListChecks, RefreshCw } from "lucide-react";
import { useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { fieldsOf } from "@/components/field-builder";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { collections } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

/** Records that a change of fields left out of format, each with the rules it breaks. */
function Quality({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const format = useFormatDate();
  // Keyed by version: a schema change runs the check again.
  const q = useQuery({ queryKey: ["collections", "quality", collection.id, collection.version], queryFn: ({ signal }) => collections.quality(collection.id, signal) });
  // Issues name the field by its key; show its label, as the form does.
  const labels = new Map(fieldsOf(collection.schema ?? null).map((r) => [r.key, r.field.name]));
  const field = (path: string) => {
    const key = (path.split("/")[1] ?? "").replace(/~1/g, "/").replace(/~0/g, "~");
    return key ? labels.get(key) ?? key : t("quality.root");
  };
  const issue = (keyword: string, message: string) => {
    const text = t(`quality.issue.${keyword}` as MessageKey);
    return text.startsWith("quality.issue.") ? message : text;
  };
  const preview = (id: string, data: Record<string, unknown>) => {
    const first = [...labels.keys()].map((k) => data?.[k]).find((v) => typeof v === "string" && v);
    return typeof first === "string" ? first : `${t("records.record")} ${id.slice(0, 8)}`;
  };

  if (q.isPending) return <ListSkeleton rows={3} />;
  if (q.isError) return <QueryErrorState error={q.error} onRetry={() => void q.refetch()} />;
  const r = q.data;
  const recheck = (
    <Button variant="outline" disabled={q.isFetching} onClick={() => void q.refetch()}>
      <RefreshCw className={q.isFetching ? "size-4 animate-spin" : "size-4"} />{t("quality.recheck")}
    </Button>
  );
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <p className="text-sm text-muted-foreground">
          {t("quality.checked").replace("{n}", String(r.checked))}
          {r.truncated && ` ${t("quality.truncated")}`}
        </p>
        <div className="ml-auto">{recheck}</div>
      </div>
      {r.invalid_count === 0 ? (
        <EmptyState icon={CircleCheck} title={t("quality.all_valid")} />
      ) : (
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
    <CollectionPage section="collections.tab.quality">
      {(collection) => roleAllows(collection.role, "editor")
        ? <Quality collection={collection} />
        : <EmptyState icon={ListChecks} title={t("error.forbidden")} />}
    </CollectionPage>
  );
}
