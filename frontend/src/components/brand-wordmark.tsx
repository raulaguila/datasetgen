import { BrandMark } from "@/components/brand-mark";
import { APP_NAME } from "@/lib/brand";
import { cn } from "@/lib/utils";

/** Mark and product name; the name is text, so a renamed project needs no new artwork. */
export function BrandWordmark({ className, height = 40 }: { className?: string; height?: number }) {
  return (
    <span
      className={cn("inline-flex shrink-0 items-center gap-2 font-heading font-semibold tracking-tight", className)}
      style={{ fontSize: Math.round(height * 0.55), height }}
    >
      <BrandMark size={Math.round(height * 0.8)} className="text-primary" />
      {APP_NAME}
    </span>
  );
}
