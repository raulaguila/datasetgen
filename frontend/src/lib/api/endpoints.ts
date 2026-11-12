// One function per API route the screens use. Paths match the backend Swagger.
import { apiDownload, apiFetch } from "@/lib/api/client";
import type {
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
  MemberRole,
  DataRecord,
  Resource,
  Role,
  Session,
  SortOrder,
  User,
  UserCounts,
  UserDetails,
  UserStatus,
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
  list: (p: { page: number; limit: number; search?: string; sort?: string; order?: SortOrder }, signal?: AbortSignal) =>
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
  removeMember: (collectionId: string, userId: string) => apiFetch<void>(`/v1/collections/${id(collectionId)}/members/${id(userId)}`, { method: "DELETE" }),
};

export const records = {
  list: (p: { collection_id: string; page: number; limit: number; search?: string; order?: SortOrder }, signal?: AbortSignal) =>
    apiFetch<Paged<DataRecord>>(`/v1/records${query(p)}`, { signal }),
  get: (recordId: string) => apiFetch<DataRecord>(`/v1/records/${id(recordId)}`),
  create: (collection_id: string, data: unknown) => apiFetch<DataRecord>("/v1/records", { method: "POST", json: { collection_id, data } }),
  update: (recordId: string, data: unknown, version?: number) => apiFetch<DataRecord>(`/v1/records/${id(recordId)}`, { method: "PATCH", json: { data, version } }),
  remove: (ids: string[]) => apiFetch<void>("/v1/records/delete", { method: "POST", json: { ids } }),
};

export const audit = {
  list: (p: { page: number; limit: number; action?: string; user_id?: string; from?: string; to?: string }, signal?: AbortSignal) =>
    apiFetch<Paged<AuditRecord>>(`/v1/audit${query(p)}`, { signal }),
  /** Downloads the filtered log as CSV; more than 10 000 records is a 400 asking for narrower filters. */
  export: (p: { action?: string; user_id?: string; from?: string; to?: string }) => apiDownload(`/v1/audit/export${query(p)}`, "audit.csv"),
};
