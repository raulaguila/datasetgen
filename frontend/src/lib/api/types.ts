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

export interface Product {
  id: string;
  name: string;
  price: string;
  enabled: boolean;
  version: number;
  created_at: string;
  updated_at: string;
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
