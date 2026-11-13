"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { FormError, FormSection, FormSections, useFormatDate } from "@/components/bits";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { records } from "@/lib/api/endpoints";
import { useMe } from "@/lib/context/me";
import { errorMessage } from "@/lib/errors";
import { useI18n } from "@/lib/i18n/provider";

const MAX = 2000;

/** A record's comments: any member writes; the author or an owner deletes. */
export function RecordComments({ recordId, owner }: { recordId: string; owner: boolean }) {
  const { t } = useI18n();
  const { user } = useMe();
  const format = useFormatDate();
  const qc = useQueryClient();
  const key = ["records", "comments", recordId];
  const list = useQuery({ queryKey: key, queryFn: () => records.comments(recordId) });
  const [body, setBody] = useState("");
  const add = useMutation({
    mutationFn: () => records.addComment(recordId, body.trim()),
    onSuccess: () => { setBody(""); void qc.invalidateQueries({ queryKey: key }); },
  });
  const remove = useMutation({
    mutationFn: (commentId: string) => records.deleteComment(recordId, commentId),
    onSuccess: () => void qc.invalidateQueries({ queryKey: key }),
    onError: (err) => toast.error(errorMessage(err, "common.save_failed", t)),
  });
  const items = list.data?.items ?? [];

  return (
    <FormSections>
      <FormSection title={t("comments.title")} description={t("comments.desc")}>
        <div className="space-y-4">
          {list.isError && <FormError message={errorMessage(list.error, "error.load_desc", t)} />}
          {items.length > 0 && (
            <ul className="space-y-3" aria-label={t("comments.title")}>
              {items.map((c) => (
                <li key={c.id} className="rounded-lg border p-3">
                  <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
                    <span><span className="font-medium text-foreground">{c.author_name ?? t("comments.gone")}</span> · {format(c.created_at)}</span>
                    {(owner || (!!c.author_id && c.author_id === user?.id)) && (
                      <Button variant="ghost" size="icon-sm" aria-label={t("comments.delete")} disabled={remove.isPending} onClick={() => remove.mutate(c.id)}>
                        <Trash2 className="size-4" />
                      </Button>
                    )}
                  </div>
                  <p className="mt-1 text-sm whitespace-pre-wrap break-words">{c.body}</p>
                </li>
              ))}
            </ul>
          )}
          <form className="space-y-2" onSubmit={(e) => { e.preventDefault(); if (body.trim()) add.mutate(); }}>
            <Textarea aria-label={t("comments.new")} placeholder={t("comments.new")} value={body} maxLength={MAX} rows={3} onChange={(e) => setBody(e.target.value)} />
            <FormError message={add.isError ? errorMessage(add.error, "common.save_failed", t) : null} />
            <Button type="submit" size="sm" disabled={add.isPending || !body.trim()}>
              {add.isPending && <Loader2 className="size-4 animate-spin" />}{t("comments.send")}
            </Button>
          </form>
        </div>
      </FormSection>
    </FormSections>
  );
}
