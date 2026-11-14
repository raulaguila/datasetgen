// Contract of the Go API (see the backend Swagger at /swagger/index.html).

export type Action = "create" | "read" | "update" | "delete";
export type UserStatus = "pending" | "active" | "locked" | "disabled";
export type Theme = "system" | "light" | "dark";
export type Language = "pt-BR" | "en-US";

export interface ApiErrorBody {
  code: string;
  message: string;
  field?: string;
  details?: Record<string, unknown>;
}

export interface Preferences {
  theme: Theme;
  language: Language;
}

export interface Me {
  id: string;
  name: string;
  email: string;
  status: UserStatus;
  created_at: string;
  is_superuser: boolean;
  permissions: Record<string, Action[]>;
  preferences: Preferences;
  /** Names of the enabled roles. */
  roles: string[];
  /** When the profile picture was last set; absent when there is none. */
  avatar_at?: string;
}

export interface MeResponse {
  user: Me;
  session_id: string;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  token_type: string;
  user?: Me;
}

export interface Pagination {
  page: number;
  limit: number;
  total_items: number;
  total_pages: number;
}

export interface Paged<T> {
  items: T[];
  pagination: Pagination;
}

export interface Session {
  id: string;
  ip_address: string | null;
  user_agent: string | null;
  created_at: string;
  last_seen_at: string;
  expires_at: string;
  current: boolean;
}

export interface Notification {
  id: string;
  level: "info" | "warning" | "critical";
  /** Translate by this; title and message are the English fallback. */
  kind?: "new_login" | "account_locked" | "session_reused" | "password_reset" | "password_changed" | "collection_added";
  params?: Record<string, string>;
  title: string;
  message: string;
  read_at: string | null;
  created_at: string;
}

export interface User {
  id: string;
  name: string;
  email: string;
  status: UserStatus;
  created_at: string;
  updated_at: string;
}

/** The user list's size: everyone it shows, and those waiting on an action. */
export interface UserCounts {
  total: number;
  pending: number;
  locked: number;
}

export interface UserDetails extends User {
  roles: { id: string; name: string }[];
  access?: {
    last_login_at: string | null;
    failed_logins: number;
    locked_until: string | null;
    recent_failures: number;
    active_sessions: number;
    /** When the live activation link stops working; null when there is none. */
    activation_expires_at: string | null;
  };
}

export interface Role {
  id: string;
  name: string;
  enabled: boolean;
  is_default: boolean;
  permissions: Record<string, Action[]>;
  member_count: number;
  created_at: string;
}

export interface Resource {
  code: string;
  description: string;
  /** The actions a role may be granted on it, in create, read, update, delete order. */
  actions: Action[];
}

export interface Member {
  id: string;
  name: string;
  email: string;
  status: UserStatus;
}

export interface Collection {
  id: string;
  name: string;
  description: string;
  /** JSON Schema; absent from list responses. */
  schema?: Record<string, unknown>;
  /** Present in list responses only. */
  record_count?: number;
  /** A slug of COLLECTION_ICONS; "" shows the initials. */
  icon: string;
  /** False for a platform admin who sees it without belonging to it; list only. */
  member?: boolean;
  /** Present in list responses asked with details only. */
  member_count?: number;
  field_count?: number;
  /** The newest change to the collection or one of its records. */
  last_activity?: string;
  /** The caller's role in this collection; a platform admin is owner everywhere. */
  role?: MemberRole;
  version: number;
  created_at: string;
  updated_at: string;
}

/** A collection's overview of the last 30 days. */
export interface CollectionStats {
  records: number;
  members: number;
  last_update?: string;
  /** Days with records created, oldest first (YYYY-MM-DD in the requested time zone). */
  per_day: { day: string; count: number }[];
  contributors: { user_id: string; name: string; created: number; edited: number }[];
}

/** Stored records that no longer match the collection's schema. */
/** The latest background check of a collection's records against its schema. */
export interface QualityReport {
  status: "none" | "running" | "done" | "failed";
  /** The collection version checked; below current_version the check is stale. */
  version: number;
  current_version: number;
  checked: number;
  invalid_count: number;
  /** The first 100, with the rules each breaks. */
  invalid: { id: string; data: Record<string, unknown>; updated_at: string; issues: { path: string; keyword: string; message: string }[] }[];
  /** The check stopped at 500 000 records. */
  truncated: boolean;
  error?: string;
  started_at?: string;
  finished_at?: string;
}

/** A bulk change to every record: set_default fills a missing field, rename moves it, remove drops it. */
export interface RecordFix {
  op: "set_default" | "rename" | "remove";
  field: string;
  to?: string;
  value?: unknown;
  /** Only count the records the change would touch. */
  dry_run?: boolean;
}

export interface RecordVersion {
  version: number;
  data: unknown;
  saved_at: string;
}

export interface ImportResult {
  created: number;
  /** Records an upsert merged into instead of creating. */
  updated?: number;
  invalid: number;
  /** index is the record's position in that batch. */
  errors: { index: number; issues: SchemaIssue[] }[];
}

export type MemberRole = "owner" | "editor" | "viewer";

export interface CollectionMember {
  user_id: string;
  name: string;
  email: string;
  status: "pending" | "active" | "locked" | "disabled";
  role: MemberRole;
  created_at: string;
}

/** One entry of a collection; data follows the collection schema. */
export interface DataRecord {
  id: string;
  collection_id: string;
  data: unknown;
  version: number;
  created_at: string;
  updated_at: string;
}

/** One broken schema rule; path is a JSON pointer into the record data. */
export interface SchemaIssue {
  path: string;
  /** The failed rule, such as "required"; the form translates by it. */
  keyword?: string;
  message: string;
}

export type SortOrder = "asc" | "desc";

export interface AuditRecord {
  id: string;
  created_at: string;
  actor_id?: string;
  actor_name?: string;
  actor_email?: string;
  request_id?: string;
  action: string;
  target_id?: string;
  target_label?: string;
  /** The target was deleted since. */
  target_gone?: boolean;
  details: Record<string, unknown>;
}

export interface InviteResult {
  user_id: string;
  status: UserStatus;
  activation_token?: string;
}

/** A program's access to one collection's records; the secret shows once, at creation. */
/** Records of one collection whose reference field names a record. */
export interface RecordComment {
  id: string;
  author_id?: string;
  author_name?: string;
  body: string;
  created_at: string;
}

export interface RecordLinks {
  collection_id: string;
  collection_name: string;
  field: string;
  /** The reference subfield of each item when field is a list of objects. */
  sub?: string;
  schema: Record<string, unknown>;
  total: number;
  records: DataRecord[];
}

/** A public link: a form that adds records or a read-only list. */
export interface ShareLink {
  id: string;
  kind: "form" | "view";
  token: string;
  created_at: string;
  last_used_at?: string;
}

/** What a share link opens, as anyone holding it sees it. */
export interface PublicLink {
  kind: "form" | "view";
  name: string;
  description?: string;
  icon?: string;
  schema: Record<string, unknown>;
  /** False when the collection has private fields. */
  searchable: boolean;
}

export interface ApiKey {
  id: string;
  name: string;
  /** The first characters of the key, to tell keys apart. */
  prefix: string;
  can_write: boolean;
  creator_name?: string;
  created_at: string;
  last_used_at?: string;
}

export type WebhookEvent = "record.created" | "record.updated" | "record.deleted" | "records.imported";
export const WEBHOOK_EVENTS: WebhookEvent[] = ["record.created", "record.updated", "record.deleted", "records.imported"];

export type AutomationTrigger = "record.created" | "record.updated" | "daily";
export type AutomationAction = "set_field" | "notify";

export interface Automation {
  id: string;
  name: string;
  enabled: boolean;
  trigger: AutomationTrigger;
  /** Daily only: the hour it runs at (server time). */
  hour?: number;
  conditions: { any: boolean; rules: { field: string; op: string; value?: unknown }[]; groups?: { any: boolean; rules: { field: string; op: string; value?: unknown }[] }[] };
  action: AutomationAction;
  /** set_field: {field, value}; notify: {message}. */
  config: { field?: string; value?: unknown; message?: string };
  created_at: string;
  last_run_at?: string;
  last_error?: string;
  runs: number;
}

export type AutomationInput = Pick<Automation, "name" | "enabled" | "trigger" | "hour" | "conditions" | "action" | "config">;

export interface WebhookDelivery {
  id: string;
  event: string;
  status?: number;
  error?: string;
  created_at: string;
}

export interface Webhook {
  id: string;
  url: string;
  events: WebhookEvent[];
  created_at: string;
  last_status?: number;
  last_error?: string;
  last_at?: string;
}

export interface TrashedRecord {
  id: string;
  data: unknown;
  deleted_at: string;
  deleted_by_name?: string;
}

export interface TrashedCollection {
  id: string;
  name: string;
  icon: string;
  record_count: number;
  deleted_at: string;
}

export interface SchemaVersion {
  version: number;
  schema: Record<string, unknown>;
  saved_at: string;
  saved_by_name?: string;
}

/** A named record list query (the list's URL parameters). */
export interface SavedView {
  id: string;
  name: string;
  query: string;
  shared: boolean;
  mine: boolean;
  created_at: string;
}

/** An attached file as a record stores it. */
export interface FileRef {
  id: string;
  name: string;
  size: number;
  type: string;
}

/** Records sharing one value of a field (key; null for all of them), with sums and averages. */
export interface RecordGroup {
  key: unknown;
  count: number;
  sums: Record<string, number>;
  avgs: Record<string, number>;
  mins: Record<string, number>;
  maxs: Record<string, number>;
}

export type StatsBucket = "day" | "week" | "month" | "year";

/** One field over the selected records; only the parts asked for are present. */
export interface FieldProfile {
  filled?: number;
  min?: number;
  max?: number;
  avg?: number;
  sum?: number;
  /** First and last date, YYYY-MM-DD. */
  first?: string;
  last?: string;
  distinct?: number;
  top?: { key: unknown; count: number }[];
  /** Records per bucket, by the bucket's first day; empty buckets are absent. */
  series?: { at: string; count: number }[];
}

export interface RecordStats {
  total: number;
  fields: Record<string, FieldProfile>;
}

/** One dashboard widget; the page draws it from the record API. */
export interface Widget {
  id: string;
  kind: "count" | "sum" | "avg" | "min" | "max" | "bars" | "list" | "timeline";
  title: string;
  /** The number summed, averaged… or the field bars count by. */
  field?: string;
  /** The records it counts, as the list's conditions. */
  where?: { any: boolean; rules: { field: string; op: string; value?: unknown }[]; groups?: { any: boolean; rules: { field: string; op: string; value?: unknown }[] }[] };
  /** Two takes a double-width slot. */
  size?: 1 | 2;
  /** A timeline's span per bar. */
  bucket?: StatsBucket;
  /** How a number reads: plain, short (1,2 mil), money or a percentage. */
  format?: "number" | "compact" | "currency" | "percent";
  /** Digits after the decimal point; the locale's default when absent. */
  decimals?: number;
  /** How bars and timelines draw: bars, a donut or a table; columns or a line. */
  visual?: "bars" | "donut" | "table" | "columns" | "line";
}
