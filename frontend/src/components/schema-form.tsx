"use client";

import Form from "@rjsf/core";
import type { IconButtonProps, RJSFSchema, UiSchema } from "@rjsf/utils";
import validator, { customizeValidator } from "@rjsf/validator-ajv8";
import Ajv2020 from "ajv/dist/2020";
import { ArrowDown, ArrowUp, Copy, Plus, X } from "lucide-react";
import { Button } from "@/components/ui/button";

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

// The server defaults to draft 2020-12 and honours $schema; mirror that here.
const validator2020 = customizeValidator({ AjvClass: Ajv2020 });

/** Picks the validator for the schema's draft and drops $schema, which ajv cannot resolve offline. */
export function forDraft(schema: Record<string, unknown>) {
  const { $schema, ...rest } = schema;
  const legacy = typeof $schema === "string" && !$schema.includes("2020-12") && !$schema.includes("2019-09");
  return { schema: rest as RJSFSchema, validator: legacy ? validator : validator2020 };
}

// The page has its own save bar; the form never submits by itself.
const hiddenSubmit: UiSchema = { "ui:submitButtonOptions": { norender: true } };

/**
 * A form generated from a collection's JSON Schema. Validation runs on the server,
 * which answers the broken rules; this only renders and collects the data.
 */
export function SchemaForm({ schema, data, disabled, onChange, idPrefix = "rec" }: {
  schema: Record<string, unknown>;
  data: unknown;
  disabled?: boolean;
  onChange?: (data: unknown) => void;
  idPrefix?: string;
}) {
  const draft = forDraft(schema);
  return (
    <Form
      className="rjsf"
      schema={draft.schema}
      uiSchema={hiddenSubmit}
      formData={data}
      validator={draft.validator}
      templates={templates}
      disabled={disabled}
      noHtml5Validate
      showErrorList={false}
      idPrefix={idPrefix}
      onChange={(e) => onChange?.(e.formData)}
      // The outer page form owns submit; a nested form element would be invalid HTML.
      tagName="div"
    />
  );
}
