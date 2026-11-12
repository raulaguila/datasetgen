import { cn } from "@/lib/utils";

/** A stack of data disks; inherits the current color. */
export function BrandMark({ className, size = 20 }: { className?: string; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 48 48"
      fill="currentColor"
      xmlns="http://www.w3.org/2000/svg"
      className={cn("shrink-0", className)}
      aria-hidden
    >
      <path
        fillRule="evenodd"
        d="M24 7c8.8 0 16 2.7 16 6s-7.2 6-16 6-16-2.7-16-6 7.2-6 16-6ZM8 16c0 3.3 7.2 6 16 6s16-2.7 16-6v8c0 3.3-7.2 6-16 6s-16-2.7-16-6Zm0 11c0 3.3 7.2 6 16 6s16-2.7 16-6v8c0 3.3-7.2 6-16 6s-16-2.7-16-6Z"
      />
    </svg>
  );
}
