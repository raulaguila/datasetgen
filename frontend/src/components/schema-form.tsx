"use client";

import Form from "@rjsf/core";
import type { IconButtonProps, RJSFSchema, UiSchema } from "@rjsf/utils";
import validator from "@rjsf/validator-ajv8";
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

// The page has its own save bar; the form never submits by itself.
const hiddenSubmit: UiSchema = { "ui:submitButtonOptions": { norender: true } };

/**
 * A form generated from a collection's JSON Schema. Validation runs on the server,
 * which answers the broken rules; this only renders and collects the data.
 * shortcut: the ajv validator is draft-07, used only for rjsf's own schema resolving;
 * switch to an Ajv2020 validator if schemas start relying on 2020-12-only keywords.
 */
export function SchemaForm({ schema, data, disabled, onChange, idPrefix = "rec" }: {
  schema: Record<string, unknown>;
  data: unknown;
  disabled?: boolean;
  onChange?: (data: unknown) => void;
  idPrefix?: string;
}) {
  return (
    <Form
      className="rjsf"
      schema={schema as RJSFSchema}
      uiSchema={hiddenSubmit}
      formData={data}
      validator={validator}
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
