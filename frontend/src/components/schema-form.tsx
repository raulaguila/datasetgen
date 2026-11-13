"use client";

import Form from "@rjsf/core";
import type { ErrorSchema, FieldProps, IconButtonProps, RJSFSchema, UiSchema, ValidatorType } from "@rjsf/utils";
import Link from "next/link";
import { useMemo, useRef, useState, type Ref } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Copy, FileUp, Loader2, Paperclip, Plus, Search, X } from "lucide-react";
import { FILE, REF, recordTitle } from "@/components/field-builder";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { collections, records, tools } from "@/lib/api/endpoints";
import type { FileRef, SchemaIssue } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { searchTerm } from "@/lib/utils";
import { useI18n } from "@/lib/i18n/provider";
import { LOCALES, messages, type Locale } from "@/lib/i18n/messages";
import { schemaValidator, type Translate } from "@/lib/schema-validator";

// The default theme draws glyphicon fonts this app does not load; these use lucide.
function iconButton(Icon: typeof Plus, variant: "outline" | "ghost" | "destructive" = "ghost") {
  return function SchemaIconButton({ onClick, disabled, title, className }: IconButtonProps) {
    return (
      <Button type="button" size="icon-sm" variant={variant} title={title} aria-label={title} disabled={disabled} className={className} onClick={onClick}>
        <Icon className="size-4" />
      </Button>
    );
  };
}

const templates = {
  ButtonTemplates: {
    AddButton: iconButton(Plus, "outline"),
    CopyButton: iconButton(Copy),
    MoveUpButton: iconButton(ArrowUp),
    MoveDownButton: iconButton(ArrowDown),
    RemoveButton: iconButton(X),
    ClearButton: iconButton(X),
  },
};

// One validator per language; the failed keyword picks the message (schema.error.*).
const translators = Object.fromEntries(
  LOCALES.map(({ value }) => [value, (keyword: string) => (messages[value] as Record<string, string>)[`schema.error.${keyword}`]]),
) as Record<Locale, Translate>;
const validators = Object.fromEntries(LOCALES.map(({ value }) => [value, schemaValidator(translators[value])])) as Record<Locale, ValidatorType>;

/** Turns the server's issues (JSON pointers) into errors shown under each field. */
export function issueErrors(issues: SchemaIssue[], translate: Translate = () => undefined): ErrorSchema {
  const root: Record<string, unknown> = {};
  for (const { path, keyword, message: raw } of issues) {
    const message = (keyword && translate(keyword)) || raw;
    let node = root;
    for (const part of path.split("/").slice(1)) {
      const key = part.replace(/~1/g, "/").replace(/~0/g, "~");
      node = (node[key] ??= {}) as Record<string, unknown>;
    }
    ((node.__errors ??= []) as string[]).push(message);
  }
  return root as ErrorSchema;
}

/**
 * A free object (no properties) typed as JSON. Text that does not parse is
 * handed on as a string, so validation refuses it instead of keeping an old value.
 */
function JsonField({ formData, onChange, fieldPathId, schema, name, required, disabled, readonly, uiSchema }: FieldProps) {
  const { t } = useI18n();
  // shortcut: the text is read once; a form reset from outside keeps it, fine while pages remount the form.
  const [text, setText] = useState(() => (formData === undefined ? "" : JSON.stringify(formData, null, 2)));
  const [bad, setBad] = useState(false);
  const id = fieldPathId.$id;
  const change = (v: string) => {
    setText(v);
    if (!v.trim()) { setBad(false); onChange(undefined, fieldPathId.path); return; }
    try { const parsed: unknown = JSON.parse(v); setBad(false); onChange(parsed, fieldPathId.path); }
    catch { setBad(true); onChange(v, fieldPathId.path); }
  };
  return (
    <>
      <label htmlFor={id} className="control-label">{schema.title ?? name}{required && <span className="required">*</span>}</label>
      {schema.description && <p id={`${id}__description`} className="field-description">{schema.description}</p>}
      <Textarea id={id} className="font-mono text-xs" rows={6} spellCheck={false} value={text} disabled={disabled || readonly}
        placeholder={typeof uiSchema?.["ui:placeholder"] === "string" ? uiSchema["ui:placeholder"] : "{ }"}
        aria-invalid={bad} aria-describedby={schema.description ? `${id}__description` : undefined} onChange={(e) => change(e.target.value)} />
      {bad && <p className="text-xs text-destructive">{t("schema.json_invalid")}</p>}
    </>
  );
}

/** The label and description every custom field draws itself (rjsf leaves them to it). */
function Head({ id, schema, name, required }: Pick<FieldProps, "schema" | "name" | "required"> & { id: string }) {
  return (
    <>
      <label id={`${id}__label`} htmlFor={id} className="control-label">{schema.title ?? name}{required && <span className="required">*</span>}</label>
      {schema.description && <p id={`${id}__description`} className="field-description">{schema.description}</p>}
    </>
  );
}

/** A button's name: the field's label, then what the button says (its visible text stays in the name). */
const named = (id: string) => ({ id, "aria-labelledby": `${id}__label ${id}` });

/** How a record of another collection reads: its first text field, or its id. */
export function useRecordLabel(collectionId: string | undefined, recordId: string | undefined) {
  const target = useQuery({ queryKey: ["collections", "detail", collectionId], queryFn: () => collections.get(collectionId!), enabled: !!collectionId && !!recordId });
  const rec = useQuery({ queryKey: ["records", "one", recordId], queryFn: () => records.get(recordId!), enabled: !!recordId, retry: false });
  if (!recordId) return undefined;
  return (rec.data && recordTitle(target.data?.schema ?? null, rec.data.data)) ?? recordId.slice(0, 8);
}

/** A record of the collection named by x-ref, picked by searching it. */
function ReferenceField({ formData, onChange, fieldPathId, schema, name, required, disabled, readonly }: FieldProps) {
  const { t } = useI18n();
  const target = typeof (schema as Record<string, unknown>)[REF] === "string" ? ((schema as Record<string, unknown>)[REF] as string) : "";
  const value = typeof formData === "string" && formData ? formData : undefined;
  const label = useRecordLabel(target, value);
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const q = searchTerm(useDebounced(text));
  const meta = useQuery({ queryKey: ["collections", "detail", target], queryFn: () => collections.get(target), enabled: open && !!target });
  const found = useQuery({ queryKey: ["records", target, "pick", q], queryFn: ({ signal }) => records.list({ collection_id: target, page: 1, limit: 8, search: q }, signal), enabled: open && !!target });
  const id = fieldPathId.$id;
  const off = disabled || readonly;
  const pick = (v: string | undefined) => { onChange(v, fieldPathId.path); setOpen(false); setText(""); };
  return (
    <>
      <Head id={id} schema={schema} name={name} required={required} />
      {value && !open ? (
        <div className="flex items-center gap-2">
          <Link href={`/records/${value}`} className="min-w-0 flex-1 truncate rounded-md border px-3 py-1.5 text-sm hover:underline">{label}</Link>
          {!off && <Button type="button" size="sm" variant="outline" onClick={() => setOpen(true)}>{t("schema.ref_change")}</Button>}
          {!off && <Button type="button" size="icon-sm" variant="ghost" aria-label={t("schema.ref_clear")} title={t("schema.ref_clear")} onClick={() => pick(undefined)}><X className="size-4" /></Button>}
        </div>
      ) : off ? (
        <p className="text-sm text-muted-foreground">—</p>
      ) : !open ? (
        <Button {...named(id)} type="button" variant="outline" onClick={() => setOpen(true)}><Search className="size-4" />{t("schema.ref_pick")}</Button>
      ) : (
        <div className="space-y-1.5 rounded-md border p-2">
          <Input id={id} autoFocus type="search" placeholder={t("schema.ref_search")} value={text} onChange={(e) => setText(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") setOpen(false); if (e.key === "Enter") e.preventDefault(); }} />
          {found.isPending ? <Loader2 className="mx-auto size-4 animate-spin text-muted-foreground" /> : (
            <ul className="max-h-56 overflow-auto">
              {(found.data?.items ?? []).map((r) => (
                <li key={r.id}>
                  <button type="button" className="w-full truncate rounded px-2 py-1.5 text-left text-sm hover:bg-muted" onClick={() => pick(r.id)}>
                    {recordTitle(meta.data?.schema ?? null, r.data) ?? r.id.slice(0, 8)}
                  </button>
                </li>
              ))}
              {found.data?.items.length === 0 && <li className="px-2 py-1.5 text-sm text-muted-foreground">{t("common.no_results")}</li>}
            </ul>
          )}
          <Button type="button" size="sm" variant="ghost" onClick={() => setOpen(false)}>{t("common.cancel")}</Button>
        </div>
      )}
    </>
  );
}

export const fileSize = (n: number) => (n < 1024 ? `${n} B` : n < 1024 * 1024 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1024 / 1024).toFixed(1)} MB`);
const FILE_MAX = 1024 * 1024;

/** An attached file: uploaded to the collection at once, the record keeps {id, name, size, type}. */
function FileField({ formData, onChange, fieldPathId, schema, name, required, disabled, readonly, registry }: FieldProps) {
  const { t } = useI18n();
  const collectionId = (registry.formContext as { collectionId?: string } | undefined)?.collectionId;
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const file = formData && typeof formData === "object" && typeof (formData as FileRef).id === "string" ? (formData as FileRef) : undefined;
  const id = fieldPathId.$id;
  const off = disabled || readonly || !collectionId;
  const upload = async (f: File) => {
    setError(null);
    if (f.size > FILE_MAX) return setError(t("schema.file_too_large"));
    setBusy(true);
    try {
      onChange(await tools.upload(collectionId!, f), fieldPathId.path);
    } catch (err) {
      setError(errorMessage(err, "common.save_failed", t));
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <Head id={id} schema={schema} name={name} required={required} />
      <div className="flex flex-wrap items-center gap-2">
        {file && (
          <a href={tools.fileUrl(file.id)} target="_blank" rel="noopener" className="inline-flex min-w-0 max-w-full items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm hover:underline">
            <Paperclip className="size-4 shrink-0" /><span className="truncate">{file.name}</span><span className="shrink-0 text-xs text-muted-foreground">{fileSize(file.size)}</span>
          </a>
        )}
        {!off && (
          <Button {...named(id)} type="button" variant="outline" size="sm" disabled={busy} onClick={() => input.current?.click()}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <FileUp className="size-4" />}{t(file ? "schema.file_replace" : "schema.file_pick")}
          </Button>
        )}
        {!off && file && <Button type="button" size="icon-sm" variant="ghost" aria-label={t("schema.file_remove")} title={t("schema.file_remove")} onClick={() => onChange(undefined, fieldPathId.path)}><X className="size-4" /></Button>}
        {!file && off && <span className="text-sm text-muted-foreground">{collectionId ? "—" : t("schema.file_preview")}</span>}
        <input ref={input} type="file" className="hidden" aria-hidden tabIndex={-1} onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ""; if (f) void upload(f); }} />
      </div>
      <p className="text-xs text-muted-foreground">{t("schema.file_hint")}</p>
      {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
    </>
  );
}

const fields = { json: JsonField, reference: ReferenceField, file: FileField };

/**
 * Text areas for strings allowed to run long (the field builder's "long text"),
 * the first example as the placeholder, and a JSON box for free objects below
 * the root, at any depth.
 */
export function uiFor(schema: unknown, nested = false): UiSchema {
  if (!schema || typeof schema !== "object") return {};
  const s = schema as { type?: unknown; maxLength?: unknown; examples?: unknown; properties?: Record<string, unknown>; items?: unknown; [REF]?: unknown; [FILE]?: unknown };
  if (nested && s.type === "string" && typeof s[REF] === "string") return { "ui:field": "reference" };
  if (nested && s.type === "object" && s[FILE] === true) return { "ui:field": "file" };
  const out: UiSchema = {};
  if (Array.isArray(s.examples) && typeof s.examples[0] === "string") out["ui:placeholder"] = s.examples[0];
  if (s.type === "string" && typeof s.maxLength === "number" && s.maxLength >= 500) out["ui:widget"] = "textarea";
  if (s.type === "string") return out;
  if (nested && s.type === "object" && s.properties === undefined) return { ...out, "ui:field": "json" };
  for (const [key, sub] of Object.entries(s.properties ?? {})) {
    const ui = uiFor(sub, true);
    if (Object.keys(ui).length) out[key] = ui;
  }
  const items = uiFor(Array.isArray(s.items) ? null : s.items, true);
  if (Object.keys(items).length) out.items = items;
  return out;
}

/**
 * A form generated from a collection's JSON Schema. Before saving, the page calls
 * formRef.current.validateForm() and turns on liveValidate: a re-render would
 * otherwise drop the errors. The server checks again; its issues come back
 * through `issues`.
 */
export function SchemaForm({ schema, data, disabled, onChange, idPrefix = "rec", formRef, issues, liveValidate, collectionId }: {
  schema: Record<string, unknown>;
  /** Where attached files go; without it (the fields preview) file fields only show. */
  collectionId?: string;
  data: unknown;
  disabled?: boolean;
  onChange?: (data: unknown) => void;
  idPrefix?: string;
  formRef?: Ref<Form>;
  /** Rules the server refused, shown under their fields. */
  issues?: SchemaIssue[];
  /** Keep errors on screen across re-renders, updating as the user types. */
  liveValidate?: boolean;
}) {
  const { locale } = useI18n();
  // The page has its own save bar; the form never submits by itself.
  const uiSchema = useMemo(() => ({ ...uiFor(schema), "ui:submitButtonOptions": { norender: true } }), [schema]);
  const formContext = useMemo(() => ({ collectionId }), [collectionId]);
  const extraErrors = useMemo(() => (issues?.length ? issueErrors(issues, translators[locale]) : undefined), [issues, locale]);
  return (
    <Form
      ref={formRef}
      className="rjsf"
      schema={schema as RJSFSchema}
      uiSchema={uiSchema}
      formData={data}
      validator={validators[locale]}
      templates={templates}
      fields={fields}
      formContext={formContext}
      disabled={disabled}
      noHtml5Validate
      showErrorList={false}
      extraErrors={extraErrors}
      liveValidate={liveValidate}
      idPrefix={idPrefix}
      onChange={(e) => onChange?.(e.formData)}
      // The outer page form owns submit; a nested form element would be invalid HTML.
      tagName="div"
    />
  );
}
