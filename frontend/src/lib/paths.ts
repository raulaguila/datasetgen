/** Where a collection lives in the app: /<owner>/<slug>, as repositories do on GitHub. */
export type At = { owner: string; slug: string };
/** A collection as the API calls need it: its address, and its id for caches and ids in data. */
export type CollectionRef = At & { id: string };

/** The collection's page, or one of its sections ("fields", "settings", ...). */
export function collectionPath(c: At, section?: string): string {
  return `/${c.owner}/${c.slug}${section ? `/${section}` : ""}`;
}

/** A record of the collection; "new" opens the empty form. */
export function recordPath(c: At, recordId: string, edit = false): string {
  return `${collectionPath(c, `r/${recordId}`)}${edit ? "/edit" : ""}`;
}

/** Old links that only know an id; their pages look the address up and move on. */
export const collectionByID = (id: string, section?: string) => `/collections/${id}${section ? `/${section}` : ""}`;
export const recordByID = (id: string) => `/records/${id}`;

/** pathname with its /<owner>/<slug> prefix swapped for the collection's current one. */
export function movedPath(pathname: string, from: At, to: At): string {
  const prefix = collectionPath(from);
  return pathname.toLowerCase().startsWith(prefix.toLowerCase()) ? collectionPath(to) + pathname.slice(prefix.length) : pathname;
}

// The app's own top-level pages; usernames cannot take these names (domain.ValidUsername).
const APP_ROOTS = new Set(["admin", "api-access", "collections", "notifications", "oauth", "profile", "records"]);

/** The collection address a pathname is under: /<owner>/<slug>/..., else undefined. */
export function collectionInPath(pathname: string): At | undefined {
  const m = /^\/([^/]+)\/([^/]+)/.exec(pathname);
  return m && !APP_ROOTS.has(m[1]) ? { owner: decodeURIComponent(m[1]).toLowerCase(), slug: decodeURIComponent(m[2]).toLowerCase() } : undefined;
}
