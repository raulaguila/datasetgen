import type { AuditRecord } from "@/lib/api/types";
import { collectionPath, recordByID } from "@/lib/paths";

/** The record page an audit entry is about; none for deletions or targetless actions. */
export function targetHref(r: AuditRecord): string | undefined {
  if (!r.target_id || r.target_gone) return undefined;
  if (r.action.startsWith("user.") || r.action.startsWith("admin.")) return `/admin/users/${r.target_id}`;
  // A collection target (record.import's too) comes with its address.
  if (r.target_owner && r.target_slug) return collectionPath({ owner: r.target_owner, slug: r.target_slug });
  if (r.action.startsWith("record.") && r.action !== "record.delete") return recordByID(r.target_id); // the record's permalink
}
