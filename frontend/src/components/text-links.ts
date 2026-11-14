"use client";

import { useQueries, useQuery } from "@tanstack/react-query";
import { linkKey } from "@/components/markdown";
import { collections, records } from "@/lib/api/endpoints";
import type { DataRecord } from "@/lib/api/types";

/** The bytes a rule's value may take (the API allows 2000). */
const RULE_BYTES = 1800;

/** The distinct http(s) targets of the Markdown links in a record's text values, nested ones too. */
export function linkTargets(value: unknown): string[] {
  const out = new Set<string>();
  const walk = (v: unknown) => {
    if (typeof v === "string") for (const m of v.matchAll(/\]\((https?:\/\/[^)\s]+)\)/g)) out.add(linkKey(m[1]));
    else if (Array.isArray(v)) v.forEach(walk);
    else if (v && typeof v === "object") Object.values(v).forEach(walk);
  };
  walk(value);
  return [...out];
}

/** Values split into lists that each fit one rule. */
export function chunks(values: string[], bytes = RULE_BYTES): string[][] {
  const out: string[][] = [];
  for (const v of values) {
    const last = out[out.length - 1];
    if (last && JSON.stringify([...last, v]).length <= bytes) last.push(v);
    else out.push([v]);
  }
  return out;
}

/**
 * Links in a record's text that point at the page another record stands for (its link field
 * holds that address), as address -> record id: the record view opens those records in the app.
 * Looks in the collections the sidebar lists.
 */
export function useTextLinks(record: DataRecord | undefined): Map<string, string> {
  const targets = record ? linkTargets(record.data) : [];
  const list = useQuery({
    queryKey: ["collections", "picker"],
    queryFn: ({ signal }) => collections.list({ page: 1, limit: 100 }, signal),
    staleTime: 30_000,
    enabled: targets.length > 0,
  });
  const ids = targets.length ? (list.data?.items ?? []).map((c) => c.id) : [];
  const metas = useQueries({ queries: ids.map((id) => ({ queryKey: ["collections", "detail", id], queryFn: () => collections.get(id), staleTime: 60_000 })) });
  // Each collection's link fields, one lookup per field.
  const lookups = metas.flatMap((m) => {
    const c = m.data;
    const props = (c?.schema?.properties ?? {}) as Record<string, { format?: string }>;
    return c ? Object.entries(props).filter(([, p]) => p?.format === "uri").map(([key]) => ({ collection: c.id, key })) : [];
  });
  const parts = chunks(targets);
  const found = useQueries({
    queries: lookups.map(({ collection, key }) => ({
      queryKey: ["records", collection, "links", key, targets],
      // shortcut: one page of 100 matches per field, raise if a text links to more records of one collection.
      queryFn: ({ signal }: { signal: AbortSignal }) => records.list({
        collection_id: collection, page: 1, limit: 100,
        where: JSON.stringify({ any: true, rules: parts.slice(0, 20).map((value) => ({ field: key, op: "in", value })) }),
      }, signal),
      staleTime: 60_000,
      retry: false,
    })),
  });
  const map = new Map<string, string>();
  found.forEach((q, i) => {
    for (const r of q.data?.items ?? []) {
      const v = (r.data as Record<string, unknown> | null)?.[lookups[i].key];
      if (typeof v === "string" && r.id !== record?.id) map.set(linkKey(v), r.id);
    }
  });
  return map;
}
