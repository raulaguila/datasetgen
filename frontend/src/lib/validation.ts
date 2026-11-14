// Live field checks: the backend rules a form can show before sending. Each rule
// says nothing about an empty value; `ready` blocks submitting that.
import type { MessageKey } from "@/lib/i18n/messages";

export type Rule = (value: string) => MessageKey | null;

const chars = (v: string) => [...v.trim()].length;
const bytes = (v: string) => new TextEncoder().encode(v).length;
const filled = (v: string) => v.trim() !== "";

export const personName: Rule = (v) => (filled(v) && (chars(v) < 2 || chars(v) > 80) ? "error.msg.person_name" : null);
export const email: Rule = (v) => (filled(v) && !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim()) ? "error.msg.email" : null);
export const collectionName: Rule = (v) => (chars(v) > 120 ? "error.msg.collection_name" : null);
export const description: Rule = (v) => (chars(v) > 2000 ? "error.msg.description" : null);
// Dot or comma: the form shows the language's decimal mark.
export const price: Rule = (v) =>
  !filled(v) ? null : v.trim().startsWith("-") ? "error.msg.price_negative" : !/^\d{1,10}([.,]\d{1,2})?$/.test(v.trim()) ? "error.msg.price_format" : null;
export const roleName: Rule = (v) => (filled(v) && !/^[a-z0-9_-]{2,40}$/.test(v.trim()) ? "error.msg.role_name" : null);
// Common passwords and the account's own name stay a server check.
export const newPassword: Rule = (v) =>
  v === "" ? null : bytes(v) < 8 ? "error.msg.password_short" : bytes(v) > 128 ? "error.msg.password_long" : null;
export const sameAs = (other: string): Rule => (v) => (v !== "" && v !== other ? "auth.password_mismatch" : null);

/** Every value filled and no rule broken: the form may submit. */
export const ready = (...fields: [string, Rule?][]) => fields.every(([v, rule]) => filled(v) && !rule?.(v));
