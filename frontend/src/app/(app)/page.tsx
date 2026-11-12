"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { ListSkeleton } from "@/components/skeletons";
import { collections } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { useUIStore } from "@/lib/store/ui";

/**
 * There is no home page: the app opens inside a collection, as the reference app
 * opens inside a workspace. The last one used, else the first, else the list.
 */
export default function Home() {
  const router = useRouter();
  const { user, loading } = useMe();
  const storedId = useUIStore((s) => s.collectionId);
  // Same query as the sidebar, so the redirect costs no extra request.
  const list = useQuery({ queryKey: ["collections", "picker"], queryFn: ({ signal }) => collections.list({ page: 1, limit: 100 }, signal), enabled: !!user, staleTime: 30_000 });

  useEffect(() => {
    if (loading || !user) return;
    if (list.isPending) return;
    const items = list.data?.items ?? [];
    const target = items.find((c) => c.id === storedId) ?? items[0];
    router.replace(target ? `/collections/${target.id}` : "/collections");
  }, [loading, user, list.isPending, list.data, storedId, router]);

  return <div className="p-[var(--space-page)]"><ListSkeleton rows={3} /></div>;
}
