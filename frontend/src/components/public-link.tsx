"use client";

import { useMemo, useRef, useState } from "react";
import dynamic from "next/dynamic";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type Form from "@rjsf/core";
import { CheckCircle2, ChevronLeft, ChevronRight, LinkIcon, Loader2 } from "lucide-react";
import { FormError, useDocumentTitle } from "@/components/bits";
import { EmptyState } from "@/components/empty-state";
import { fieldsOf, typeOf, type FieldRow } from "@/components/field-builder";
import { PublicControls } from "@/components/public-controls";
import { pickColumns } from "@/components/record-grid";
import { ListSkeleton } from "@/components/skeletons";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ApiError } from "@/lib/api/client";
import { publicLinks } from "@/lib/api/endpoints";
import type { PublicLink, SchemaIssue } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { useI18n } from "@/lib/i18n/provider";

const SchemaForm = dynamic(() => import("@/components/schema-form").then((m) => m.SchemaForm), { ssr: false, loading: () => <ListSkeleton rows={2} /> });

/**
 * The fields a stranger can fill: reference and file fields need a session
 * (picking a record, uploading), so the public form leaves them out, required
 * or not; the server takes the record without them and Quality lists it.
 */
function fillable(schema: Record<string, unknown>): Record<string, unknown> {
  const props = { ...((schema.properties ?? {}) as Record<string, unknown>) };
  for (const [key, p] of Object.entries(props)) {
    const type = typeOf(p);
    if (type === "reference" || type === "references" || type === "file" || type === "files") delete props[key];
  }
  const required = Array.isArray(schema.required) ? (schema.required as string[]).filter((k) => k in props) : undefined;
  return { ...schema, properties: props, ...(required ? { required } : {}) };
}

function Shell({ title, description, children }: { title?: string; description?: string; children: React.ReactNode }) {
  useDocumentTitle(title);
  return (
    <main className="mx-auto w-full max-w-3xl flex-1 space-y-6 px-4 py-8">
      <div className="flex items-start justify-between gap-4">
        <div>
          {title && <h1 className="font-heading text-2xl font-semibold tracking-tight">{title}</h1>}
          {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
        </div>
        <PublicControls />
      </div>
      {children}
    </main>
  );
}

/** Loads a link; a wrong, deleted or other-kind link reads as not found. */
export function PublicLinkPage({ token, kind }: { token: string; kind: PublicLink["kind"] }) {
  const { t } = useI18n();
  const link = useQuery({ queryKey: ["public", token], queryFn: () => publicLinks.get(token), retry: false });
  if (link.isPending) return <Shell><ListSkeleton rows={3} /></Shell>;
  if (link.isError || link.data.kind !== kind) {
    const missing = link.isError ? link.error instanceof ApiError && link.error.status === 404 : true;
    return (
      <Shell>
        <EmptyState icon={LinkIcon} title={t(missing ? "share.not_found" : "error.load_desc")} description={missing ? t("share.not_found_desc") : undefined} />
      </Shell>
    );
  }
  return kind === "form" ? <PublicForm token={token} link={link.data} /> : <PublicList token={token} link={link.data} />;
}

function PublicForm({ token, link }: { token: string; link: PublicLink }) {
  const { t } = useI18n();
  const schema = useMemo(() => fillable(link.schema), [link.schema]);
  const formRef = useRef<Form>(null);
  const [data, setData] = useState<unknown>({});
  const [checked, setChecked] = useState(false);
  const [issues, setIssues] = useState<SchemaIssue[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);
  const [sent, setSent] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setIssues([]);
    setChecked(true);
    if (!formRef.current?.validateForm()) return setError(t("records.invalid"));
    setError(null);
    setPending(true);
    try {
      await publicLinks.submit(token, data);
      setSent(true);
    } catch (err) {
      const found = err instanceof ApiError && err.code === "VALIDATION_FAILED" ? (err.details?.issues as SchemaIssue[] | undefined) : undefined;
      setIssues(found ?? []);
      setError(found ? t("records.invalid") : errorMessage(err, "common.save_failed", t));
    } finally {
      setPending(false);
    }
  };

  if (sent) {
    return (
      <Shell title={link.name}>
        <div role="status" className="space-y-4 rounded-lg border p-6 text-center">
          <CheckCircle2 className="mx-auto size-8 text-success" aria-hidden />
          <p className="font-medium">{t("share.sent")}</p>
          <Button variant="outline" onClick={() => { setData({}); setChecked(false); setSent(false); }}>{t("share.send_another")}</Button>
        </div>
      </Shell>
    );
  }
  return (
    <Shell title={link.name} description={[link.description?.trim(), t("records.required_legend")].filter(Boolean).join(" ")}>
      <form noValidate onSubmit={submit} className="space-y-4 rounded-lg border p-4 sm:p-6">
        <SchemaForm schema={schema} data={data} onChange={setData} formRef={formRef} liveValidate={checked} issues={issues} idPrefix="pub" />
        <FormError message={error} />
        <Button type="submit" disabled={pending}>{pending && <Loader2 className="size-4 animate-spin" />}{t("share.send")}</Button>
      </form>
    </Shell>
  );
}

const LIMIT = 25;

function PublicList({ token, link }: { token: string; link: PublicLink }) {
  const { t, locale } = useI18n();
  const [page, setPage] = useState(1);
  const [q, setQ] = useState("");
  const search = useDebounced(q.trim(), 300);
  const columns = useMemo(() => {
    const rows = fieldsOf(link.schema as never).filter((r) => !["file", "files", "reference", "references"].includes(r.field.type));
    return pickColumns(rows, "").length ? pickColumns(rows, "") : rows.slice(0, 4);
  }, [link.schema]);
  const list = useQuery({
    queryKey: ["public", token, "records", page, search],
    queryFn: ({ signal }) => publicLinks.records(token, { page, limit: LIMIT, search: search || undefined }, signal),
    placeholderData: keepPreviousData,
  });
  const show = (v: unknown, c: FieldRow) => {
    if (v === undefined || v === null || v === "") return "—";
    if (typeof v === "boolean") return t(v ? "common.yes" : "common.no");
    if (c.field.type === "number" || c.field.type === "integer") return typeof v === "number" ? v.toLocaleString(locale) : String(v);
    return typeof v === "object" ? JSON.stringify(v) : String(v);
  };
  const pages = list.data?.pagination.total_pages ?? 1;
  return (
    <Shell title={link.name} description={link.description}>
      {link.searchable && (
        <Input type="search" aria-label={t("common.search")} placeholder={t("common.search")} value={q} className="max-w-sm"
          onChange={(e) => { setQ(e.target.value); setPage(1); }} />
      )}
      {list.isPending ? <ListSkeleton rows={4} /> : list.isError ? <FormError message={errorMessage(list.error, "error.load_desc", t)} /> : list.data.items.length === 0 ? (
        <EmptyState icon={LinkIcon} title={t("common.no_results")} />
      ) : (
        <div className="overflow-x-auto rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>{columns.map((c) => <TableHead key={c.key}>{c.field.name}</TableHead>)}</TableRow>
            </TableHeader>
            <TableBody>
              {list.data.items.map((r) => {
                const data = (r.data ?? {}) as Record<string, unknown>;
                return (
                  <TableRow key={r.id}>
                    {columns.map((c) => <TableCell key={c.key} className="max-w-64 truncate">{show(data[c.key], c)}</TableCell>)}
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
      {pages > 1 && (
        <nav className="flex items-center justify-end gap-2 text-sm" aria-label={t("share.pages")}>
          <Button variant="outline" size="icon-sm" aria-label={t("share.prev")} disabled={page <= 1} onClick={() => setPage(page - 1)}><ChevronLeft className="size-4" /></Button>
          <span>{page} / {pages}</span>
          <Button variant="outline" size="icon-sm" aria-label={t("share.next")} disabled={page >= pages} onClick={() => setPage(page + 1)}><ChevronRight className="size-4" /></Button>
        </nav>
      )}
    </Shell>
  );
}
