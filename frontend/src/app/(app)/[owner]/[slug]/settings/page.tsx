import { redirect } from "next/navigation";

/** The old Settings page: name, description and icon now edit in a dialog from the overview. */
export default async function CollectionSettingsPage({ params }: { params: Promise<{ owner: string; slug: string }> }) {
  const { owner, slug } = await params;
  redirect(`/${owner}/${slug}/overview`);
}
