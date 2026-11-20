// One function per API route the screens use. Paths match the backend Swagger.
import { ApiError, apiDownload, apiFetch } from "@/lib/api/client";
import type { At } from "@/lib/paths";
import type {
  Automation,
  AutomationInput,
  McpCall,
  McpTestResult,
  McpTool,
  McpToolInput,
  Widget,
  PublicLink,
  WebhookDelivery,
  RecordComment, RecordLinks,
  ShareLink,
  AuditRecord,
  InviteResult,
  MeResponse,
  Notification,
  Paged,
  Preferences,
  Collection,
  CollectionMember,
  CollectionStats,
  MemberRole,
  QualityReport,
  DataRecord,
  ImportResult,
  RecordFix,
  RecordVersion,
  Session,
  SortOrder,
  User,
  CollectionOption,
  UserDetails,
  UserStatus,
  ApiKey,
  KeyScope,
  Webhook,
  WebhookEvent,
  TrashedRecord,
  TrashedCollection,
  SchemaVersion,
  SavedView,
  FileRef,
  RecordGroup,
  RecordStats,
  StatsBucket,
} from "@/lib/api/types";

/** The caller's time zone: rules with {"today": n} dates count days in it. */
const withTz = <T extends { where?: string }>(p: T) => (p.where ? { ...p, tz: Intl.DateTimeFormat().resolvedOptions().timeZone } : p);

function query(params: Record<string, string | number | boolean | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v !== undefined && v !== "") q.set(k, String(v));
  const s = q.toString();
  return s ? `?${s}` : "";
}

const id = (v: string) => encodeURIComponent(v);

export const account = {
  me: () => apiFetch<MeResponse>("/v1/me"),
  /** Changes the name, the username or both; a new username moves the account's collection addresses. */
  updateProfile: (input: { name?: string; username?: string }) => apiFetch<MeResponse>("/v1/me", { method: "PATCH", json: input }),
  changePassword: (current_password: string, new_password: string) =>
    apiFetch<void>("/v1/me/password", { method: "POST", json: { current_password, new_password } }),
  sessions: () => apiFetch<{ items: Session[] }>("/v1/me/sessions"),
  revokeSession: (sessionId: string) => apiFetch<void>(`/v1/me/sessions/${id(sessionId)}`, { method: "DELETE" }),
  keys: () => apiFetch<{ items: ApiKey[] }>("/v1/me/keys"),
  createKey: (name: string, scopes: KeyScope[], expiresInDays = 0) =>
    apiFetch<ApiKey & { secret: string }>("/v1/me/keys", { method: "POST", json: { name, scopes, expires_in_days: expiresInDays } }),
  // renewDays: undefined keeps the end; 0 removes it; 1-365 counts from now.
  updateKey: (keyId: string, name: string, scopes: KeyScope[], renewDays?: number) => apiFetch<ApiKey>(`/v1/me/keys/${id(keyId)}`, { method: "PUT", json: { name, scopes, renew_days: renewDays } }),
  deleteKey: (keyId: string) => apiFetch<void>(`/v1/me/keys/${id(keyId)}`, { method: "DELETE" }),
  keyCalls: (keyId: string) => apiFetch<{ items: McpCall[] }>(`/v1/me/keys/${id(keyId)}/calls`),
  mfa: () => apiFetch<{ enabled: boolean; recovery_left: number }>("/v1/me/mfa"),
  setupMFA: () => apiFetch<{ secret: string; uri: string }>("/v1/me/mfa/setup", { method: "POST" }),
  enableMFA: (code: string) => apiFetch<{ recovery_codes: string[] }>("/v1/me/mfa/enable", { method: "POST", json: { code } }),
  disableMFA: (password: string) => apiFetch<void>("/v1/me/mfa/disable", { method: "POST", json: { password } }),
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
  invite: (name: string, email: string) => apiFetch<InviteResult>("/v1/users", { method: "POST", json: { name, email } }),
  /** A new activation email for a pending account; the old link stops working. */
  reinvite: (userId: string) => apiFetch<InviteResult>(`/v1/users/${id(userId)}/invite`, { method: "POST" }),
  /** A new activation link for a pending account, without email; the old link stops working. */
  activationLink: (userId: string) =>
    apiFetch<{ user_id: string; status: UserStatus; activation_token: string; expires_at: string }>(`/v1/users/${id(userId)}/activation-link`, { method: "POST" }),
  /** Gives or takes the admin role; password is the caller's own. */
  setAdmin: (userId: string, admin: boolean, password: string) =>
    apiFetch<void>(`/v1/users/${id(userId)}/admin`, { method: "PUT", json: { admin, password } }),
  revokeSessions: (userId: string) => apiFetch<void>(`/v1/users/${id(userId)}/sessions`, { method: "DELETE" }),
  get: (userId: string) => apiFetch<UserDetails>(`/v1/users/${id(userId)}`),
  disable: (ids: string[]) => apiFetch<void>("/v1/users/disable", { method: "POST", json: { ids } }),
  enable: (ids: string[]) => apiFetch<void>("/v1/users/enable", { method: "POST", json: { ids } }),
  unlock: (ids: string[]) => apiFetch<void>("/v1/users/unlock", { method: "POST", json: { ids } }),
};


export interface CollectionInput {
  name: string;
  description: string;
  icon?: string;
  schema: unknown;
}

let picker: { read: number; items: Promise<CollectionOption[]> } | undefined;

/** A collection's API address, /v1/collections/<owner>/<slug>, as its page's. */
const col = (c: At) => `/v1/collections/${id(c.owner)}/${id(c.slug)}`;

export const collections = {
  /** details adds member_count, field_count and last_activity, which cost a pass over members and records. */
  list: (p: { page: number; limit: number; search?: string; sort?: string; order?: SortOrder; details?: boolean }, signal?: AbortSignal) =>
    apiFetch<Paged<Collection>>(`/v1/collections${query(p)}`, { signal }),
  /** Every collection the caller reaches (up to 1000) as id, name and role, without counts: for pickers. */
  options: (signal?: AbortSignal) => apiFetch<{ items: CollectionOption[] }>("/v1/collections/options", { signal }),
  /**
   * The address of a collection known by id (a reference field, an audit entry), from
   * the picker list, read once per 30 s; a miss reads it again, for a collection just made.
   */
  at: async (collectionId: string): Promise<CollectionOption> => {
    for (const fresh of [false, true]) {
      if (fresh || !picker || Date.now() - picker.read > 30_000) {
        const items = collections.options().then((r) => r.items);
        picker = { read: Date.now(), items };
        items.catch(() => { picker = undefined; });
      }
      const found = (await picker.items).find((c) => c.id === collectionId);
      if (found) return found;
    }
    throw new ApiError(404, { code: "NOT_FOUND", message: "collection not found" });
  },
  /** A collection known by id, with its schema. */
  byID: async (collectionId: string, signal?: AbortSignal) => collections.get(await collections.at(collectionId), signal),
  /** An old address (before a rename, transfer or username change) answers too: compare owner and slug to move on. */
  get: (c: At, signal?: AbortSignal) => apiFetch<Collection>(col(c), { signal }),
  /** Moves the address to another owner member of the collection. */
  transfer: (c: At, userId: string) => apiFetch<Collection>(`${col(c)}/transfer`, { method: "POST", json: { user_id: userId } }),
  create: (input: CollectionInput) => apiFetch<Collection>("/v1/collections", { method: "POST", json: input }),
  update: (c: At, input: Partial<CollectionInput> & { version?: number }) => apiFetch<Collection>(`${col(c)}`, { method: "PATCH", json: input }),
  remove: (ids: string[]) => apiFetch<void>("/v1/collections/delete", { method: "POST", json: { ids } }),
  members: (c: At) => apiFetch<{ items: CollectionMember[] }>(`${col(c)}/members`),
  /** An email without an account is invited (name required) and joins at once. */
  addMember: (c: At, input: { email: string; name?: string; role: MemberRole }) =>
    apiFetch<{ member: CollectionMember; invited: boolean; activation_token?: string }>(`${col(c)}/members`, { method: "POST", json: input }),
  setMemberRole: (c: At, userId: string, role: MemberRole) =>
    apiFetch<void>(`${col(c)}/members/${id(userId)}`, { method: "PATCH", json: { role } }),
  /** The collection's own audit log (owners). */
  audit: (c: At, p: { page: number; limit: number; action?: string; user_id?: string; from?: string; to?: string }, signal?: AbortSignal) =>
    apiFetch<Paged<AuditRecord>>(`${col(c)}/audit${query(p)}`, { signal }),
  /** Checks every record against the current schema (editors and owners). */
  quality: (c: At, signal?: AbortSignal) => apiFetch<QualityReport>(`${col(c)}/quality`, { signal }),
  /** Queues a background check unless one is running. */
  startQuality: (c: At) => apiFetch<QualityReport>(`${col(c)}/quality`, { method: "POST" }),
  fix: (c: At, fix: RecordFix) => apiFetch<{ count: number }>(`${col(c)}/quality/fix`, { method: "POST", json: fix }),
  deleteInvalid: (c: At) => apiFetch<{ count: number }>(`${col(c)}/quality/delete-invalid`, { method: "POST" }),
  duplicate: (c: At, name: string, withRecords: boolean) =>
    apiFetch<Collection>(`${col(c)}/duplicate`, { method: "POST", json: { name, records: withRecords } }),
  stats: (c: At, signal?: AbortSignal) =>
    apiFetch<CollectionStats>(`${col(c)}/stats${query({ tz: Intl.DateTimeFormat().resolvedOptions().timeZone })}`, { signal }),
  leave: (c: At) => apiFetch<void>(`${col(c)}/leave`, { method: "POST" }),
  removeMember: (c: At, userId: string) => apiFetch<void>(`${col(c)}/members/${id(userId)}`, { method: "DELETE" }),
};

export const records = {
  /** filter: a JSON object of top-level fields and exact values. */
  /** sort: created_at, updated_at or data.<field>; ids: up to 100 record ids, comma separated. */
  list: (c: At, p: { page: number; limit: number; search?: string; sort?: string; order?: SortOrder; filter?: string; where?: string; ids?: string }, signal?: AbortSignal) =>
    apiFetch<Paged<DataRecord>>(`${col(c)}/records${query(withTz(p))}`, { signal }),
  versions: (recordId: string, signal?: AbortSignal) => apiFetch<{ items: RecordVersion[] }>(`/v1/records/${id(recordId)}/versions`, { signal }),
  restore: (recordId: string, version: number, current_version?: number) =>
    apiFetch<DataRecord>(`/v1/records/${id(recordId)}/restore`, { method: "POST", json: { version, current_version } }),
  /** One batch of a file import; dry_run only checks. */
  importBatch: (c: At, body: { records: unknown[]; sourceRecords?: string[]; dry_run?: boolean; skip_invalid?: boolean; upsert_key?: string; batch_key?: string }) => {
    if (!body.sourceRecords) return apiFetch<ImportResult>(`${col(c)}/records/import`, { method: "POST", json: body });
    const rest = JSON.stringify({ ...body, records: undefined, sourceRecords: undefined });
    const head = rest === "{}" ? "{" : `${rest.slice(0, -1)},`;
    const blob = new Blob([head, '"records":[', body.sourceRecords.join(","), "]}"], { type: "application/json" });
    return apiFetch<ImportResult>(`${col(c)}/records/import`, { method: "POST", blob });
  },
  export: (c: At, format: "jsonl" | "csv" | "collection", valid: boolean) =>
    apiDownload(`${col(c)}/records/export${query({ format, valid: valid || undefined })}`, `records.${format}`),
  /** Counts the selected records by a field (or all together) with sums and averages of numeric fields. */
  aggregate: (c: At, p: { search?: string; filter?: string; where?: string; group?: string; sum?: string }, signal?: AbortSignal) =>
    apiFetch<{ items: RecordGroup[] }>(`${col(c)}/records/aggregate${query(withTz(p))}`, { signal }),
  /** Field lists are comma separated: filled counts, number ranges, date ranges with a series, common values. */
  stats: (c: At, p: { where?: string; filled?: string; numbers?: string; dates?: string; values?: string; bucket?: StatsBucket }, signal?: AbortSignal) =>
    apiFetch<RecordStats>(`${col(c)}/records/profile${query(withTz(p))}`, { signal }),
  get: (recordId: string) => apiFetch<DataRecord>(`/v1/records/${id(recordId)}`),
  create: (c: At, data: unknown) => apiFetch<DataRecord>(`${col(c)}/records`, { method: "POST", json: { data } }),
  update: (recordId: string, data: unknown, version?: number) => apiFetch<DataRecord>(`/v1/records/${id(recordId)}`, { method: "PUT", json: { data, version } }),
  remove: (ids: string[]) => apiFetch<void>("/v1/records/delete", { method: "POST", json: { ids } }),
  /** Sets the same fields on records of one collection, all or none; null removes a field. */
  bulkUpdate: (ids: string[], set: Record<string, unknown>) => apiFetch<{ count: number }>("/v1/records/update", { method: "POST", json: { ids, set } }),
  links: (recordId: string) => apiFetch<{ items: RecordLinks[] }>(`/v1/records/${id(recordId)}/links`),
  comments: (recordId: string) => apiFetch<{ items: RecordComment[] }>(`/v1/records/${id(recordId)}/comments`),
  addComment: (recordId: string, body: string) => apiFetch<RecordComment>(`/v1/records/${id(recordId)}/comments`, { method: "POST", json: { body } }),
  deleteComment: (recordId: string, commentId: string) => apiFetch<void>(`/v1/records/${id(recordId)}/comments/${id(commentId)}`, { method: "DELETE" }),
};

/** API keys, webhooks, trash, field history, saved views and files of a collection. */
/** The consent page of an assistant's OAuth sign in (the ChatGPT app). */
export const oauth = {
  client: (clientId: string, redirectUri: string) =>
    apiFetch<{ client_name: string }>(`/v1/oauth/client?${new URLSearchParams({ client_id: clientId, redirect_uri: redirectUri })}`),
  approve: (body: { client_id: string; redirect_uri: string; code_challenge: string; code_challenge_method: string; name: string; scopes: KeyScope[]; expires_in_days: number }) =>
    apiFetch<{ code: string }>("/v1/oauth/approve", { method: "POST", json: body }),
};

export const tools = {
  keys: (c: At) => apiFetch<{ items: ApiKey[] }>(`${col(c)}/keys`),
  /** Takes the collection out of the key; the key keeps its other collections. */
  deleteKey: (c: At, keyId: string) => apiFetch<void>(`${col(c)}/keys/${id(keyId)}`, { method: "DELETE" }),
  shares: (c: At) => apiFetch<{ items: ShareLink[] }>(`${col(c)}/shares`),
  createShare: (c: At, kind: ShareLink["kind"], expiresInDays: number) =>
    apiFetch<ShareLink>(`${col(c)}/shares`, { method: "POST", json: { kind, expires_in_days: expiresInDays } }),
  deleteShare: (c: At, shareId: string) => apiFetch<void>(`${col(c)}/shares/${id(shareId)}`, { method: "DELETE" }),
  webhooks: (c: At) => apiFetch<{ items: Webhook[] }>(`${col(c)}/webhooks`),
  createWebhook: (c: At, url: string, events: WebhookEvent[]) => apiFetch<Webhook & { secret: string }>(`${col(c)}/webhooks`, { method: "POST", json: { url, events } }),
  // enabled false pauses the hook; true resumes it and clears its failures. The secret stays.
  updateWebhook: (c: At, hookId: string, url: string, events: WebhookEvent[], enabled: boolean) =>
    apiFetch<Webhook>(`${col(c)}/webhooks/${id(hookId)}`, { method: "PATCH", json: { url, events, enabled } }),
  deleteWebhook: (c: At, hookId: string) => apiFetch<void>(`${col(c)}/webhooks/${id(hookId)}`, { method: "DELETE" }),
  /** Sends a test delivery now; answers the hook with its new last status. */
  testWebhook: (c: At, hookId: string) => apiFetch<Webhook>(`${col(c)}/webhooks/${id(hookId)}/test`, { method: "POST" }),
  deliveries: (c: At, hookId: string) => apiFetch<{ items: WebhookDelivery[] }>(`${col(c)}/webhooks/${id(hookId)}/deliveries`),
  redeliver: (c: At, hookId: string, deliveryId: string) =>
    apiFetch<WebhookDelivery>(`${col(c)}/webhooks/${id(hookId)}/deliveries/${id(deliveryId)}/redeliver`, { method: "POST" }),
  automations: (c: At) => apiFetch<{ items: Automation[] }>(`${col(c)}/automations`),
  createAutomation: (c: At, input: AutomationInput) => apiFetch<Automation>(`${col(c)}/automations`, { method: "POST", json: input }),
  updateAutomation: (c: At, autoId: string, input: AutomationInput) => apiFetch<Automation>(`${col(c)}/automations/${id(autoId)}`, { method: "PUT", json: input }),
  deleteAutomation: (c: At, autoId: string) => apiFetch<void>(`${col(c)}/automations/${id(autoId)}`, { method: "DELETE" }),
  mcpTools: (c: At) => apiFetch<{ items: McpTool[] }>(`${col(c)}/mcp-tools`),
  createMcpTool: (c: At, input: McpToolInput) => apiFetch<McpTool>(`${col(c)}/mcp-tools`, { method: "POST", json: input }),
  updateMcpTool: (c: At, toolId: string, input: McpToolInput) => apiFetch<McpTool>(`${col(c)}/mcp-tools/${id(toolId)}`, { method: "PUT", json: input }),
  deleteMcpTool: (c: At, toolId: string) => apiFetch<void>(`${col(c)}/mcp-tools/${id(toolId)}`, { method: "DELETE" }),
  testMcpTool: (c: At, tool: McpToolInput, args: Record<string, unknown>) =>
    apiFetch<McpTestResult>(`${col(c)}/mcp-tools/test`, { method: "POST", json: { tool, args } }),
  mcpCalls: (c: At) => apiFetch<{ items: McpCall[] }>(`${col(c)}/mcp-calls`),
  runAutomation: (c: At, autoId: string) => apiFetch<Automation>(`${col(c)}/automations/${id(autoId)}/run`, { method: "POST" }),
  trash: (c: At, signal?: AbortSignal) => apiFetch<{ items: TrashedRecord[]; retention_days: number }>(`${col(c)}/trash`, { signal }),
  /** Values, for one record, give its unique fields new values on the way back. */
  restoreRecords: (c: At, ids: string[], values?: Record<string, unknown>) => apiFetch<{ count: number }>(`${col(c)}/trash/restore`, { method: "POST", json: values ? { ids, values } : { ids } }),
  purgeRecords: (c: At, ids: string[]) => apiFetch<{ count: number }>(`${col(c)}/trash/purge`, { method: "POST", json: { ids } }),
  trashedCollections: () => apiFetch<{ items: TrashedCollection[]; retention_days: number }>("/v1/collections/trash"),
  restoreCollection: (collectionId: string) => apiFetch<void>(`/v1/collections/trash/${id(collectionId)}/restore`, { method: "POST" }),
  purgeCollection: (collectionId: string) => apiFetch<void>(`/v1/collections/trash/${id(collectionId)}`, { method: "DELETE" }),
  schemaVersions: (c: At, signal?: AbortSignal) => apiFetch<{ items: SchemaVersion[] }>(`${col(c)}/schema-versions`, { signal }),
  dashboard: (c: At, signal?: AbortSignal) => apiFetch<{ widgets: Widget[] }>(`${col(c)}/dashboard`, { signal }),
  saveDashboard: (c: At, widgets: Widget[]) => apiFetch<void>(`${col(c)}/dashboard`, { method: "PUT", json: { widgets } }),
  views: (c: At) => apiFetch<{ items: SavedView[] }>(`${col(c)}/views`),
  createView: (c: At, input: { name: string; query: string; shared: boolean }) => apiFetch<SavedView>(`${col(c)}/views`, { method: "POST", json: input }),
  deleteView: (c: At, viewId: string) => apiFetch<void>(`${col(c)}/views/${id(viewId)}`, { method: "DELETE" }),
  upload: (c: At, file: File) => apiFetch<FileRef>(`${col(c)}/files?name=${encodeURIComponent(file.name)}`, { method: "POST", blob: file.type ? file : new Blob([file], { type: "application/octet-stream" }) }),
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
