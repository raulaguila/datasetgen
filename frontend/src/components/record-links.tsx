"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { FormSection, FormSections } from "@/components/bits";
import { fieldsOf, recordTitle } from "@/components/field-builder";
import { records } from "@/lib/api/endpoints";
import { useI18n } from "@/lib/i18n/provider";

/** The records of other collections pointing at this one (its backlinks). */
export function useRecordLinks(recordId: string | undefined) {
  return useQuery({ queryKey: ["records", "links", recordId], queryFn: () => records.links(recordId!), enabled: !!recordId });
}

export function RecordLinks({ recordId }: { recordId: string }) {
  const { t } = useI18n();
  const links = useRecordLinks(recordId);
  const items = links.data?.items ?? [];
  if (!items.length) return null;
  return (
    <FormSections>
      <FormSection title={t("links.title")} description={t("links.desc")}>
        <div className="space-y-4">
          {items.map((l) => {
            const field = fieldsOf(l.schema).find((f) => f.key === l.field)?.field.name ?? l.field;
            return (
              <section key={`${l.collection_id}:${l.field}`} aria-label={`${l.collection_name}: ${field}`} className="space-y-1.5">
                <h3 className="text-sm font-medium">
                  <Link href={`/collections/${l.collection_id}`} className="hover:underline">{t("links.via").replace("{collection}", l.collection_name).replace("{field}", field)}</Link>
                  <span className="ml-2 text-xs text-muted-foreground">{l.total}</span>
                </h3>
                <ul className="flex flex-wrap gap-1.5">
                  {l.records.map((r) => (
                    <li key={r.id}>
                      <Link href={`/records/${r.id}`} className="block max-w-64 truncate rounded-full border bg-muted/40 px-2.5 py-0.5 text-sm hover:bg-muted">
                        {recordTitle(l.schema, r.data) ?? r.id.slice(0, 8)}
                      </Link>
                    </li>
                  ))}
                  {l.total > l.records.length && <li className="px-1 py-0.5 text-sm text-muted-foreground">{t("links.more").replace("{n}", String(l.total - l.records.length))}</li>}
                </ul>
              </section>
            );
          })}
        </div>
      </FormSection>
    </FormSections>
  );
}
