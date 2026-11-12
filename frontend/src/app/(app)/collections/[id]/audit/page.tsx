"use client";

import { ScrollText } from "lucide-react";
import { AuditLog } from "@/components/audit-log";
import { EmptyState } from "@/components/empty-state";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { CollectionPage } from "../_parts";

export default function CollectionAuditPage() {
  const { t } = useI18n();
  return (
    <CollectionPage section="collections.tab.audit">
      {(collection) => roleAllows(collection.role, "owner")
        ? <AuditLog collectionId={collection.id} />
        : <EmptyState icon={ScrollText} title={t("error.forbidden")} />}
    </CollectionPage>
  );
}
