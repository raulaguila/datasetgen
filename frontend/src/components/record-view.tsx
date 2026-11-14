"use client";

import Link from "next/link";
import { Check, ExternalLink, FileIcon } from "lucide-react";
import { useFormatDate } from "@/components/bits";
import { FILE, REF, fileRefs, isImage } from "@/components/field-builder";
import { Markdown, looksLikeMarkdown } from "@/components/markdown";
import { useNumberFormat } from "@/components/record-groups";
import { useRecordLabel } from "@/components/schema-form";
import { tools } from "@/lib/api/endpoints";
import type { Collection, DataRecord } from "@/lib/api/types";
import { useI18n } from "@/lib/i18n/provider";
import { cn } from "@/lib/utils";

type S = Record<string, unknown>;
const obj = (v: unknown): S => (v && typeof v === "object" && !Array.isArray(v) ? (v as S) : {});
const propsOf = (s: S) => obj(s.properties);
const titleOf = (s: S, key: string) => (typeof s.title === "string" && s.title ? s.title : key);
/** Fields that name the object they sit in, by key or title. */
const NAMING = /^(r[oó]tulo|label|t[ií]tulo|title|nome|name)$/i;

export const isEmpty = (v: unknown) =>
  v === undefined || v === null || v === "" || (Array.isArray(v) && v.length === 0) || (typeof v === "object" && !Array.isArray(v) && Object.keys(v as S).length === 0);

/** A value that reads in one line: it goes in the summary grid; the rest gets a section of its own. */
export function isShort(s: S, v: unknown): boolean {
  if (typeof v === "string") return !(typeof s.maxLength === "number" && s.maxLength >= 500) && v.length <= 160 && !v.includes("\n");
  if (Array.isArray(v)) return s[REF] === undefined && s[FILE] === undefined && v.length <= 6 && v.every((x) => typeof x !== "object");
  return typeof v !== "object" || v === null;
}

/** The header shows these as chips: the record's unique values and its picks from a list. */
export function isChip(s: S, v: unknown): boolean {
  // A link stays a link in the summary, never a plain chip.
  return typeof v === "string" && v !== "" && s.format !== "uri" && (s["x-unique"] === true || Array.isArray(s.enum));
}

export function Chip({ children, mono, strong, title }: { children: React.ReactNode; mono?: boolean; strong?: boolean; title?: string }) {
  return (
    <span title={title} className={cn("inline-flex max-w-full items-center truncate rounded-md border px-2 py-0.5 text-xs",
      strong ? "border-primary/30 bg-primary/10 text-primary" : "bg-muted/50", mono && "font-mono")}>{children}</span>
  );
}

function RefLink({ target, id }: { target: string; id: string }) {
  const label = useRecordLabel(target, id);
  return <Link href={`/records/${id}`} className="truncate text-primary underline-offset-2 hover:underline">{label}</Link>;
}

function Files({ value }: { value: unknown }) {
  return (
    <ul className="flex flex-wrap gap-2">
      {fileRefs(value).map((f) => (
        <li key={f.id}>
          <a href={tools.fileUrl(f.id)} target="_blank" rel="noopener" className="inline-flex max-w-64 items-center gap-2 rounded-md border px-2 py-1 text-sm hover:underline">
            {isImage(f)
              // eslint-disable-next-line @next/next/no-img-element -- a private file behind the session proxy, not an optimizable asset
              ? <img src={tools.fileUrl(f.id)} alt="" loading="lazy" className="size-8 shrink-0 rounded object-cover" /> : <FileIcon className="size-4 shrink-0" />}
            <span className="truncate">{f.name ?? f.id}</span>
          </a>
        </li>
      ))}
    </ul>
  );
}

/** Any value drawn by what its schema says it is: dates and numbers in the user's locale, links, chips, Markdown, nested groups. */
export function Value({ schema, value }: { schema: S; value: unknown }) {
  const { t, locale } = useI18n();
  const num = useNumberFormat();
  const formatDate = useFormatDate();
  if (isEmpty(value)) return <span className="text-muted-foreground">—</span>;
  if (typeof value === "boolean")
    return value
      ? <span className="inline-flex items-center gap-1"><Check className="size-3.5 text-success" />{t("common.yes")}</span>
      : <span className="text-muted-foreground">{t("common.no")}</span>;
  // Whole numbers are often codes or revisions, where a thousands separator misleads.
  if (typeof value === "number") return <span className="tabular-nums">{schema.type === "integer" ? String(value) : num(value)}</span>;
  if (typeof value === "string") {
    if (typeof schema[REF] === "string") return <RefLink target={schema[REF]} id={value} />;
    if (schema.format === "date") return <span>{new Date(`${value}T00:00:00Z`).toLocaleDateString(locale, { dateStyle: "medium", timeZone: "UTC" })}</span>;
    if (schema.format === "date-time") return <span>{formatDate(value)}</span>;
    if (schema.format === "uri" && /^https?:\/\//.test(value)) {
      const host = (() => { try { return new URL(value).host; } catch { return value; } })();
      return <a href={value} target="_blank" rel="noreferrer noopener" className="inline-flex max-w-full items-center gap-1 text-primary underline-offset-2 hover:underline" title={value}><span className="truncate">{host}</span><ExternalLink className="size-3 shrink-0" /></a>;
    }
    if (schema.format === "email") return <a href={`mailto:${value}`} className="text-primary underline-offset-2 hover:underline">{value}</a>;
    if (Array.isArray(schema.enum)) return <Chip>{value}</Chip>;
    if (!isShort(schema, value)) return looksLikeMarkdown(value) ? <Markdown text={value} /> : <p className="whitespace-pre-wrap text-sm leading-relaxed break-words">{value}</p>;
    return <span className="break-words">{value}</span>;
  }
  if (Array.isArray(value)) {
    if (schema[FILE] === true) return <Files value={value} />;
    const items = obj(schema.items);
    if (typeof schema[REF] === "string") {
      const target = schema[REF];
      return <ul className="flex flex-wrap gap-x-3 gap-y-1">{value.map((id) => <li key={String(id)}><RefLink target={target} id={String(id)} /></li>)}</ul>;
    }
    if (value.every((x) => typeof x !== "object" || x === null))
      return <ul className="flex flex-wrap gap-1.5">{value.map((x, i) => <li key={i}><Chip strong={Array.isArray(items.enum)}>{String(x)}</Chip></li>)}</ul>;
    return (
      <ol className="grid gap-2">
        {value.map((x, i) => <li key={i} className="rounded-lg border p-3"><Group schema={items} value={x} heading /></li>)}
      </ol>
    );
  }
  if (schema[FILE] === true) return <Files value={value} />;
  if (Object.keys(propsOf(schema)).length) return <Group schema={schema} value={value} />;
  return <pre className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">{JSON.stringify(value, null, 2)}</pre>;
}

/** An object's fields as label and value; with `heading`, its first short text names it instead. */
function Group({ schema, value, heading = false }: { schema: S; value: unknown; heading?: boolean }) {
  const data = obj(value);
  const props = propsOf(schema);
  const keys = [...Object.keys(props), ...Object.keys(data).filter((k) => !(k in props))].filter((k) => !isEmpty(data[k]));
  // The item's name leads: a label or title field first, else its first short text.
  const short = (k: string) => typeof data[k] === "string" && isShort(obj(props[k]), data[k]);
  const named = (k: string) => NAMING.test(k) || NAMING.test(titleOf(obj(props[k]), k));
  const lead = heading ? (keys.find((k) => named(k) && short(k)) ?? keys.find(short)) : undefined;
  if (typeof value !== "object" || value === null) return <span>{String(value)}</span>;
  return (
    <div className="space-y-2">
      {lead && <p className="text-sm font-medium break-words">{String(data[lead])}</p>}
      <dl className="grid gap-x-4 gap-y-1.5 text-sm sm:grid-cols-[minmax(6rem,auto)_minmax(0,1fr)]">
        {keys.filter((k) => k !== lead).map((k) => (
          <div key={k} className="contents">
            <dt className="text-muted-foreground">{titleOf(obj(props[k]), k)}</dt>
            <dd className="min-w-0"><Value schema={obj(props[k])} value={data[k]} /></dd>
          </div>
        ))}
      </dl>
    </div>
  );
}

/**
 * A record laid out to be read: short values in a summary grid, then each list,
 * group and long text in a section of its own, in the schema's field order.
 * `skip` names fields the page already shows (its title, the header chips).
 */
export function RecordView({ collection, record, skip }: { collection: Collection; record: DataRecord; skip: Set<string> }) {
  const { t } = useI18n();
  const schema = (collection.schema ?? {}) as S;
  const data = obj(record.data);
  const props = propsOf(schema);
  const keys = [...Object.keys(props), ...Object.keys(data).filter((k) => !(k in props))].filter((k) => !skip.has(k));
  const short = keys.filter((k) => isShort(obj(props[k]), data[k]));
  const blocks = keys.filter((k) => !short.includes(k) && !isEmpty(data[k]));
  // Yes/no fields read as one line: the ones that hold, then the ones that do not.
  const flag = (k: string) => typeof data[k] === "boolean" || obj(props[k]).type === "boolean";
  const plain = short.filter((k) => !flag(k));
  const yes = short.filter((k) => flag(k) && data[k] === true);
  const no = short.filter((k) => flag(k) && data[k] === false);
  return (
    <div className="space-y-4">
      {(plain.length > 0 || yes.length > 0 || no.length > 0) && (
        <section aria-label={t("records.summary")} className="@container rounded-xl border bg-card p-4">
          {plain.length > 0 && (
            <dl className="grid grid-cols-2 gap-x-4 gap-y-3 @lg:grid-cols-3">
              {plain.map((k) => (
                <div key={k} className="min-w-0">
                  <dt className="truncate text-xs text-muted-foreground" title={titleOf(obj(props[k]), k)}>{titleOf(obj(props[k]), k)}</dt>
                  <dd className="mt-0.5 text-sm"><Value schema={obj(props[k])} value={data[k]} /></dd>
                </div>
              ))}
            </dl>
          )}
          {(yes.length > 0 || no.length > 0) && (
            <div className={cn("space-y-1.5 text-sm", plain.length > 0 && "mt-3 border-t pt-3")}>
              {yes.length > 0 && (
                <ul className="flex flex-wrap gap-1.5" aria-label={t("common.yes")}>
                  {yes.map((k) => <li key={k}><Chip strong><Check className="mr-1 size-3" aria-hidden />{titleOf(obj(props[k]), k)}</Chip></li>)}
                </ul>
              )}
              {no.length > 0 && <p className="text-xs text-muted-foreground">{t("common.no")}: {no.map((k) => titleOf(obj(props[k]), k)).join(" · ")}</p>}
            </div>
          )}
        </section>
      )}
      {blocks.map((k) => {
        const s = obj(props[k]);
        return (
          <section key={k} aria-labelledby={`f-${k}`} className="rounded-xl border bg-card p-4">
            <h2 id={`f-${k}`} className="text-sm font-semibold">{titleOf(s, k)}</h2>
            {typeof s.description === "string" && s.description && <p className="mt-0.5 text-xs text-muted-foreground">{s.description}</p>}
            <div className="mt-3"><Value schema={s} value={data[k]} /></div>
          </section>
        );
      })}
    </div>
  );
}
