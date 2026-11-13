"use client";

import Form from "@rjsf/core";
import type { ErrorSchema, FieldProps, IconButtonProps, RJSFSchema, UiSchema, ValidatorType } from "@rjsf/utils";
import { useMemo, useState, type Ref } from "react";
import { ArrowDown, ArrowUp, Copy, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import type { SchemaIssue } from "@/lib/api/types";
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

const fields = { json: JsonField };

/**
 * Text areas for strings allowed to run long (the field builder's "long text"),
 * the first example as the placeholder, and a JSON box for free objects below
 * the root, at any depth.
 */
export function uiFor(schema: unknown, nested = false): UiSchema {
  if (!schema || typeof schema !== "object") return {};
  const s = schema as { type?: unknown; maxLength?: unknown; examples?: unknown; properties?: Record<string, unknown>; items?: unknown };
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
export function SchemaForm({ schema, data, disabled, onChange, idPrefix = "rec", formRef, issues, liveValidate }: {
  schema: Record<string, unknown>;
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
