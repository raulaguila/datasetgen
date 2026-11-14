"use client";

import Form, { getDefaultRegistry } from "@rjsf/core";
import { englishStringTranslator, enumOptionsIndexForValue, enumOptionsValueForIndex, TranslatableString, type WidgetProps, type ArrayFieldItemTemplateProps, type ErrorSchema, type FieldProps, type IconButtonProps, type RJSFSchema, type UiSchema, type ValidatorType } from "@rjsf/utils";
import Link from "next/link";
import { useMemo, useRef, useState, type Ref } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowDown, ArrowUp, Copy, FileUp, Loader2, Paperclip, Plus, Search, X } from "lucide-react";
import { AUTO, FILE, FORMULA, REF, fileRefs, isImage, recordTitle } from "@/components/field-builder";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { collections, records, tools } from "@/lib/api/endpoints";
import type { FileRef, SchemaIssue } from "@/lib/api/types";
import { errorMessage } from "@/lib/errors";
import { useDebounced } from "@/lib/hooks/use-debounced";
import { searchTerm } from "@/lib/utils";
import { useI18n } from "@/lib/i18n/provider";
import { LOCALES, messages, type Locale, type MessageKey } from "@/lib/i18n/messages";
import { schemaValidator, type Translate } from "@/lib/schema-validator";

// The default theme draws glyphicon fonts this app does not load; these use lucide.
// Some buttons (add) come without a title: the name falls back to the registry's translated string.
function iconButton(Icon: typeof Plus, name: TranslatableString, variant: "outline" | "ghost" | "destructive" = "ghost") {
  return function SchemaIconButton({ onClick, disabled, title, className, registry }: IconButtonProps) {
    const label = title || registry.translateString(name);
    return (
      <Button type="button" size="icon-sm" variant={variant} title={label} aria-label={label} disabled={disabled} className={className} onClick={onClick}>
        <Icon className="size-4" />
      </Button>
    );
  };
}

/** Adds a list item: a labelled dashed button under the list, like the field builder's. */
function AddButton({ onClick, disabled, className, registry }: IconButtonProps) {
  return (
    <Button type="button" size="sm" variant="outline" disabled={disabled} className={`${className ?? ""} w-full border-dashed`} onClick={onClick}>
      <Plus className="size-4" />{registry.translateString(TranslatableString.AddItemButton)}
    </Button>
  );
}

/**
 * One list item. An object item is a card numbered #1, #2… with its buttons in
 * the header (its own "Steps-1" title is hidden by the CSS); a plain value sits
 * in one row with its buttons beside it.
 */
// The stock item buttons (move, copy, remove); they draw through ButtonTemplates below.
const ItemButtons = getDefaultRegistry().templates.ArrayFieldItemButtonsTemplate;

function ArrayItem({ children, className, buttonsProps, hasToolbar, index, schema }: ArrayFieldItemTemplateProps) {
  const toolbox = hasToolbar && <div className="array-item-toolbox flex shrink-0 gap-0.5"><ItemButtons {...buttonsProps} /></div>;
  if (schema.type !== "object" || !schema.properties) return <div className={`${className} rjsf-item-row`}><div className="min-w-0 flex-1">{children}</div>{toolbox}</div>;
  return (
    <div className={`${className} rjsf-item-card`}>
      <div className="rjsf-item-head"><span className="font-mono text-xs text-muted-foreground">#{index + 1}</span>{toolbox}</div>
      {children}
    </div>
  );
}

const templates = {
  ArrayFieldItemTemplate: ArrayItem,
  ButtonTemplates: {
    AddButton,
    CopyButton: iconButton(Copy, TranslatableString.CopyButton),
    MoveUpButton: iconButton(ArrowUp, TranslatableString.MoveUpButton),
    MoveDownButton: iconButton(ArrowDown, TranslatableString.MoveDownButton),
    RemoveButton: iconButton(X, TranslatableString.RemoveButton),
    ClearButton: iconButton(X, TranslatableString.RemoveButton),
  },
};

// The list buttons' names (their tooltip and accessible name) in the user's language.
const BUTTON_KEYS: Partial<Record<TranslatableString, MessageKey>> = {
  [TranslatableString.AddItemButton]: "schema.item_add",
  [TranslatableString.AddButton]: "schema.item_add",
  [TranslatableString.RemoveButton]: "schema.item_remove",
  [TranslatableString.CopyButton]: "schema.item_copy",
  [TranslatableString.MoveUpButton]: "schema.item_up",
  [TranslatableString.MoveDownButton]: "schema.item_down",
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

/** Searches the target collection and hands back the picked record's id. */
function RecordPicker({ id, target, onPick, onClose }: { id: string; target: string; onPick: (id: string) => void; onClose: () => void }) {
  const { t } = useI18n();
  const [text, setText] = useState("");
  const q = searchTerm(useDebounced(text));
  const meta = useQuery({ queryKey: ["collections", "detail", target], queryFn: () => collections.get(target), enabled: !!target });
  const found = useQuery({ queryKey: ["records", target, "pick", q], queryFn: ({ signal }) => records.list({ collection_id: target, page: 1, limit: 8, search: q }, signal), enabled: !!target });
  return (
    <div className="space-y-1.5 rounded-md border p-2">
      <Input id={id} autoFocus type="search" placeholder={t("schema.ref_search")} value={text} onChange={(e) => setText(e.target.value)} onKeyDown={(e) => { if (e.key === "Escape") onClose(); if (e.key === "Enter") e.preventDefault(); }} />
      {found.isPending ? <Loader2 className="mx-auto size-4 animate-spin text-muted-foreground" /> : (
        <ul className="max-h-56 overflow-auto">
          {(found.data?.items ?? []).map((r) => (
            <li key={r.id}>
              <button type="button" className="w-full truncate rounded px-2 py-1.5 text-left text-sm hover:bg-muted" onClick={() => { onPick(r.id); setText(""); }}>
                {recordTitle(meta.data?.schema ?? null, r.data) ?? r.id.slice(0, 8)}
              </button>
            </li>
          ))}
          {found.data?.items.length === 0 && <li className="px-2 py-1.5 text-sm text-muted-foreground">{t("common.no_results")}</li>}
        </ul>
      )}
      <Button type="button" size="sm" variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
    </div>
  );
}

const refTarget = (schema: unknown) => {
  const v = (schema as Record<string, unknown>)[REF];
  return typeof v === "string" ? v : "";
};

/** A record of the collection named by x-ref, picked by searching it. */
function ReferenceField({ formData, onChange, fieldPathId, schema, name, required, disabled, readonly }: FieldProps) {
  const { t } = useI18n();
  const target = refTarget(schema);
  const value = typeof formData === "string" && formData ? formData : undefined;
  const label = useRecordLabel(target, value);
  const [open, setOpen] = useState(false);
  const id = fieldPathId.$id;
  const off = disabled || readonly;
  const pick = (v: string | undefined) => { onChange(v, fieldPathId.path); setOpen(false); };
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
        <RecordPicker id={id} target={target} onPick={pick} onClose={() => setOpen(false)} />
      )}
    </>
  );
}

function RefChip({ target, value, onRemove }: { target: string; value: string; onRemove?: () => void }) {
  const { t } = useI18n();
  const label = useRecordLabel(target, value);
  return (
    <li className="flex items-center gap-1 rounded-full border bg-muted/40 py-0.5 pl-2.5 pr-0.5 text-sm">
      <Link href={`/records/${value}`} className="max-w-60 truncate hover:underline">{label}</Link>
      {onRemove && <button type="button" className="grid size-6 place-items-center rounded-full hover:bg-accent" aria-label={`${t("schema.ref_clear")}: ${label}`} onClick={onRemove}><X className="size-3" /></button>}
    </li>
  );
}

/** Several records of the collection named by x-ref, kept as a list of ids. */
function ReferencesField({ formData, onChange, fieldPathId, schema, name, required, disabled, readonly }: FieldProps) {
  const { t } = useI18n();
  const target = refTarget(schema);
  const value = Array.isArray(formData) ? (formData as unknown[]).filter((v): v is string => typeof v === "string") : [];
  const [open, setOpen] = useState(false);
  const id = fieldPathId.$id;
  const off = disabled || readonly;
  const set = (next: string[]) => onChange(next.length ? next : undefined, fieldPathId.path);
  return (
    <>
      <Head id={id} schema={schema} name={name} required={required} />
      {value.length > 0 && (
        <ul className="flex flex-wrap gap-1.5" aria-label={String(schema.title ?? name)}>
          {value.map((v) => <RefChip key={v} target={target} value={v} onRemove={off ? undefined : () => set(value.filter((x) => x !== v))} />)}
        </ul>
      )}
      {off ? (value.length === 0 && <p className="text-sm text-muted-foreground">—</p>) : !open ? (
        <Button {...named(id)} type="button" variant="outline" size="sm" onClick={() => setOpen(true)}><Search className="size-4" />{t("schema.ref_add")}</Button>
      ) : (
        <RecordPicker id={id} target={target} onPick={(r) => { if (!value.includes(r)) set([...value, r]); }} onClose={() => setOpen(false)} />
      )}
    </>
  );
}

export const fileSize = (n: number) => (n < 1024 ? `${n} B` : n < 1024 * 1024 ? `${(n / 1024).toFixed(1)} KB` : `${(n / 1024 / 1024).toFixed(1)} MB`);
const FILE_MAX = 8 * 1024 * 1024;
const FILES_MAX = 20;

/** One attached file: a thumbnail for images, else a clip; opens in a new tab. */
function FileChip({ file, onRemove }: { file: FileRef; onRemove?: () => void }) {
  const { t } = useI18n();
  return (
    <span className="inline-flex min-w-0 max-w-full items-center gap-1">
      <a href={tools.fileUrl(file.id)} target="_blank" rel="noopener" className="inline-flex min-w-0 max-w-full items-center gap-2 rounded-md border px-2 py-1 text-sm hover:underline">
        {isImage(file)
          // eslint-disable-next-line @next/next/no-img-element -- a private file behind the session proxy, not an optimizable asset
          ? <img src={tools.fileUrl(file.id)} alt="" loading="lazy" className="size-8 shrink-0 rounded object-cover" />
          : <Paperclip className="size-4 shrink-0" />}
        <span className="truncate">{file.name}</span><span className="shrink-0 text-xs text-muted-foreground">{fileSize(file.size)}</span>
      </a>
      {onRemove && <Button type="button" size="icon-sm" variant="ghost" aria-label={`${t("schema.file_remove")}: ${file.name}`} title={t("schema.file_remove")} onClick={onRemove}><X className="size-4" /></Button>}
    </span>
  );
}

/**
 * An attached file, or several (an array field): each is uploaded to the
 * collection at once, the record keeps {id, name, size, type}.
 */
function FileField({ formData, onChange, fieldPathId, schema, name, required, disabled, readonly, registry }: FieldProps) {
  const { t } = useI18n();
  const collectionId = (registry.formContext as { collectionId?: string } | undefined)?.collectionId;
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const many = schema.type === "array";
  const files = fileRefs(formData);
  const id = fieldPathId.$id;
  const off = disabled || readonly || !collectionId;
  const full = many && files.length >= FILES_MAX;
  const set = (next: FileRef[]) => onChange(many ? (next.length ? next : undefined) : next[0], fieldPathId.path);
  const upload = async (picked: File[]) => {
    setError(null);
    const list = many ? picked.slice(0, FILES_MAX - files.length) : picked.slice(0, 1);
    if (list.some((f) => f.size > FILE_MAX)) return setError(t("schema.file_too_large"));
    setBusy(true);
    try {
      const added: FileRef[] = [];
      for (const f of list) added.push(await tools.upload(collectionId!, f));
      set(many ? [...files, ...added] : added);
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
        {files.length > 0 && (
          <ul className="flex min-w-0 max-w-full flex-wrap gap-2" aria-label={String(schema.title ?? name)}>
            {files.map((f, i) => <li key={f.id} className="min-w-0 max-w-full"><FileChip file={f} onRemove={off ? undefined : () => set(files.filter((_, j) => j !== i))} /></li>)}
          </ul>
        )}
        {!off && !full && (
          <Button {...named(id)} type="button" variant="outline" size="sm" disabled={busy} onClick={() => input.current?.click()}>
            {busy ? <Loader2 className="size-4 animate-spin" /> : <FileUp className="size-4" />}{t(many ? "schema.files_add" : files.length ? "schema.file_replace" : "schema.file_pick")}
          </Button>
        )}
        {!files.length && off && <span className="text-sm text-muted-foreground">{collectionId ? "—" : t("schema.file_preview")}</span>}
        <input ref={input} type="file" multiple={many} className="hidden" aria-hidden tabIndex={-1} onChange={(e) => { const f = [...(e.target.files ?? [])]; e.target.value = ""; if (f.length) void upload(f); }} />
      </div>
      <p className="text-xs text-muted-foreground">{t(full ? "schema.files_full" : "schema.file_hint")}</p>
      {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
    </>
  );
}

const fields = { json: JsonField, reference: ReferenceField, references: ReferencesField, file: FileField };

const NONE = "\u0000none";
const DefaultSelect = getDefaultRegistry().widgets.SelectWidget;

/** One choice of a list, drawn with the app's select (the browser's own looked foreign); several choices keep the stock widget. */
function SelectWidget(props: WidgetProps) {
  const { id, options, value, required, disabled, readonly, onChange, onBlur, rawErrors, multiple } = props;
  const { t } = useI18n();
  if (multiple) return <DefaultSelect {...props} />;
  const choices = options.enumOptions ?? [];
  const index = enumOptionsIndexForValue(value, choices);
  const current = index === undefined || Array.isArray(index) ? NONE : String(index);
  const items = { [NONE]: "—", ...Object.fromEntries(choices.map((c, i) => [String(i), c.label])) };
  return (
    <Select value={current === NONE ? null : current} items={items} disabled={disabled || readonly}
      onValueChange={(v) => onChange(v === NONE || v == null ? options.emptyValue : enumOptionsValueForIndex(String(v), choices, options.emptyValue))}
      onOpenChange={(open) => { if (!open) onBlur(id, value); }}>
      <SelectTrigger id={id} className="w-full" aria-invalid={!!rawErrors?.length} aria-required={required}><SelectValue placeholder={t("schema.pick")} /></SelectTrigger>
      <SelectContent>
        {!required && <SelectItem value={NONE}>—</SelectItem>}
        {choices.map((c, i) => <SelectItem key={i} value={String(i)}>{c.label}</SelectItem>)}
      </SelectContent>
    </Select>
  );
}

const widgets = { SelectWidget };

/**
 * Text areas for strings allowed to run long (the field builder's "long text"),
 * the first example as the placeholder, and a JSON box for free objects below
 * the root, at any depth.
 */
export function uiFor(schema: unknown, nested = false): UiSchema {
  if (!schema || typeof schema !== "object") return {};
  const s = schema as { type?: unknown; maxLength?: unknown; examples?: unknown; properties?: Record<string, unknown>; items?: unknown; [REF]?: unknown; [FILE]?: unknown; [AUTO]?: unknown; [FORMULA]?: unknown };
  if (nested && s.type === "string" && typeof s[REF] === "string") return { "ui:field": "reference" };
  if (nested && s.type === "array" && typeof s[REF] === "string") return { "ui:field": "references" };
  if (nested && (s.type === "object" || s.type === "array") && s[FILE] === true) return { "ui:field": "file" };
  if (nested && (typeof s[AUTO] === "string" || typeof s[FORMULA] === "string")) return { "ui:readonly": true }; // the server fills it on save
  const out: UiSchema = {};
  if (Array.isArray(s.examples) && typeof s.examples[0] === "string") out["ui:placeholder"] = s.examples[0];
  if (s.type === "string" && typeof s.maxLength === "number" && s.maxLength >= 500) out["ui:widget"] = "textarea";
  if (s.type === "string") return out;
  if (nested && s.type === "object" && s.properties === undefined) return { ...out, "ui:field": "json" };
  // Several picks from a fixed list: a checkbox each, not the browser's cramped multi-select box.
  if (s.type === "array" && (s as { uniqueItems?: unknown }).uniqueItems === true && Array.isArray((s.items as { enum?: unknown } | undefined)?.enum)) return { ...out, "ui:widget": "checkboxes" };
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
export function SchemaForm({ schema, data, disabled, onChange, idPrefix = "rec", formRef, issues, liveValidate, collectionId, columns = 1 }: {
  schema: Record<string, unknown>;
  /** Two puts short fields side by side when the form is wide enough; long text, groups and lists keep the whole row. */
  columns?: 1 | 2;
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
  const { locale, t } = useI18n();
  const translateString = useMemo(() => (str: TranslatableString, params?: string[]) => {
    const key = BUTTON_KEYS[str];
    return key ? t(key) : englishStringTranslator(str, params);
  }, [t]);
  // The page has its own save bar; the form never submits by itself.
  const uiSchema = useMemo(() => ({ ...uiFor(schema), "ui:submitButtonOptions": { norender: true } }), [schema]);
  const formContext = useMemo(() => ({ collectionId }), [collectionId]);
  const extraErrors = useMemo(() => (issues?.length ? issueErrors(issues, translators[locale]) : undefined), [issues, locale]);
  return (
    <div className="rjsf-frame" data-columns={columns}>
    <Form
      ref={formRef}
      className="rjsf"
      schema={schema as RJSFSchema}
      uiSchema={uiSchema}
      formData={data}
      validator={validators[locale]}
      templates={templates}
      translateString={translateString}
      fields={fields}
      widgets={widgets}
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
    </div>
  );
}
