"use client";

import { PageBody, PageHeader } from "@/components/bits";
import { ListSkeleton } from "@/components/skeletons";
import { useMe } from "@/lib/context/me";
import { useI18n } from "@/lib/i18n/provider";
import { useCrumb } from "@/lib/store/crumb";
import { NewCollectionForm } from "./_new";

export default function NewCollectionPage() {
  const { t } = useI18n();
  const { loading } = useMe();
  useCrumb(t("collections.new"));
  return (
    <>
      <PageHeader title={t("collections.new")} description={t("collections.new_desc")} />
      <PageBody>{loading ? <ListSkeleton rows={3} /> : <NewCollectionForm />}</PageBody>
    </>
  );
}
