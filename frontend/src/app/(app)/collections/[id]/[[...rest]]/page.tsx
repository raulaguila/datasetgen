"use client";

import { useParams } from "next/navigation";
import { AddressRedirect } from "@/components/address-redirect";
import { collections } from "@/lib/api/endpoints";
import { collectionPath } from "@/lib/paths";

/** /collections/<id>/<section>: the address before collections lived at /<owner>/<slug>. */
export default function CollectionByIDPage() {
  const { id, rest } = useParams<{ id: string; rest?: string[] }>();
  const section = rest?.join("/");
  return <AddressRedirect queryKey={["collection", id, section]} find={async () => collectionPath(await collections.get(id), section)} />;
}
