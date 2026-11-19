import type { AuditRecord } from "@/lib/api/types";
import { collectionByID, recordByID } from "@/lib/paths";

/** The record page an audit entry is about; none for deletions or targetless actions. */
export function targetHref(r: AuditRecord): string | undefined {
  if (!r.target_id || r.target_gone) return undefined;
  if (r.action.startsWith("user.") || r.action.startsWith("admin.")) return `/admin/users/${r.target_id}`;
  if (r.action === "record.import") return collectionByID(r.target_id); // its target is the collection
  if (r.action.startsWith("record.") && r.action !== "record.delete") return recordByID(r.target_id); // the page there finds its collection
  if (r.action.startsWith("collection.") && r.action !== "collection.delete") return collectionByID(r.target_id);
}
