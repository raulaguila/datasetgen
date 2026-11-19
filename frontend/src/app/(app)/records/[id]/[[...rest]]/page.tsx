"use client";

import { useParams } from "next/navigation";
import { AddressRedirect } from "@/components/address-redirect";
import { collections, records } from "@/lib/api/endpoints";
import { recordPath } from "@/lib/paths";

/** /records/<id>[/edit]: links that know only the record, such as notifications and the API. */
export default function RecordByIDPage() {
  const { id, rest } = useParams<{ id: string; rest?: string[] }>();
  const edit = rest?.[0] === "edit";
  return (
    <AddressRedirect
      queryKey={["record", id, edit]}
      find={async () => recordPath(await collections.get((await records.get(id)).collection_id), id, edit)}
    />
  );
}
