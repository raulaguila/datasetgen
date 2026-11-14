import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

/** Substring search runs on a trigram index, which needs 3 characters; a shorter term searches nothing yet. */
export const MIN_SEARCH = 3;
export const searchTerm = (q: string) => (q.trim().length >= MIN_SEARCH ? q.trim() : "");

/** Only a path on this site; "//evil.com" and "https://..." go home instead. */
export function safeNext(raw: string | null): string {
  return raw && raw.startsWith("/") && !raw.startsWith("//") && !raw.startsWith("/\\") ? raw : "/";
}

/** A random id; `crypto.randomUUID` exists only on HTTPS or localhost, getRandomValues everywhere. */
export const newId = () => Array.from(crypto.getRandomValues(new Uint8Array(16)), (b) => b.toString(16).padStart(2, "0")).join("");

/** Copies text; `navigator.clipboard` exists only on HTTPS or localhost, so plain HTTP falls back to a hidden field. */
export async function copyText(text: string) {
  if (navigator.clipboard) return navigator.clipboard.writeText(text);
  const el = document.createElement("textarea");
  el.value = text;
  el.style.position = "fixed";
  el.style.opacity = "0";
  document.body.appendChild(el);
  el.select();
  const ok = document.execCommand("copy");
  el.remove();
  if (!ok) throw new Error("copy failed");
}
