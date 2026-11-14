import { Fragment, type ReactNode } from "react";
import Link from "next/link";

/**
 * The Markdown that long text fields carry (headings, lists, quotes, code, tables,
 * bold, italic, links), drawn as React elements: no HTML from the text reaches the page.
 * shortcut: a small subset with no nesting inside lists, swap for a real parser if fields need more.
 */
export function looksLikeMarkdown(text: string): boolean {
  return /^(#{1,6} |```|> |\|.*\|\s*$|[-*] |\d+\. )/m.test(text) || /\*\*[^*\n]+\*\*|\[[^\]\n]+\]\(https?:\/\//.test(text);
}

/** A link as records hold it: no #fragment, so "page/#part" finds the record of "page/". */
export const linkKey = (href: string) => href.split("#")[0];

type Block =
  | { kind: "heading"; level: number; text: string }
  | { kind: "code"; text: string }
  | { kind: "quote"; text: string }
  | { kind: "list"; ordered: boolean; items: string[] }
  | { kind: "table"; head: string[]; rows: string[][] }
  | { kind: "para"; text: string };

const cells = (line: string) => line.trim().replace(/^\||\|$/g, "").split(/(?<!\\)\|/).map((c) => c.trim().replace(/\\\|/g, "|"));

export function blocksOf(text: string): Block[] {
  const lines = text.replace(/\r\n?/g, "\n").split("\n");
  const out: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) { i++; continue; }
    if (line.startsWith("```")) {
      const body: string[] = [];
      for (i++; i < lines.length && !lines[i].startsWith("```"); i++) body.push(lines[i]);
      out.push({ kind: "code", text: body.join("\n") });
      i++;
      continue;
    }
    const h = /^(#{1,6})\s+(.*)$/.exec(line);
    if (h) { out.push({ kind: "heading", level: h[1].length, text: h[2] }); i++; continue; }
    if (line.startsWith(">")) {
      const body: string[] = [];
      for (; i < lines.length && lines[i].startsWith(">"); i++) body.push(lines[i].replace(/^>\s?/, ""));
      out.push({ kind: "quote", text: body.join("\n") });
      continue;
    }
    if (/^\s*\|.*\|\s*$/.test(line) && /^\s*\|?\s*:?-{3,}/.test(lines[i + 1] ?? "")) {
      const head = cells(line);
      const rows: string[][] = [];
      for (i += 2; i < lines.length && /^\s*\|.*\|\s*$/.test(lines[i]); i++) rows.push(cells(lines[i]));
      out.push({ kind: "table", head, rows });
      continue;
    }
    const item = /^\s*([-*]|\d+\.)\s+/;
    if (item.test(line)) {
      const ordered = /^\s*\d+\./.test(line);
      const items: string[] = [];
      for (; i < lines.length && item.test(lines[i]); i++) items.push(lines[i].replace(item, ""));
      out.push({ kind: "list", ordered, items });
      continue;
    }
    const body: string[] = [];
    for (; i < lines.length && lines[i].trim() && !/^(#{1,6}\s|```|>|\s*([-*]|\d+\.)\s)/.test(lines[i]) && !/^\s*\|.*\|\s*$/.test(lines[i]); i++) body.push(lines[i]);
    if (!body.length) body.push(lines[i++]); // a line no rule claims stays text
    out.push({ kind: "para", text: body.join("\n") });
  }
  return out;
}

/**
 * `code`, **bold**, *italic* and [links](https://…); anything else is plain text.
 * `links` maps addresses to records: those links open the record here.
 */
export function inline(text: string, links?: Map<string, string>): ReactNode[] {
  const out: ReactNode[] = [];
  const re = /`([^`]+)`|\*\*([^*]+)\*\*|\*([^*\s][^*]*)\*|\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g;
  let last = 0;
  for (const m of text.matchAll(re)) {
    if (m.index > last) out.push(text.slice(last, m.index));
    const k = out.length;
    if (m[1] !== undefined) out.push(<code key={k} className="rounded bg-muted px-1 py-0.5 font-mono text-[0.85em]">{m[1]}</code>);
    else if (m[2] !== undefined) out.push(<strong key={k} className="font-semibold">{inline(m[2], links)}</strong>);
    else if (m[3] !== undefined) out.push(<em key={k}>{inline(m[3], links)}</em>);
    else if (links?.has(linkKey(m[5]))) out.push(<Link key={k} href={`/records/${links.get(linkKey(m[5]))}`} title={m[5]} className="text-primary underline-offset-2 hover:underline">{m[4]}</Link>);
    else out.push(<a key={k} href={m[5]} target="_blank" rel="noreferrer noopener" className="text-primary underline-offset-2 hover:underline">{m[4]}</a>);
    last = m.index + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

const lineBreaks = (text: string, links?: Map<string, string>) => text.split("\n").map((l, i) => <Fragment key={i}>{i > 0 && <br />}{inline(l, links)}</Fragment>);

export function Markdown({ text, links }: { text: string; links?: Map<string, string> }) {
  return (
    <div className="space-y-3 text-sm leading-relaxed break-words">
      {blocksOf(text).map((b, i) => {
        switch (b.kind) {
          case "heading": {
            const size = b.level <= 2 ? "text-base" : "text-sm";
            return <p key={i} role="heading" aria-level={b.level + 2} className={`${size} pt-1 font-semibold`}>{inline(b.text, links)}</p>;
          }
          case "code": return <pre key={i} className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs leading-relaxed">{b.text}</pre>;
          case "quote": return <blockquote key={i} className="border-l-2 pl-3 text-muted-foreground">{lineBreaks(b.text, links)}</blockquote>;
          case "list": {
            const Tag = b.ordered ? "ol" : "ul";
            return <Tag key={i} className={`space-y-1 pl-5 ${b.ordered ? "list-decimal" : "list-disc"}`}>{b.items.map((it, j) => <li key={j}>{inline(it, links)}</li>)}</Tag>;
          }
          case "table": return (
            <div key={i} className="overflow-x-auto rounded-md border">
              <table className="w-full text-left text-xs">
                <thead className="bg-muted/50"><tr>{b.head.map((c, j) => <th key={j} className="px-2.5 py-1.5 font-medium">{inline(c, links)}</th>)}</tr></thead>
                <tbody className="divide-y">{b.rows.map((r, j) => <tr key={j}>{r.map((c, k) => <td key={k} className="px-2.5 py-1.5 align-top">{inline(c, links)}</td>)}</tr>)}</tbody>
              </table>
            </div>
          );
          default: return <p key={i}>{lineBreaks(b.text, links)}</p>;
        }
      })}
    </div>
  );
}
