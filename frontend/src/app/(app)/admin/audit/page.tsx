"use client";

import { PageBody, PageHeader } from "@/components/bits";
import { AuditLog } from "@/components/audit-log";
import { useI18n } from "@/lib/i18n/provider";

export default function AuditPage() {
  const { t } = useI18n();
  return (
    <>
      <PageHeader title={t("nav.audit")} description={t("audit.subtitle")} />
      <PageBody><AuditLog /></PageBody>
    </>
  );
}
