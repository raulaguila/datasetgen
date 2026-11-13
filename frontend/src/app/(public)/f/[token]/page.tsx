"use client";

import { useParams } from "next/navigation";
import { PublicLinkPage } from "@/components/public-link";

/** A collection's public form: anyone with the link adds a record. */
export default function PublicFormPage() {
  const { token } = useParams<{ token: string }>();
  return <PublicLinkPage token={token} kind="form" />;
}
