"use client";

import { useState } from "react";
import { Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useI18n } from "@/lib/i18n/provider";

export const FIELD_TYPES = ["string", "text", "number", "integer", "boolean", "enum", "strings"] as const;
export type FieldType = (typeof FIELD_TYPES)[number];

/** Long text: SchemaForm draws a text area for strings allowed this long. */
export const LONG_TEXT = 5000;

export type Field = { name: string; type: FieldType; required: boolean; options: string };

function property(f: Field): Record<string, unknown> {
  const title = f.name.trim();
  switch (f.type) {
    case "text": return { type: "string", title, maxLength: LONG_TEXT };
    case "enum": return { type: "string", title, enum: f.options.split(",").map((o) => o.trim()).filter(Boolean) };
    case "strings": return { type: "array", title, items: { type: "string" } };
    default: return { type: f.type, title };
  }
}

/** Adds one top-level field to an object schema, keeping every other key as it was. */
export function addField(schema: Record<string, unknown>, f: Field): Record<string, unknown> {
  const name = f.name.trim();
  const properties = { ...(schema.properties as Record<string, unknown> | undefined), [name]: property(f) };
  const required = (schema.required as string[] | undefined) ?? [];
  return {
    ...schema,
    type: schema.type ?? "object",
    properties,
    ...(f.required && !required.includes(name) ? { required: [...required, name] } : {}),
  };
}

const blank: Field = { name: "", type: "string", required: false, options: "" };

/** A row that writes one field into the schema, so nobody types JSON for the common cases. */
export function FieldBuilder({ schema, disabled, onAdd }: { schema: Record<string, unknown> | null; disabled?: boolean; onAdd: (next: Record<string, unknown>) => void }) {
  const { t } = useI18n();
  const [field, setField] = useState<Field>(blank);
  const set = (patch: Partial<Field>) => setField((f) => ({ ...f, ...patch }));
  const name = field.name.trim();
  const taken = !!name && !!schema && Object.hasOwn((schema.properties as object | undefined) ?? {}, name);
  const ready = !!schema && !!name && !taken && (field.type !== "enum" || field.options.split(",").some((o) => o.trim()));
  const types = Object.fromEntries(FIELD_TYPES.map((k) => [k, t(`collections.type.${k}`)]));

  return (
    <div className="flex flex-wrap items-end gap-3 rounded-lg border border-dashed p-3">
      <div className="min-w-40 flex-1 space-y-1.5">
        <Label htmlFor="fb-name">{t("collections.field_name")}</Label>
        <Input id="fb-name" value={field.name} disabled={disabled} aria-invalid={taken} aria-describedby="fb-note" onChange={(e) => set({ name: e.target.value })} />
      </div>
      <div className="w-48 space-y-1.5 max-sm:w-full">
        <Label>{t("collections.field_type")}</Label>
        <Select value={field.type} items={types} disabled={disabled} onValueChange={(v) => set({ type: v as FieldType })}>
          <SelectTrigger className="w-full" aria-label={t("collections.field_type")}><SelectValue /></SelectTrigger>
          <SelectContent>
            {FIELD_TYPES.map((k) => <SelectItem key={k} value={k}>{types[k]}</SelectItem>)}
          </SelectContent>
        </Select>
      </div>
      {field.type === "enum" && (
        <div className="min-w-48 flex-1 space-y-1.5">
          <Label htmlFor="fb-options">{t("collections.field_options")}</Label>
          <Input id="fb-options" value={field.options} disabled={disabled} onChange={(e) => set({ options: e.target.value })} />
        </div>
      )}
      <label className="flex h-9 items-center gap-2 text-sm">
        <Checkbox checked={field.required} disabled={disabled} onCheckedChange={(on) => set({ required: on === true })} />
        {t("collections.field_required")}
      </label>
      <Button type="button" variant="outline" disabled={disabled || !ready} onClick={() => { onAdd(addField(schema!, field)); setField(blank); }}>
        <Plus className="size-4" />{t("collections.add_field")}
      </Button>
      {taken && <p id="fb-note" className="basis-full text-xs text-destructive" role="status">{t("collections.field_exists")}</p>}
    </div>
  );
}
