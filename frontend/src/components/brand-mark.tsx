import { cn } from "@/lib/utils";

/** Shield with a keyhole; inherits the current color. */
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
        d="M24 3 6 10v12c0 11.5 7.7 20.6 18 23 10.3-2.4 18-11.5 18-23V10L24 3Zm0 13a5 5 0 0 0-2.5 9.33V33h5v-7.67A5 5 0 0 0 24 16Z"
      />
    </svg>
  );
}
