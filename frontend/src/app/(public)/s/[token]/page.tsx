"use client";

import { useParams } from "next/navigation";
import { PublicLinkPage } from "@/components/public-link";

/** A collection's read-only list: anyone with the link reads the records. */
export default function PublicListPage() {
  const { token } = useParams<{ token: string }>();
  return <PublicLinkPage token={token} kind="view" />;
}
