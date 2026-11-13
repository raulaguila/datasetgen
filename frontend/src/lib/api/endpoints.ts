// One function per API route the screens use. Paths match the backend Swagger.
import { apiDownload, apiFetch } from "@/lib/api/client";
import type {
  Automation,
  AutomationInput,
  Widget,
  PublicLink,
  WebhookDelivery,
  RecordComment, RecordLinks,
  ShareLink,
  Action,
  AuditRecord,
  InviteResult,
  MeResponse,
  Member,
  Notification,
  Paged,
  Preferences,
  Product,
  Collection,
  CollectionMember,
  CollectionStats,
  MemberRole,
  QualityReport,
  DataRecord,
  ImportResult,
  RecordFix,
  RecordVersion,
  Resource,
  Role,
  Session,
  SortOrder,
  User,
  UserCounts,
  UserDetails,
  UserStatus,
  ApiKey,
  Webhook,
  WebhookEvent,
  TrashedRecord,
  TrashedCollection,
  SchemaVersion,
  SavedView,
  FileRef,
  RecordGroup,
} from "@/lib/api/types";

function query(params: Record<string, string | number | boolean | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") q.set(k, String(v));
  const s = q.toString();
  return s ? `?${s}` : "";
}

const id = (v: string) => encodeURIComponent(v);

export const account = {
  me: () => apiFetch<MeResponse>("/v1/me"),
  rename: (name: string) => apiFetch<MeResponse>("/v1/me", { method: "PATCH", json: { name } }),
  changePassword: (current_password: string, new_password: string) =>
    apiFetch<void>("/v1/me/password", { method: "POST", json: { current_password, new_password } }),
  sessions: () => apiFetch<{ items: Session[] }>("/v1/me/sessions"),
  revokeSession: (sessionId: string) => apiFetch<void>(`/v1/me/sessions/${id(sessionId)}`, { method: "DELETE" }),
  revokeOtherSessions: () => apiFetch<void>("/v1/me/sessions/others", { method: "DELETE" }),
  setAvatar: (image: Blob) => apiFetch<void>("/v1/me/avatar", { method: "PUT", blob: image }),
  removeAvatar: () => apiFetch<void>("/v1/me/avatar", { method: "DELETE" }),
  /** The picture's address; avatar_at in the query makes a new picture a new URL. */
  avatarUrl: (avatarAt: string) => `/api/proxy/v1/me/avatar?v=${Date.parse(avatarAt)}`,
  savePreferences: (p: Preferences) => apiFetch<Preferences>("/v1/me/preferences", { method: "PUT", json: p }),
};

export const notifications = {
  list: (page: number, limit: number, signal?: AbortSignal) => apiFetch<Paged<Notification>>(`/v1/me/notifications${query({ page, limit })}`, { signal }),
  unread: () => apiFetch<{ unread: number }>("/v1/me/notifications/count"),
  /** Marks the listed notifications, or all of them when ids is left out. */
  markRead: (ids?: string[]) => apiFetch<void>("/v1/me/notifications/read", { method: "PUT", json: ids && { ids } }),
};

export const users = {
  list: (p: { page: number; limit: number; search?: string; status?: UserStatus | ""; role_id?: string; sort?: string; order?: SortOrder }, signal?: AbortSignal) =>
    apiFetch<Paged<User>>(`/v1/users${query(p)}`, { signal }),
  counts: () => apiFetch<UserCounts>("/v1/users/counts"),
  invite: (name: string, email: string) => apiFetch<InviteResult>("/v1/users", { method: "POST", json: { name, email } }),
  /** A new activation email for a pending account; the old link stops working. */
  reinvite: (userId: string) => apiFetch<InviteResult>(`/v1/users/${id(userId)}/invite`, { method: "POST" }),
  /** A new activation link for a pending account, without email; the old link stops working. */
  activationLink: (userId: string) =>
    apiFetch<{ user_id: string; status: UserStatus; activation_token: string; expires_at: string }>(`/v1/users/${id(userId)}/activation-link`, { method: "POST" }),
  revokeSessions: (userId: string) => apiFetch<void>(`/v1/users/${id(userId)}/sessions`, { method: "DELETE" }),
  get: (userId: string) => apiFetch<UserDetails>(`/v1/users/${id(userId)}`),
  disable: (ids: string[]) => apiFetch<void>("/v1/users/disable", { method: "POST", json: { ids } }),
  enable: (ids: string[]) => apiFetch<void>("/v1/users/enable", { method: "POST", json: { ids } }),
  unlock: (ids: string[]) => apiFetch<void>("/v1/users/unlock", { method: "POST", json: { ids } }),
  setRoles: (userId: string, roleIds: string[]) => apiFetch<void>(`/v1/users/${id(userId)}/roles`, { method: "PUT", json: { role_ids: roleIds } }),
};

export const roles = {
  list: (p: { page: number; limit: number; search?: string; sort?: string; order?: SortOrder }, signal?: AbortSignal) => apiFetch<Paged<Role>>(`/v1/roles${query(p)}`, { signal }),
  get: (roleId: string) => apiFetch<Role>(`/v1/roles/${id(roleId)}`),
  create: (name: string) => apiFetch<Role>("/v1/roles", { method: "POST", json: { name } }),
  update: (roleId: string, patch: { name?: string; enabled?: boolean; is_default?: boolean }) => apiFetch<Role>(`/v1/roles/${id(roleId)}`, { method: "PATCH", json: patch }),
  remove: (ids: string[]) => apiFetch<void>("/v1/roles/delete", { method: "POST", json: { ids } }),
  setPermissions: (roleId: string, permissions: Record<string, Action[]>) =>
    apiFetch<Role>(`/v1/roles/${id(roleId)}/permissions`, { method: "PUT", json: { permissions } }),
  members: (roleId: string, page: number, limit: number, search?: string, signal?: AbortSignal) =>
    apiFetch<Paged<Member>>(`/v1/roles/${id(roleId)}/members${query({ page, limit, search })}`, { signal }),
  addMember: (roleId: string, userId: string) => apiFetch<void>(`/v1/roles/${id(roleId)}/members/${id(userId)}`, { method: "PUT" }),
  removeMember: (roleId: string, userId: string) => apiFetch<void>(`/v1/roles/${id(roleId)}/members/${id(userId)}`, { method: "DELETE" }),
  resources: () => apiFetch<{ items: Resource[] }>("/v1/permissions/resources"),
};

export interface ProductInput {
  name: string;
  price: string;
  enabled: boolean;
}

export interface CollectionInput {
  name: string;
  description: string;
  icon?: string;
  schema: unknown;
}

export const products = {
  list: (p: { page: number; limit: number; search?: string; sort?: string; order?: SortOrder; enabled?: boolean }, signal?: AbortSignal) =>
    apiFetch<Paged<Product>>(`/v1/products${query(p)}`, { signal }),
  get: (productId: string) => apiFetch<Product>(`/v1/products/${id(productId)}`),
  create: (input: ProductInput) => apiFetch<Product>("/v1/products", { method: "POST", json: input }),
  update: (productId: string, input: Partial<ProductInput> & { version?: number }) => apiFetch<Product>(`/v1/products/${id(productId)}`, { method: "PATCH", json: input }),
  remove: (ids: string[]) => apiFetch<void>("/v1/products/delete", { method: "POST", json: { ids } }),
};

export const collections = {
  /** details adds member_count, field_count and last_activity, which cost a pass over members and records. */
  list: (p: { page: number; limit: number; search?: string; sort?: string; order?: SortOrder; details?: boolean }, signal?: AbortSignal) =>
    apiFetch<Paged<Collection>>(`/v1/collections${query(p)}`, { signal }),
  get: (collectionId: string) => apiFetch<Collection>(`/v1/collections/${id(collectionId)}`),
  create: (input: CollectionInput) => apiFetch<Collection>("/v1/collections", { method: "POST", json: input }),
  update: (collectionId: string, input: Partial<CollectionInput> & { version?: number }) => apiFetch<Collection>(`/v1/collections/${id(collectionId)}`, { method: "PATCH", json: input }),
  remove: (ids: string[]) => apiFetch<void>("/v1/collections/delete", { method: "POST", json: { ids } }),
  members: (collectionId: string) => apiFetch<{ items: CollectionMember[] }>(`/v1/collections/${id(collectionId)}/members`),
  /** An email without an account is invited (name required) and joins at once. */
  addMember: (collectionId: string, input: { email: string; name?: string; role: MemberRole }) =>
    apiFetch<{ member: CollectionMember; invited: boolean; activation_token?: string }>(`/v1/collections/${id(collectionId)}/members`, { method: "POST", json: input }),
  setMemberRole: (collectionId: string, userId: string, role: MemberRole) =>
    apiFetch<void>(`/v1/collections/${id(collectionId)}/members/${id(userId)}`, { method: "PATCH", json: { role } }),
  /** The collection's own audit log (owners). */
  audit: (collectionId: string, p: { page: number; limit: number; action?: string; user_id?: string; from?: string; to?: string }, signal?: AbortSignal) =>
    apiFetch<Paged<AuditRecord>>(`/v1/collections/${id(collectionId)}/audit${query(p)}`, { signal }),
  /** Checks every record against the current schema (editors and owners). */
  quality: (collectionId: string, signal?: AbortSignal) => apiFetch<QualityReport>(`/v1/collections/${id(collectionId)}/quality`, { signal }),
  /** Queues a background check unless one is running. */
  startQuality: (collectionId: string) => apiFetch<QualityReport>(`/v1/collections/${id(collectionId)}/quality`, { method: "POST" }),
  fix: (collectionId: string, fix: RecordFix) => apiFetch<{ count: number }>(`/v1/collections/${id(collectionId)}/fix`, { method: "POST", json: fix }),
  deleteInvalid: (collectionId: string) => apiFetch<{ count: number }>(`/v1/collections/${id(collectionId)}/delete-invalid`, { method: "POST" }),
  duplicate: (collectionId: string, name: string, withRecords: boolean) =>
    apiFetch<Collection>(`/v1/collections/${id(collectionId)}/duplicate`, { method: "POST", json: { name, records: withRecords } }),
  stats: (collectionId: string, signal?: AbortSignal) =>
    apiFetch<CollectionStats>(`/v1/collections/${id(collectionId)}/stats${query({ tz: Intl.DateTimeFormat().resolvedOptions().timeZone })}`, { signal }),
  leave: (collectionId: string) => apiFetch<void>(`/v1/collections/${id(collectionId)}/leave`, { method: "POST" }),
  removeMember: (collectionId: string, userId: string) => apiFetch<void>(`/v1/collections/${id(collectionId)}/members/${id(userId)}`, { method: "DELETE" }),
};

export const records = {
  /** filter: a JSON object of top-level fields and exact values. */
  /** sort: created_at, updated_at or data.<field>; ids: up to 100 record ids, comma separated. */
  list: (p: { collection_id: string; page: number; limit: number; search?: string; sort?: string; order?: SortOrder; filter?: string; where?: string; ids?: string }, signal?: AbortSignal) =>
    apiFetch<Paged<DataRecord>>(`/v1/records${query(p)}`, { signal }),
  versions: (recordId: string, signal?: AbortSignal) => apiFetch<{ items: RecordVersion[] }>(`/v1/records/${id(recordId)}/versions`, { signal }),
  restore: (recordId: string, version: number, current_version?: number) =>
    apiFetch<DataRecord>(`/v1/records/${id(recordId)}/restore`, { method: "POST", json: { version, current_version } }),
  /** One batch of a file import; dry_run only checks. */
  importBatch: (body: { collection_id: string; records: unknown[]; dry_run?: boolean; skip_invalid?: boolean; upsert_key?: string }) =>
    apiFetch<ImportResult>("/v1/records/import", { method: "POST", json: body }),
  export: (collection_id: string, format: "jsonl" | "csv", valid: boolean) =>
    apiDownload(`/v1/records/export${query({ collection_id, format, valid: valid || undefined })}`, `records.${format}`),
  /** Counts the selected records by a field (or all together) with sums and averages of numeric fields. */
  aggregate: (p: { collection_id: string; search?: string; filter?: string; where?: string; group?: string; sum?: string }, signal?: AbortSignal) =>
    apiFetch<{ items: RecordGroup[] }>(`/v1/records/aggregate${query(p)}`, { signal }),
  get: (recordId: string) => apiFetch<DataRecord>(`/v1/records/${id(recordId)}`),
  create: (collection_id: string, data: unknown) => apiFetch<DataRecord>("/v1/records", { method: "POST", json: { collection_id, data } }),
  update: (recordId: string, data: unknown, version?: number) => apiFetch<DataRecord>(`/v1/records/${id(recordId)}`, { method: "PATCH", json: { data, version } }),
  remove: (ids: string[]) => apiFetch<void>("/v1/records/delete", { method: "POST", json: { ids } }),
  /** Sets the same fields on records of one collection, all or none; null removes a field. */
  bulkUpdate: (ids: string[], set: Record<string, unknown>) => apiFetch<{ count: number }>("/v1/records/bulk-update", { method: "POST", json: { ids, set } }),
  links: (recordId: string) => apiFetch<{ items: RecordLinks[] }>(`/v1/records/${id(recordId)}/links`),
  comments: (recordId: string) => apiFetch<{ items: RecordComment[] }>(`/v1/records/${id(recordId)}/comments`),
  addComment: (recordId: string, body: string) => apiFetch<RecordComment>(`/v1/records/${id(recordId)}/comments`, { method: "POST", json: { body } }),
  deleteComment: (recordId: string, commentId: string) => apiFetch<void>(`/v1/records/${id(recordId)}/comments/${id(commentId)}`, { method: "DELETE" }),
};

const col = (collectionId: string) => `/v1/collections/${id(collectionId)}`;

/** API keys, webhooks, trash, field history, saved views and files of a collection. */
export const tools = {
  keys: (collectionId: string) => apiFetch<{ items: ApiKey[] }>(`${col(collectionId)}/keys`),
  createKey: (collectionId: string, name: string, can_write: boolean) => apiFetch<ApiKey & { secret: string }>(`${col(collectionId)}/keys`, { method: "POST", json: { name, can_write } }),
  deleteKey: (collectionId: string, keyId: string) => apiFetch<void>(`${col(collectionId)}/keys/${id(keyId)}`, { method: "DELETE" }),
  shares: (collectionId: string) => apiFetch<{ items: ShareLink[] }>(`${col(collectionId)}/shares`),
  createShare: (collectionId: string, kind: ShareLink["kind"]) => apiFetch<ShareLink>(`${col(collectionId)}/shares`, { method: "POST", json: { kind } }),
  deleteShare: (collectionId: string, shareId: string) => apiFetch<void>(`${col(collectionId)}/shares/${id(shareId)}`, { method: "DELETE" }),
  webhooks: (collectionId: string) => apiFetch<{ items: Webhook[] }>(`${col(collectionId)}/webhooks`),
  createWebhook: (collectionId: string, url: string, events: WebhookEvent[]) => apiFetch<Webhook & { secret: string }>(`${col(collectionId)}/webhooks`, { method: "POST", json: { url, events } }),
  deleteWebhook: (collectionId: string, hookId: string) => apiFetch<void>(`${col(collectionId)}/webhooks/${id(hookId)}`, { method: "DELETE" }),
  /** Sends a test delivery now; answers the hook with its new last status. */
  testWebhook: (collectionId: string, hookId: string) => apiFetch<Webhook>(`${col(collectionId)}/webhooks/${id(hookId)}/test`, { method: "POST" }),
  deliveries: (collectionId: string, hookId: string) => apiFetch<{ items: WebhookDelivery[] }>(`${col(collectionId)}/webhooks/${id(hookId)}/deliveries`),
  redeliver: (collectionId: string, hookId: string, deliveryId: string) =>
    apiFetch<WebhookDelivery>(`${col(collectionId)}/webhooks/${id(hookId)}/deliveries/${id(deliveryId)}/redeliver`, { method: "POST" }),
  automations: (collectionId: string) => apiFetch<{ items: Automation[] }>(`${col(collectionId)}/automations`),
  createAutomation: (collectionId: string, input: AutomationInput) => apiFetch<Automation>(`${col(collectionId)}/automations`, { method: "POST", json: input }),
  updateAutomation: (collectionId: string, autoId: string, input: AutomationInput) => apiFetch<Automation>(`${col(collectionId)}/automations/${id(autoId)}`, { method: "PUT", json: input }),
  deleteAutomation: (collectionId: string, autoId: string) => apiFetch<void>(`${col(collectionId)}/automations/${id(autoId)}`, { method: "DELETE" }),
  runAutomation: (collectionId: string, autoId: string) => apiFetch<Automation>(`${col(collectionId)}/automations/${id(autoId)}/run`, { method: "POST" }),
  trash: (collectionId: string, signal?: AbortSignal) => apiFetch<{ items: TrashedRecord[]; retention_days: number }>(`${col(collectionId)}/trash`, { signal }),
  restoreRecords: (collectionId: string, ids: string[]) => apiFetch<{ count: number }>(`${col(collectionId)}/trash/restore`, { method: "POST", json: { ids } }),
  purgeRecords: (collectionId: string, ids: string[]) => apiFetch<{ count: number }>(`${col(collectionId)}/trash/purge`, { method: "POST", json: { ids } }),
  trashedCollections: () => apiFetch<{ items: TrashedCollection[]; retention_days: number }>("/v1/collections/trash"),
  restoreCollection: (collectionId: string) => apiFetch<void>(`/v1/collections/trash/${id(collectionId)}/restore`, { method: "POST" }),
  purgeCollection: (collectionId: string) => apiFetch<void>(`/v1/collections/trash/${id(collectionId)}`, { method: "DELETE" }),
  schemaVersions: (collectionId: string, signal?: AbortSignal) => apiFetch<{ items: SchemaVersion[] }>(`${col(collectionId)}/schema-versions`, { signal }),
  dashboard: (collectionId: string, signal?: AbortSignal) => apiFetch<{ widgets: Widget[] }>(`${col(collectionId)}/dashboard`, { signal }),
  saveDashboard: (collectionId: string, widgets: Widget[]) => apiFetch<void>(`${col(collectionId)}/dashboard`, { method: "PUT", json: { widgets } }),
  views: (collectionId: string) => apiFetch<{ items: SavedView[] }>(`${col(collectionId)}/views`),
  createView: (collectionId: string, input: { name: string; query: string; shared: boolean }) => apiFetch<SavedView>(`${col(collectionId)}/views`, { method: "POST", json: input }),
  deleteView: (collectionId: string, viewId: string) => apiFetch<void>(`${col(collectionId)}/views/${id(viewId)}`, { method: "DELETE" }),
  upload: (collectionId: string, file: File) => apiFetch<FileRef>(`${col(collectionId)}/files?name=${encodeURIComponent(file.name)}`, { method: "POST", blob: file.type ? file : new Blob([file], { type: "application/octet-stream" }) }),
  fileUrl: (fileId: string) => `/api/proxy/v1/files/${id(fileId)}`,
};

export const audit = {
  list: (p: { page: number; limit: number; action?: string; user_id?: string; from?: string; to?: string }, signal?: AbortSignal) =>
    apiFetch<Paged<AuditRecord>>(`/v1/audit${query(p)}`, { signal }),
  /** Downloads the filtered log as CSV; more than 10 000 records is a 400 asking for narrower filters. */
  export: (p: { action?: string; user_id?: string; from?: string; to?: string }) => apiDownload(`/v1/audit/export${query(p)}`, "audit.csv"),
};

/** A share link, opened by anyone holding it (no session). */
export const publicLinks = {
  get: (token: string) => apiFetch<PublicLink>(`/v1/public/${id(token)}`),
  records: (token: string, p: { page: number; limit: number; search?: string }, signal?: AbortSignal) =>
    apiFetch<Paged<DataRecord>>(`/v1/public/${id(token)}/records${query(p)}`, { signal }),
  submit: (token: string, data: unknown) => apiFetch<{ id: string }>(`/v1/public/${id(token)}/records`, { method: "POST", json: { data } }),
};
