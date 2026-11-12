"use client";

import { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { collections } from "@/lib/api/endpoints";
import type { Collection } from "@/lib/api/types";

const qualityKey = (id: string) => ["collections", "quality", id];

/** The latest check, polled while it runs; a missing or stale check starts once on open. */
export function useQuality(collection: Collection, enabled = true) {
  const client = useQueryClient();
  const q = useQuery({
    queryKey: qualityKey(collection.id),
    queryFn: ({ signal }) => collections.quality(collection.id, signal),
    enabled,
    refetchInterval: (query) => (query.state.data?.status === "running" ? 1500 : false),
  });
  const start = useMutation({
    mutationFn: () => collections.startQuality(collection.id),
    onSuccess: (r) => client.setQueryData(qualityKey(collection.id), r),
  });
  const r = q.data;
  const stale = !!r && r.status !== "running" && (r.status === "none" || r.version < r.current_version);
  const started = useRef(false);
  useEffect(() => {
    if (enabled && stale && !started.current) {
      started.current = true;
      start.mutate();
    }
  }, [enabled, stale, start]);
  return { q, start, stale };
}

