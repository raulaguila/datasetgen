"use client";

import { useEffect } from "react";
import { refreshAccessToken } from "@/lib/api/session-refresh";
import { redirectToLogin } from "@/lib/api/client";

// Proactive silent session refresh. The access token lives ~15 min; without
// this an idle-but-open tab only refreshes reactively (a 401 then a retry) on
// the next action. Here we refresh ahead of expiry so the token is always warm.
//
// Cross-tab safety: refresh tokens are single-use (the backend revokes the old
// one on rotation), so two tabs refreshing at once would make one of them send a
// revoked token and get logged out. We single-flight with the Web Locks API and
// a shared localStorage timestamp so only one tab refreshes per window.
const REFRESH_MS = 12 * 60 * 1000; // < 15 min access TTL
const MIN_GAP_MS = 8 * 60 * 1000; // skip if another tab refreshed this recently
const STAMP_KEY = "session_refresh_at";

export function SessionKeepalive() {
  useEffect(() => {
    let stopped = false;

    const lastRefresh = () => Number(localStorage.getItem(STAMP_KEY) ?? "0");

    const doRefresh = async () => {
      try {
        const r = await refreshAccessToken();
        if (r.ok) {
          localStorage.setItem(STAMP_KEY, String(Date.now()));
        } else if (r.status === 401 && !location.pathname.startsWith("/login")) {
          await redirectToLogin();
        }
      } catch {
        // Network blip — leave it to the next tick / the reactive proxy retry.
      }
    };

    const maybeRefresh = async () => {
      if (stopped) return;
      if (Date.now() - lastRefresh() < MIN_GAP_MS) return; // another tab handled it
      // Single-flight across tabs: only the lock holder refreshes; others skip.
      const locks = navigator.locks;
      if (locks?.request) {
        await locks.request("session-refresh", { ifAvailable: true }, async (lock) => {
          if (!lock) return;
          if (Date.now() - lastRefresh() < MIN_GAP_MS) return; // re-check under lock
          await doRefresh();
        });
      } else {
        await doRefresh();
      }
    };

    const id = window.setInterval(() => void maybeRefresh(), REFRESH_MS);
    // Refresh when returning to a backgrounded tab (timers throttle while hidden).
    const onVisible = () => {
      if (document.visibilityState === "visible") void maybeRefresh();
    };
    document.addEventListener("visibilitychange", onVisible);

    return () => {
      stopped = true;
      window.clearInterval(id);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, []);

  return null;
}
