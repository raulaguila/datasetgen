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
