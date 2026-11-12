"use client";

// One rotation at a time: refresh tokens are single use, so the keepalive and a
// retrying request must share it. Each caller gets its own copy of the answer.
let pending: Promise<Response> | null = null;

export async function refreshAccessToken(): Promise<Response> {
  pending ??= fetch("/api/auth/refresh", { method: "POST", cache: "no-store" }).finally(() => {
    pending = null;
  });
  return (await pending).clone();
}
