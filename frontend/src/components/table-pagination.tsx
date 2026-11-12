"use client";

// Footer for server-paginated tables. Maps to the backend's ?limit&offset.
// Layout: "Linhas [10▾]  ·  21–30 de 142  ·  ‹ 3 / 15 ›"
import { ChevronLeft, ChevronRight } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";
import { useI18n } from "@/lib/i18n/provider";

export const PAGE_SIZE_OPTIONS = [10, 25, 50, 100]; // the API caps a page at 100

export function TablePagination({
  page,
  pageSize,
  totalItems,
  onPageChange,
  onPageSizeChange,
  className,
}: {
  page: number; // 1-based
  pageSize: number;
  totalItems: number;
  onPageChange: (p: number) => void;
  onPageSizeChange: (n: number) => void;
  className?: string;
}) {
  const { t } = useI18n();
  if (totalItems === 0) return null;
  const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
  const startIndex = (page - 1) * pageSize;
  const endIndex = Math.min(startIndex + pageSize, totalItems);
  const canPrev = page > 1;
  const canNext = page < totalPages;

  return (
    <div
      className={cn(
        "flex flex-wrap items-center gap-x-3 gap-y-2 border-t bg-muted px-3 py-2 text-xs text-muted-foreground",
        className,
      )}
    >
      <div className="flex items-center gap-2">
        <span>{t("table.rows")}</span>
        <Select
          value={String(pageSize)}
          items={Object.fromEntries(PAGE_SIZE_OPTIONS.map((n) => [String(n), String(n)]))}
          onValueChange={(v) => onPageSizeChange(Number(v))}
        >
          <SelectTrigger size="sm" className="w-20" aria-label={t("table.rows")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PAGE_SIZE_OPTIONS.map((n) => (
              <SelectItem key={n} value={String(n)}>
                {n}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className="font-mono tabular-nums">
        {startIndex + 1}–{endIndex} {t("table.of")} {totalItems}
      </div>

      <div className="hidden flex-1 sm:block" />

      <div className="flex items-center gap-1">
        <button
          onClick={() => canPrev && onPageChange(page - 1)}
          disabled={!canPrev}
          aria-label={t("table.prev")}
          className={cn(
            "flex size-11 items-center md:size-[var(--control-h-sm)] justify-center rounded-md border",
            canPrev ? "hover:border-ring hover:text-foreground" : "cursor-not-allowed opacity-40",
          )}
        >
          <ChevronLeft className="size-3.5" />
        </button>
        <span className="px-1 font-mono tabular-nums">
          {page} / {totalPages}
        </span>
        <button
          onClick={() => canNext && onPageChange(page + 1)}
          disabled={!canNext}
          aria-label={t("table.next")}
          className={cn(
            "flex size-11 items-center md:size-[var(--control-h-sm)] justify-center rounded-md border",
            canNext ? "hover:border-ring hover:text-foreground" : "cursor-not-allowed opacity-40",
          )}
        >
          <ChevronRight className="size-3.5" />
        </button>
      </div>
    </div>
  );
}

/**
 * The page to move to when this one came back empty while earlier pages still
 * have rows, as after deleting the last rows of the last page; null otherwise.
 */
export function emptyPageFallback(page: number, limit: number, data?: { items: unknown[]; pagination: { total_items: number } }): number | null {
  if (!data || data.items.length > 0 || page <= 1) return null;
  const last = Math.max(1, Math.ceil(data.pagination.total_items / limit));
  return last < page ? last : null;
}
