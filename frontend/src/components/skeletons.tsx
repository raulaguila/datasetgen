import { Card } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";

// Stacked rows placeholder for a compact list (e.g. dashboard KB list).
export function ListSkeleton({ rows = 5 }: { rows?: number }) {
  return (
    <Card className="divide-y gap-0 p-0">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="px-4 py-3">
          <Skeleton className="h-4 w-1/2" />
        </div>
      ))}
    </Card>
  );
}
