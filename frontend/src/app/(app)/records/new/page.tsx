"use client";

import { useSearchParams } from "next/navigation";
import { AddressRedirect } from "@/components/address-redirect";
import { collections } from "@/lib/api/endpoints";
import { recordPath } from "@/lib/paths";

/** /records/new?collection=<id>: the new-record form of that collection. */
export default function NewRecordByIDPage() {
  const id = useSearchParams().get("collection") ?? "";
  return <AddressRedirect queryKey={["new-record", id]} find={async () => recordPath(await collections.get(id), "new")} />;
}
