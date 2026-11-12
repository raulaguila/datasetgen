"use client";

import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Bell, CheckCheck, Info, OctagonAlert } from "lucide-react";
import { UNREAD_KEY } from "@/components/app-shell";
import { PageBody, PageHeader, useFormatDate } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { QueryErrorState } from "@/components/query-error-state";
import { ListSkeleton } from "@/components/skeletons";
import { TablePagination, emptyPageFallback } from "@/components/table-pagination";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { notifications } from "@/lib/api/endpoints";
import type { Notification } from "@/lib/api/types";
import { useGoToPage, useListParams } from "@/lib/hooks/use-list-params";
import type { MessageKey } from "@/lib/i18n/messages";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";

const ICON: Record<Notification["level"], { icon: typeof Info; cls: string }> = {
  info: { icon: Info, cls: "bg-primary/10 text-primary" },
  warning: { icon: AlertTriangle, cls: "bg-amber-500/15 text-amber-700 dark:text-amber-300" },
  critical: { icon: OctagonAlert, cls: "bg-destructive/10 text-destructive" },
};

/** The text in the user's language when the kind is known; the server's English otherwise. */
function useNotificationText() {
  const { t } = useI18n();
  const format = useFormatDate();
  return (n: Notification) => {
    if (!n.kind) return { title: n.title, message: n.message };
    const role = n.params?.role;
    const params: Record<string, string> = {
      ...n.params,
      until: n.params?.until ? format(n.params.until) : "",
      role: role === "owner" || role === "editor" || role === "viewer" ? t(`members.role.${role}`) : role ?? "",
    };
    const fill = (key: string) => t(key as MessageKey).replace(/\{(\w+)\}/g, (_, k: string) => params[k] ?? "");
    return { title: fill(`notifications.kind.${n.kind}.title`), message: fill(`notifications.kind.${n.kind}.message`) };
  };
}

export default function NotificationsPage() {
  const { t } = useI18n();
  const format = useFormatDate();
  const text = useNotificationText();
  const client = useQueryClient();
  const [{ page, limit }, set] = useListParams({ page: 1, limit: 25 });
  const list = useQuery({
    queryKey: ["notifications", page, limit],
    queryFn: ({ signal }) => notifications.list(page, limit, signal),
    placeholderData: keepPreviousData,
  });
  const fallback = list.isPlaceholderData ? null : emptyPageFallback(page, limit, list.data);
  useGoToPage(fallback, set); // the last rows of the last page went away
  const unread = useQuery({ queryKey: UNREAD_KEY, queryFn: notifications.unread }); // shared with the bell
  const markRead = useMutation({
    mutationFn: notifications.markRead,
    onSuccess: () => void client.invalidateQueries({ queryKey: ["notifications"] }),
  });

  return (
    <>
      <PageHeader
        title={t("nav.notifications")}
        description={t("notifications.subtitle")}
        action={(unread.data?.unread ?? 0) > 0 && (
          <Button variant="outline" disabled={markRead.isPending} onClick={() => markRead.mutate(undefined)}>
            <CheckCheck className="size-4" />{t("notifications.mark_all_read")}
          </Button>
        )}
      />
      <PageBody>
        {list.isPending ? (
          <ListSkeleton />
        ) : list.isError ? (
          <QueryErrorState error={list.error} onRetry={() => void list.refetch()} />
        ) : list.data.items.length === 0 ? (
          <EmptyState icon={Bell} title={t("notifications.empty")} description={t("notifications.empty_desc")} />
        ) : (
          <Card className="gap-0 overflow-hidden p-0">
            <ul className="divide-y">
              {list.data.items.map((n) => {
                const { icon: Icon, cls } = ICON[n.level];
                const { title, message } = text(n);
                return (
                  <li key={n.id} className={cn("flex flex-wrap gap-x-3 border-l-2 border-transparent px-4 py-3", !n.read_at && "border-l-primary bg-primary/[0.05]")}>
                    <span className={cn("flex size-8 shrink-0 items-center justify-center rounded-full", cls)}>
                      <Icon className="size-4" />
                    </span>
                    <div className="min-w-0 flex-1 max-sm:basis-[calc(100%-2.75rem)]">
                      <div className="flex flex-wrap items-baseline gap-x-2">
                        {!n.read_at && <span className="size-2 self-center rounded-full bg-primary" role="img" aria-label={t("notifications.unread")} />}
                        <span className={cn("text-sm", !n.read_at && "font-semibold")}>{title}</span>
                        <span className="text-xs text-muted-foreground">{format(n.created_at)}</span>
                      </div>
                      <p className="mt-0.5 text-sm text-muted-foreground">{message}</p>
                    </div>
                    {!n.read_at && (
                      <Button variant="ghost" size="sm" className="max-sm:ml-9" disabled={markRead.isPending} onClick={() => markRead.mutate([n.id])}>
                        {t("notifications.mark_read")}
                      </Button>
                    )}
                  </li>
                );
              })}
            </ul>
            <TablePagination
              page={page}
              pageSize={limit}
              totalItems={list.data.pagination.total_items}
              onPageChange={(n) => set({ page: n })}
              onPageSizeChange={(n) => set({ limit: n, page: 1 })}
            />
          </Card>
        )}
      </PageBody>
    </>
  );
}
