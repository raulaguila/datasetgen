// Browser-side API client. Every call goes through the BFF (/api/proxy), which
// attaches the token. A 401 tries one silent refresh before sending the user
// to /login; a transient refresh failure surfaces as an error instead.
"use client";

import type { ApiErrorBody } from "@/lib/api/types";
import { refreshAccessToken } from "@/lib/api/session-refresh";

export class ApiError extends Error {
  status: number;
  code: string;
  field?: string;
  details?: Record<string, unknown>;
  /** Seconds to wait before trying again, from Retry-After. */
  retryAfter?: number;
  constructor(status: number, body: Partial<ApiErrorBody>, retryAfter?: number) {
    super(body.message ?? `HTTP ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.code = body.code ?? "UNKNOWN";
    this.field = body.field;
    this.details = body.details;
    this.retryAfter = retryAfter;
  }
}

export async function errorFrom(res: Response): Promise<ApiError> {
  const body = (await res.json().catch(() => ({}))) as Partial<ApiErrorBody>;
  const retry = Number(res.headers.get("retry-after"));
  return new ApiError(res.status, body, Number.isFinite(retry) && retry > 0 ? retry : undefined);
}

export interface FetchOpts {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  json?: unknown;
  /** A raw body such as an image; sent with the blob's own type. */
  blob?: Blob;
  signal?: AbortSignal;
}

export async function redirectToLogin() {
  try {
    await fetch("/api/auth/logout", { method: "POST", signal: AbortSignal.timeout(5_000) });
  } catch {
    /* the BFF already cleared a dead cookie; this is a fallback */
  }
  const next = encodeURIComponent(window.location.pathname + window.location.search);
  // eslint-disable-next-line @next/next/no-location-assign-relative-destination -- a full load drops the previous user's cached data
  window.location.assign(`/login?next=${next}`);
}

/** The response of a successful call, after the silent refresh; throws ApiError otherwise. */
async function apiResponse(path: string, opts: FetchOpts): Promise<Response> {
  const init: RequestInit = {
    method: opts.method ?? "GET",
    signal: opts.signal,
    headers: opts.blob ? { "content-type": opts.blob.type } : opts.json === undefined ? undefined : { "content-type": "application/json" },
    body: opts.blob ?? (opts.json === undefined ? undefined : JSON.stringify(opts.json)),
  };
  let res = await fetch("/api/proxy" + path, init);
  if (res.status === 401) {
    const refreshed = await refreshAccessToken();
    if (refreshed.status === 401) {
      await redirectToLogin();
      throw await errorFrom(refreshed);
    }
    if (!refreshed.ok) throw await errorFrom(refreshed);
    res = await fetch("/api/proxy" + path, init);
    if (res.status === 401) {
      await redirectToLogin();
      throw await errorFrom(res);
    }
  }
  if (!res.ok) throw await errorFrom(res);
  return res;
}

export async function apiFetch<T>(path: string, opts: FetchOpts = {}): Promise<T> {
  const res = await apiResponse(path, opts);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

/** Saves a file answer (such as a CSV export) under the name the server gave. */
export async function apiDownload(path: string, fallbackName: string): Promise<void> {
  const res = await apiResponse(path, {});
  const name = /filename="([^"]+)"/.exec(res.headers.get("content-disposition") ?? "")?.[1] ?? fallbackName;
  const url = URL.createObjectURL(await res.blob());
  const a = Object.assign(document.createElement("a"), { href: url, download: name });
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}

/** POST to the auth BFF (login, register, activate, password reset). */
export async function authPost<T = unknown>(action: string, json: unknown): Promise<T> {
  const res = await fetch(`/api/auth/${action}`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify(json),
  });
  if (!res.ok) throw await errorFrom(res);
  return res.status === 204 ? (undefined as T) : ((await res.json()) as T);
}
