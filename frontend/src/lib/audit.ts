import type { AuditRecord } from "@/lib/api/types";

/** The record page an audit entry is about; none for deletions or targetless actions. */
export function targetHref(r: AuditRecord): string | undefined {
  if (!r.target_id || r.target_gone) return undefined;
  if (r.action.startsWith("user.") || r.action.startsWith("admin.")) return `/admin/users/${r.target_id}`;
  if (r.action.startsWith("role.")) return `/admin/roles/${r.target_id}`;
  if (r.action === "record.import") return `/collections/${r.target_id}`; // its target is the collection
  if (r.action.startsWith("record.") && r.action !== "record.delete") return `/records/${r.target_id}`; // the record page finds its collection
  if (r.action.startsWith("collection.") && r.action !== "collection.delete") return `/collections/${r.target_id}`;
}
