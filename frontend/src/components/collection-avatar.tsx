import { avatarStyle, initials } from "@/components/bits";
import { COLLECTION_ICONS } from "@/lib/collection-icons";
import { cn } from "@/lib/utils";

/** A collection's chosen icon, or its initials, on its own tint. */
export function CollectionAvatar({ collection, className }: { collection: { id: string; name: string; icon?: string }; className?: string }) {
  const Icon = collection.icon ? COLLECTION_ICONS[collection.icon] : undefined;
  return (
    <span className={cn("grid shrink-0 place-items-center", className)} style={avatarStyle(collection.id)} aria-hidden>
      {Icon ? <Icon className="size-[55%]" /> : initials(collection.name)}
    </span>
  );
}
