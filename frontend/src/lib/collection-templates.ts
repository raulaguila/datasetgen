import type { Field } from "@/components/field-builder";
import type { Locale } from "@/lib/i18n/messages";

/** "basic" is the starter schema every new collection opens with; the rest are built here. */
export const TEMPLATE_IDS = ["basic", "blank", "contacts", "tasks", "inventory"] as const;
export type TemplateId = (typeof TEMPLATE_IDS)[number];
type Built = Exclude<TemplateId, "basic">;

type Fields = Partial<Field>[];

/** Each template's fields in each language: keys come from the labels, so they read in it too. */
const TEMPLATES: Record<Locale, Record<Built, Fields>> = {
  "pt-BR": {
    blank: [],
    contacts: [
      { name: "Nome", required: true },
      { name: "E-mail", type: "email", unique: true },
      { name: "Telefone" },
      { name: "Empresa" },
      { name: "Observações", type: "text" },
    ],
    tasks: [
      { name: "Título", required: true },
      { name: "Situação", type: "enum", options: "A fazer, Fazendo, Feito", default: "A fazer", required: true },
      { name: "Prazo", type: "date" },
      { name: "Responsável" },
      { name: "Descrição", type: "text" },
    ],
    inventory: [
      { name: "Item", required: true },
      { name: "SKU", unique: true },
      { name: "Quantidade", type: "integer", min: "0" },
      { name: "Preço", type: "number", min: "0" },
      { name: "Local" },
    ],
  },
  "en-US": {
    blank: [],
    contacts: [
      { name: "Name", required: true },
      { name: "Email", type: "email", unique: true },
      { name: "Phone" },
      { name: "Company" },
      { name: "Notes", type: "text" },
    ],
    tasks: [
      { name: "Title", required: true },
      { name: "Status", type: "enum", options: "To do, Doing, Done", default: "To do", required: true },
      { name: "Due", type: "date" },
      { name: "Owner" },
      { name: "Description", type: "text" },
    ],
    inventory: [
      { name: "Item", required: true },
      { name: "SKU", unique: true },
      { name: "Quantity", type: "integer", min: "0" },
      { name: "Price", type: "number", min: "0" },
      { name: "Location" },
    ],
  },
};

export const templateFields = (locale: Locale, id: Built): Fields => TEMPLATES[locale][id];
