"use client";

import { useSyncExternalStore } from "react";
import { fieldsOf } from "@/components/field-builder";
import { CodeBlock } from "@/components/secret";
import type { Collection } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";

/** The web address, known only in the browser; the server render shows the path alone. */
export function useOrigin(): string {
  return useSyncExternalStore(() => () => {}, () => window.location.origin, () => "");
}

/** How a program reads and writes a collection over REST, with examples built from its fields. */
export function RestExamples({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  // The web address answers the API too: the proxy passes API keys through.
  const base = `${useOrigin()}/api/proxy`;
  const example = Object.fromEntries(fieldsOf(collection.schema ?? null).filter((r) => !r.advanced).slice(0, 3).map((r) => [
    r.key,
    r.field.type === "number" || r.field.type === "integer" ? 1 : r.field.type === "boolean" ? true : r.field.type === "strings" ? ["a"] : r.field.type === "enum" ? r.field.options.split(",")[0]?.trim() ?? "" : r.field.type === "date" ? "2026-01-31" : "…",
  ]));
  const auth = `-H "Authorization: Bearer $DATASET_KEY"`;
  return (
    <div className="space-y-4 text-sm">
      <p>{t("integrations.docs_base")} <code className="font-mono text-xs">{base}</code></p>
      <div className="space-y-1.5">
        <h3 className="font-medium">{t("integrations.docs_list")}</h3>
        <CodeBlock>{`curl ${auth} \\\n  "${base}/v1/records?collection_id=${collection.id}&page=1&limit=50&sort=updated_at&order=desc"`}</CodeBlock>
        <p className="text-xs text-muted-foreground">{t("integrations.docs_list_hint")}</p>
      </div>
      <div className="space-y-1.5">
        <h3 className="font-medium">{t("integrations.docs_create")}</h3>
        <CodeBlock>{`curl ${auth} -H "Content-Type: application/json" \\\n  -d '${JSON.stringify({ collection_id: collection.id, data: example })}' \\\n  "${base}/v1/records"`}</CodeBlock>
      </div>
      <div className="space-y-1.5">
        <h3 className="font-medium">{t("integrations.docs_update")}</h3>
        <CodeBlock>{`curl ${auth} -X PATCH -H "Content-Type: application/json" \\\n  -d '${JSON.stringify({ data: example, version: 1 })}' \\\n  "${base}/v1/records/<record-id>"`}</CodeBlock>
        <p className="text-xs text-muted-foreground">{t("integrations.docs_update_hint")}</p>
      </div>
      <div className="space-y-1.5">
        <h3 className="font-medium">{t("integrations.docs_delete")}</h3>
        <CodeBlock>{`curl ${auth} -H "Content-Type: application/json" \\\n  -d '{"ids":["<record-id>"]}' "${base}/v1/records/delete"`}</CodeBlock>
      </div>
    </div>
  );
}
