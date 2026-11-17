/**
 * Reads the first sheet of an .xlsx file into rows of text cells, like
 * parseCsv. No dependency: the zip is read by hand and inflated with the
 * browser's DecompressionStream. The sheet streams: only the zip's directory,
 * the shared strings (which the format needs whole) and one piece of the sheet
 * are in memory at a time. Rows with no filled cell are dropped, as in CSV.
 * shortcut: reads xl/worksheets/sheet1.xml (the first sheet in every common writer) and ignores styles, so dates arrive as Excel serial numbers (cellValue converts them for date fields); follow workbook.xml rels if a file ever stores its sheets elsewhere.
 * shortcut: no zip64, so an .xlsx over 4 GB is refused; CSV has no such limit.
 */
export async function* xlsxRows(file: Blob): AsyncGenerator<string[]> {
  const entries = await directory(file);
  const sheet = entries.get("xl/worksheets/sheet1.xml");
  if (!sheet) throw new Error("xlsx: no first sheet");
  const sharedEntry = entries.get("xl/sharedStrings.xml");
  let shared: string[] = [];
  if (sharedEntry) {
    let xml = "";
    for await (const piece of entryText(file, sharedEntry)) xml += piece;
    shared = [...xml.matchAll(/<si>([\s\S]*?)<\/si>/g)].map((m) => texts(m[1]));
  }
  let buf = "";
  for await (const piece of entryText(file, sheet)) {
    buf += piece;
    // Whole rows only: a row ends with </row> or is one self-closing tag.
    let cut = 0;
    const re = /<row\b[^>]*?(?:\/>|>([\s\S]*?)<\/row>)/g;
    for (let m = re.exec(buf); m; m = re.exec(buf)) {
      cut = re.lastIndex;
      const row = cells(m[1] ?? "", shared);
      if (row.some((c) => c !== "")) yield row;
    }
    buf = buf.slice(cut);
  }
}

/** Every row of the first sheet; xlsxRows without the streaming. */
export async function parseXlsx(buf: ArrayBuffer): Promise<string[][]> {
  const rows: string[][] = [];
  for await (const row of xlsxRows(new Blob([buf]))) rows.push(row);
  return rows;
}

function cells(xml: string, shared: string[]): string[] {
  const out: string[] = [];
  let next = 0;
  for (const c of xml.matchAll(/<c\b([^>]*?)(?:\/>|>([\s\S]*?)<\/c>)/g)) {
    const ref = /\br="([A-Z]+)\d+"/.exec(c[1])?.[1];
    const col = ref ? colIndex(ref) : next;
    next = col + 1;
    const type = /\bt="(\w+)"/.exec(c[1])?.[1];
    const body = c[2] ?? "";
    const v = /<v>([\s\S]*?)<\/v>/.exec(body)?.[1];
    let value = "";
    if (type === "s") value = shared[Number(v)] ?? "";
    else if (type === "inlineStr") value = texts(body);
    else if (type === "b") value = v === "1" ? "true" : "false";
    else if (v !== undefined) value = decode(v);
    while (out.length < col) out.push("");
    out[col] = value;
  }
  return out;
}

const texts = (xml: string) => [...xml.matchAll(/<t\b[^>]*>([\s\S]*?)<\/t>/g)].map((m) => decode(m[1])).join("");

const decode = (s: string) =>
  s.replace(/&(lt|gt|quot|apos|amp|#\d+|#x[0-9a-f]+);/gi, (_, e: string) =>
    e[0] === "#" ? String.fromCodePoint(e[1] === "x" || e[1] === "X" ? parseInt(e.slice(2), 16) : Number(e.slice(1))) : ({ lt: "<", gt: ">", quot: '"', apos: "'", amp: "&" } as Record<string, string>)[e.toLowerCase()]);

function colIndex(letters: string): number {
  let n = 0;
  for (const ch of letters) n = n * 26 + ch.charCodeAt(0) - 64;
  return n - 1;
}

type Entry = { method: number; size: number; local: number };

/** The zip's entries by name, from its central directory: the file's tail is all that is read. */
async function directory(file: Blob): Promise<Map<string, Entry>> {
  const bytes = async (from: number, to: number) => new Uint8Array(await file.slice(from, to).arrayBuffer());
  const tailStart = Math.max(0, file.size - 22 - 65535); // the end record, after a comment of up to 64 KB
  const tail = await bytes(tailStart, file.size);
  const tv = new DataView(tail.buffer, tail.byteOffset, tail.byteLength);
  let end = tail.length - 22;
  while (end >= 0 && tv.getUint32(end, true) !== 0x06054b50) end--;
  if (end < 0) throw new Error("xlsx: not a zip file");
  const count = tv.getUint16(end + 10, true);
  const dirSize = tv.getUint32(end + 12, true);
  const dirAt = tv.getUint32(end + 16, true);
  const dir = await bytes(dirAt, dirAt + dirSize);
  const view = new DataView(dir.buffer, dir.byteOffset, dir.byteLength);
  const utf8 = new TextDecoder();
  const out = new Map<string, Entry>();
  let p = 0;
  for (let i = 0; i < count; i++) {
    if (view.getUint32(p, true) !== 0x02014b50) throw new Error("xlsx: broken zip");
    const nameLen = view.getUint16(p + 28, true);
    out.set(utf8.decode(dir.subarray(p + 46, p + 46 + nameLen)), {
      method: view.getUint16(p + 10, true),
      size: view.getUint32(p + 20, true),
      local: view.getUint32(p + 42, true),
    });
    p += 46 + nameLen + view.getUint16(p + 30, true) + view.getUint16(p + 32, true);
  }
  return out;
}

/** One entry's text, inflated as it is read. */
async function* entryText(file: Blob, e: Entry): AsyncGenerator<string> {
  const head = new DataView(await file.slice(e.local, e.local + 30).arrayBuffer());
  const start = e.local + 30 + head.getUint16(26, true) + head.getUint16(28, true);
  const raw = file.slice(start, start + e.size);
  // Test doubles' Blob has no stream(): one piece then.
  const source = typeof raw.stream === "function" ? raw.stream()
    : new ReadableStream<Uint8Array>({ async start(c) { c.enqueue(new Uint8Array(await raw.arrayBuffer())); c.close(); } });
  let stream: ReadableStream<Uint8Array>;
  if (e.method === 0) stream = source;
  else if (e.method === 8) stream = (source as ReadableStream<BufferSource>).pipeThrough(new DecompressionStream("deflate-raw")) as ReadableStream<Uint8Array>;
  else throw new Error("xlsx: unsupported compression");
  const reader = stream.getReader();
  const utf8 = new TextDecoder();
  for (let r = await reader.read(); !r.done; r = await reader.read()) yield utf8.decode(r.value, { stream: true });
  const rest = utf8.decode();
  if (rest) yield rest;
}
