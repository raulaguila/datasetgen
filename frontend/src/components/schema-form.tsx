"use client";

import Form from "@rjsf/core";
import type { ErrorSchema, IconButtonProps, RJSFSchema, UiSchema } from "@rjsf/utils";
import validator, { customizeValidator } from "@rjsf/validator-ajv8";
import Ajv2020 from "ajv/dist/2020";
import localizePt from "ajv-i18n/localize/pt-BR";
import { useMemo, type Ref } from "react";
import { ArrowDown, ArrowUp, Copy, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { SchemaIssue } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";
import type { Locale } from "@/lib/i18n/messages";

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

// The server defaults to draft 2020-12 and honours $schema; mirror that here,
// with ajv's messages in the app language.
const validators = {
  "pt-BR": { legacy: customizeValidator({}, localizePt), modern: customizeValidator({ AjvClass: Ajv2020 }, localizePt) },
  "en-US": { legacy: validator, modern: customizeValidator({ AjvClass: Ajv2020 }) },
};

/** Picks the validator for the schema's draft and drops $schema, which ajv cannot resolve offline. */
export function forDraft(schema: Record<string, unknown>, locale: Locale = "en-US") {
  const { $schema, ...rest } = schema;
  const legacy = typeof $schema === "string" && !$schema.includes("2020-12") && !$schema.includes("2019-09");
  return { schema: rest as RJSFSchema, validator: validators[locale][legacy ? "legacy" : "modern"] };
}

/** Turns the server's issues (JSON pointers) into errors shown under each field. */
export function issueErrors(issues: SchemaIssue[]): ErrorSchema {
  const root: Record<string, unknown> = {};
  for (const { path, message } of issues) {
    let node = root;
    for (const part of path.split("/").slice(1)) {
      const key = part.replace(/~1/g, "/").replace(/~0/g, "~");
      node = (node[key] ??= {}) as Record<string, unknown>;
    }
    ((node.__errors ??= []) as string[]).push(message);
  }
  return root as ErrorSchema;
}

/** Text areas for strings allowed to run long (the field builder's "long text"), at any depth. */
export function uiFor(schema: unknown): UiSchema {
  if (!schema || typeof schema !== "object") return {};
  const s = schema as { type?: unknown; maxLength?: unknown; properties?: Record<string, unknown>; items?: unknown };
  if (s.type === "string" && typeof s.maxLength === "number" && s.maxLength >= 500) return { "ui:widget": "textarea" };
  const out: UiSchema = {};
  for (const [key, sub] of Object.entries(s.properties ?? {})) {
    const ui = uiFor(sub);
    if (Object.keys(ui).length) out[key] = ui;
  }
  const items = uiFor(Array.isArray(s.items) ? null : s.items);
  if (Object.keys(items).length) out.items = items;
  return out;
}

/**
 * A form generated from a collection's JSON Schema. The page calls
 * formRef.current.validateForm() before saving to show broken rules under each
 * field; the server checks again and its issues come back through extraErrors.
 */
export function SchemaForm({ schema, data, disabled, onChange, idPrefix = "rec", formRef, extraErrors }: {
  schema: Record<string, unknown>;
  data: unknown;
  disabled?: boolean;
  onChange?: (data: unknown) => void;
  idPrefix?: string;
  formRef?: Ref<Form>;
  extraErrors?: ErrorSchema;
}) {
  const { locale } = useI18n();
  const draft = useMemo(() => forDraft(schema, locale), [schema, locale]);
  // The page has its own save bar; the form never submits by itself.
  const uiSchema = useMemo(() => ({ ...uiFor(draft.schema), "ui:submitButtonOptions": { norender: true } }), [draft.schema]);
  return (
    <Form
      ref={formRef}
      className="rjsf"
      schema={draft.schema}
      uiSchema={uiSchema}
      formData={data}
      validator={draft.validator}
      templates={templates}
      disabled={disabled}
      noHtml5Validate
      showErrorList={false}
      extraErrors={extraErrors}
      idPrefix={idPrefix}
      onChange={(e) => onChange?.(e.formData)}
      // The outer page form owns submit; a nested form element would be invalid HTML.
      tagName="div"
    />
  );
}
