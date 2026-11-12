"use client";

import { CollectionForm, CollectionPage } from "../_parts";

export default function CollectionFieldsPage() {
  return <CollectionPage section="collections.tab.fields">{(collection) => <CollectionForm key={collection.id} collection={collection} parts={["fields"]} />}</CollectionPage>;
}
