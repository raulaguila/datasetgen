"use client";

import { CollectionForm, CollectionPage } from "../_parts";

export default function CollectionFieldsPage() {
  return <CollectionPage section="collections.tab.fields" description="collections.fields_desc">{(collection) => <CollectionForm key={collection.id} collection={collection} parts={["fields"]} />}</CollectionPage>;
}
