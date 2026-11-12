"use client";

import { useEffect } from "react";

/**
 * Asks before leaving while `dirty`: tab close or reload, and clicks on in-app links.
 * shortcut: browser back/forward inside the app is not caught, the App Router has no blocking API.
 */
export function useUnsavedGuard(dirty: boolean, message: string) {
  useEffect(() => {
    if (!dirty) return;
    const unload = (e: BeforeUnloadEvent) => e.preventDefault();
    const click = (e: MouseEvent) => {
      const a = (e.target as Element | null)?.closest("a[href]");
      if (!a || a.getAttribute("target") === "_blank" || e.metaKey || e.ctrlKey || e.shiftKey) return;
      if (!window.confirm(message)) {
        e.preventDefault();
        e.stopPropagation();
      }
    };
    window.addEventListener("beforeunload", unload);
    document.addEventListener("click", click, true);
    return () => {
      window.removeEventListener("beforeunload", unload);
      document.removeEventListener("click", click, true);
    };
  }, [dirty, message]);
}
