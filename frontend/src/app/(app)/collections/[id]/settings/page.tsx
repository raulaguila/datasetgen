"use client";

import { useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { DangerZone } from "@/components/bits";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { collections } from "@/lib/api/endpoints";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";
import { useUIStore } from "@/lib/store/ui";
import { CollectionForm, CollectionPage } from "../_parts";

export default function CollectionSettingsPage() {
  const { t } = useI18n();
  const router = useRouter();
  const client = useQueryClient();
  const { id } = useParams<{ id: string }>();
  const setCollection = useUIStore((s) => s.setCollection);
  const [confirmDelete, setConfirmDelete] = useState(false);

  return (
    <CollectionPage section="collections.general_settings">
      {(collection) => (
        <div className="space-y-6">
          <CollectionForm key={collection.id} collection={collection} parts={["details"]} />
          {roleAllows(collection.role, "owner") && (
            <DangerZone description={t("collections.delete_hint")} label={t("collections.delete")} onClick={() => setConfirmDelete(true)} />
          )}
          <ConfirmDialog
            open={confirmDelete}
            onOpenChange={setConfirmDelete}
            title={t("collections.delete")}
            description={`${collection.name}. ${t("collections.delete_hint")}`}
            destructive
            confirmLabel={t("common.delete")}
            onConfirm={async () => {
              await collections.remove([id]); // the dialog shows a refusal
              setCollection(null);
              router.push("/collections");
              client.removeQueries({ queryKey: ["collections", "detail", id] });
              void client.invalidateQueries({ queryKey: ["collections"] });
            }}
          />
        </div>
      )}
    </CollectionPage>
  );
}
