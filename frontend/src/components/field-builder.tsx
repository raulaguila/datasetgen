"use client";

import { useState } from "react";
import { ArrowDown, ArrowUp, Pencil, Plus, Trash2 } from "lucide-react";
import { IconAction } from "@/components/bits";
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
/** The shortest maxLength SchemaForm (uiFor) draws as a text area. */
const TEXT_AREA_FROM = 500;

export type Field = { name: string; type: FieldType; required: boolean; options: string };
/** A top-level field: `key` is where its value is stored, `field.name` its label. Advanced ones are edited as JSON. */
export type FieldRow = { key: string; field: Field; advanced: boolean };

type Schema = Record<string, unknown>;
type Props = Record<string, Record<string, unknown>>;

const optionsOf = (f: Field) => f.options.split(",").map((o) => o.trim()).filter(Boolean);

function property(f: Field): Record<string, unknown> {
  const title = f.name.trim();
  switch (f.type) {
    case "text": return { type: "string", title, maxLength: LONG_TEXT };
    case "enum": return { type: "string", title, enum: optionsOf(f) };
    case "strings": return { type: "array", title, items: { type: "string" } };
    default: return { type: f.type, title };
  }
}

/** The builder type that draws this property, or undefined when only JSON can describe it. */
export function typeOf(p: unknown): FieldType | undefined {
  if (!p || typeof p !== "object") return undefined;
  const s = p as Record<string, unknown>;
  // Options are edited comma separated, so an option holding a comma stays in JSON.
  if (Array.isArray(s.enum)) return s.enum.every((v) => typeof v === "string" && !v.includes(",")) ? "enum" : undefined;
  switch (s.type) {
    case "string": return typeof s.maxLength === "number" && s.maxLength >= TEXT_AREA_FROM ? "text" : "string";
    case "number":
    case "integer":
    case "boolean": return s.type;
    case "array": {
      const items = s.items as Record<string, unknown> | undefined;
      return items?.type === "string" && !items.enum ? "strings" : undefined;
    }
  }
  return undefined;
}

const propsOf = (schema: Schema) => (schema.properties ?? {}) as Props;
const requiredOf = (schema: Schema) => (Array.isArray(schema.required) ? (schema.required as string[]) : []);

/** The first plain text value a record has, in field order, to name it in lists and titles. */
export function recordTitle(schema: Schema | null, data: unknown): string | undefined {
  const obj = data && typeof data === "object" ? (data as Record<string, unknown>) : {};
  const v = fieldsOf(schema).filter((r) => !r.advanced).map((r) => obj[r.key]).find((x) => typeof x === "string" && x.trim());
  return typeof v === "string" ? v : undefined;
}

export function fieldsOf(schema: Schema | null): FieldRow[] {
  if (!schema) return [];
  const required = requiredOf(schema);
  return Object.entries(propsOf(schema)).map(([key, p]) => {
    const type = typeOf(p);
    return {
      key,
      advanced: !type,
      field: {
        name: typeof p?.title === "string" && p.title ? p.title : key,
        type: type ?? "string",
        required: required.includes(key),
        options: Array.isArray(p?.enum) ? p.enum.join(", ") : "",
      },
    };
  });
}

/** The schema with these properties (in this order) and required list, every other key kept. */
function rebuild(schema: Schema, entries: [string, unknown][], required: string[]): Schema {
  const rest = { ...schema };
  delete rest.required;
  return { ...rest, type: schema.type ?? "object", properties: Object.fromEntries(entries), ...(required.length ? { required } : {}) };
}

/** Adds one top-level field to an object schema, keeping every other key as it was. */
export function addField(schema: Schema, f: Field): Schema {
  const name = f.name.trim();
  const required = requiredOf(schema);
  return rebuild(schema, [...Object.entries(propsOf(schema)), [name, property(f)]], f.required && !required.includes(name) ? [...required, name] : required);
}

/**
 * Changes a field in place. The key never changes, so stored records keep their
 * values; on the same type the property's other rules (description, limits) stay.
 */
export function updateField(schema: Schema, key: string, f: Field): Schema {
  const entries = Object.entries(propsOf(schema)).map(([k, old]): [string, unknown] => {
    if (k !== key) return [k, old];
    if (typeOf(old) !== f.type) return [k, { ...(old?.description ? { description: old.description } : {}), ...property(f) }];
    return [k, { ...old, title: f.name.trim(), ...(f.type === "enum" ? { enum: optionsOf(f) } : {}) }];
  });
  const required = requiredOf(schema);
  const next = !f.required ? required.filter((k) => k !== key) : required.includes(key) ? required : [...required, key];
  return rebuild(schema, entries, next);
}

export function removeField(schema: Schema, key: string): Schema {
  return rebuild(schema, Object.entries(propsOf(schema)).filter(([k]) => k !== key), requiredOf(schema).filter((k) => k !== key));
}

/** Moves a field one place up (-1) or down (+1); the form follows this order. */
export function moveField(schema: Schema, key: string, delta: -1 | 1): Schema {
  const entries = Object.entries(propsOf(schema));
  const i = entries.findIndex(([k]) => k === key);
  const j = i + delta;
  if (i < 0 || j < 0 || j >= entries.length) return schema;
  [entries[i], entries[j]] = [entries[j], entries[i]];
  return rebuild(schema, entries, requiredOf(schema));
}

const blank: Field = { name: "", type: "string", required: false, options: "" };

/** One row that writes a field into the schema, so nobody types JSON for the common cases. */
function FieldForm({ initial, taken, submitLabel, onSubmit, onCancel }: {
  initial: Field;
  taken: (name: string) => boolean;
  submitLabel: string;
  onSubmit: (f: Field) => void;
  onCancel?: () => void;
}) {
  const { t } = useI18n();
  const [field, setField] = useState<Field>(initial);
  const set = (patch: Partial<Field>) => setField((f) => ({ ...f, ...patch }));
  const name = field.name.trim();
  const isTaken = !!name && taken(name);
  const ready = !!name && !isTaken && (field.type !== "enum" || optionsOf(field).length > 0);
  const types = Object.fromEntries(FIELD_TYPES.map((k) => [k, t(`collections.type.${k}`)]));
  const submit = () => {
    if (!ready) return;
    onSubmit(field);
    setField(blank);
  };

  return (
    <div className="flex flex-wrap items-end gap-3 rounded-lg border border-dashed p-3">
      <div className="min-w-40 flex-1 space-y-1.5">
        <Label htmlFor="fb-name">{t("collections.field_name")}</Label>
        <Input
          id="fb-name"
          value={field.name}
          autoFocus={!!onCancel}
          aria-invalid={isTaken}
          aria-describedby="fb-note"
          onChange={(e) => set({ name: e.target.value })}
          // Enter adds the field instead of submitting the collection form.
          onKeyDown={(e) => { if (e.key === "Enter") { e.preventDefault(); submit(); } }}
        />
      </div>
      <div className="w-48 space-y-1.5 max-sm:w-full">
        <Label>{t("collections.field_type")}</Label>
        <Select value={field.type} items={types} onValueChange={(v) => set({ type: v as FieldType })}>
          <SelectTrigger className="w-full" aria-label={t("collections.field_type")}><SelectValue /></SelectTrigger>
          <SelectContent>
            {FIELD_TYPES.map((k) => <SelectItem key={k} value={k}>{types[k]}</SelectItem>)}
          </SelectContent>
        </Select>
      </div>
      {field.type === "enum" && (
        <div className="min-w-48 flex-1 space-y-1.5">
          <Label htmlFor="fb-options">{t("collections.field_options")}</Label>
          <Input id="fb-options" value={field.options} onChange={(e) => set({ options: e.target.value })} />
        </div>
      )}
      <label className="flex h-9 items-center gap-2 text-sm">
        <Checkbox checked={field.required} onCheckedChange={(on) => set({ required: on === true })} />
        {t("collections.field_required")}
      </label>
      <div className="flex gap-2">
        {onCancel && <Button type="button" variant="ghost" onClick={onCancel}>{t("common.cancel")}</Button>}
        <Button type="button" variant="outline" disabled={!ready} onClick={submit}>
          {!onCancel && <Plus className="size-4" />}{submitLabel}
        </Button>
      </div>
      {isTaken && <p id="fb-note" className="basis-full text-xs text-destructive" role="status">{t("collections.field_exists")}</p>}
    </div>
  );
}

/** The schema's fields as a list: add, edit, remove and reorder without touching JSON. */
export function FieldsEditor({ schema, editable, onChange }: { schema: Schema | null; editable: boolean; onChange: (next: Schema) => void }) {
  const { t } = useI18n();
  const [editing, setEditing] = useState<string | null>(null);
  const rows = fieldsOf(schema);
  const keys = new Set(rows.map((r) => r.key));
  const current = rows.find((r) => r.key === editing);

  if (!schema) return <p className="text-sm text-muted-foreground">{t("collections.fields_need_json")}</p>;
  return (
    <div className="space-y-3">
      {rows.length === 0 ? (
        <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">{t("collections.fields_empty")}</p>
      ) : (
        <ul aria-label={t("collections.fields")} className="divide-y rounded-lg border">
          {rows.map(({ key, field, advanced }, i) => (
            <li key={key} className="flex items-center gap-3 px-3 py-2" data-editing={key === editing || undefined}>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-medium">
                  {field.name}
                  {field.required && <span className="ml-1 text-destructive" title={t("collections.field_required")}>*</span>}
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {advanced ? t("collections.type.advanced") : t(`collections.type.${field.type}`)}
                  {field.type === "enum" && !advanced && `: ${field.options}`}
                  {key !== field.name && <span className="font-mono"> · {key}</span>}
                </p>
              </div>
              {editable && (
                <div className="flex shrink-0">
                  <IconAction label={t("collections.field_up")} target={field.name} className={i === 0 ? "invisible" : undefined} onClick={() => onChange(moveField(schema, key, -1))}><ArrowUp className="size-4" /></IconAction>
                  <IconAction label={t("collections.field_down")} target={field.name} className={i === rows.length - 1 ? "invisible" : undefined} onClick={() => onChange(moveField(schema, key, 1))}><ArrowDown className="size-4" /></IconAction>
                  {!advanced && <IconAction label={t("collections.field_edit")} target={field.name} onClick={() => setEditing(key)}><Pencil className="size-4" /></IconAction>}
                  <IconAction label={t("collections.field_remove")} target={field.name} className="text-destructive" onClick={() => { if (editing === key) setEditing(null); onChange(removeField(schema, key)); }}><Trash2 className="size-4" /></IconAction>
                </div>
              )}
            </li>
          ))}
        </ul>
      )}
      {editable && (current ? (
        <div className="space-y-1.5">
          <p className="text-sm font-medium">{t("collections.field_editing")} {current.field.name}</p>
          <FieldForm
            key={current.key}
            initial={current.field}
            taken={() => false}
            submitLabel={t("collections.field_save")}
            onSubmit={(f) => { onChange(updateField(schema, current.key, f)); setEditing(null); }}
            onCancel={() => setEditing(null)}
          />
        </div>
      ) : (
        <FieldForm key="new" initial={blank} taken={(name) => keys.has(name)} submitLabel={t("collections.add_field")} onSubmit={(f) => onChange(addField(schema, f))} />
      ))}
    </div>
  );
}
