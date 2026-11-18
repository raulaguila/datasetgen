"use client";

import { useState } from "react";
import { ChevronDown, Download, FileUp } from "lucide-react";
import { toast } from "sonner";
import { RecordImportDialog } from "@/components/record-import-dialog";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { records } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";
import { roleAllows } from "@/lib/permissions";

const FORMATS = [
  { format: "jsonl", valid: false, key: "export.jsonl" },
  { format: "csv", valid: false, key: "export.csv" },
  { format: "jsonl", valid: true, key: "export.jsonl_valid" },
  { format: "csv", valid: true, key: "export.csv_valid" },
] as const;

/** Export for everyone who can read the collection; editors also get import in the same menu. */
export function ExportMenu({ collection }: { collection: Collection }) {
  const { t } = useI18n();
  const [importing, setImporting] = useState(false);
  const canImport = roleAllows(collection.role, "editor");
  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger render={<Button variant="outline" />}>
          <Download className="size-4" />{t("export.title")}<ChevronDown className="size-3.5 opacity-60" aria-hidden />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="min-w-56">
          {FORMATS.map((f) => (
            <DropdownMenuItem key={f.key} onClick={() => void records.export(collection.id, f.format, f.valid).catch((err) => toast.error(errorMessage(err, "common.save_failed", t)))}>
              <Download className="size-4" />{t(f.key)}
            </DropdownMenuItem>
          ))}
          {canImport && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => setImporting(true)}><FileUp className="size-4" />{t("import.title")}</DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
      {canImport && <RecordImportDialog collectionId={collection.id} schema={collection.schema} open={importing} onOpenChange={setImporting} />}
    </>
  );
}
