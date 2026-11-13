/**
 * Reads the first sheet of an .xlsx file into rows of text cells, like
 * parseCsv. No dependency: the zip is read by hand and inflated with the
 * browser's DecompressionStream.
 * shortcut: reads xl/worksheets/sheet1.xml (the first sheet in every common writer) and ignores styles, so dates arrive as Excel serial numbers (cellValue converts them for date fields); follow workbook.xml rels if a file ever stores its sheets elsewhere.
 */
export async function parseXlsx(buf: ArrayBuffer): Promise<string[][]> {
  const files = await unzip(new Uint8Array(buf), ["xl/sharedStrings.xml", "xl/worksheets/sheet1.xml"]);
  const sheet = files.get("xl/worksheets/sheet1.xml");
  if (!sheet) throw new Error("xlsx: no first sheet");
  const shared = [...(files.get("xl/sharedStrings.xml") ?? "").matchAll(/<si>([\s\S]*?)<\/si>/g)].map((m) => texts(m[1]));
  const rows: string[][] = [];
  for (const row of sheet.matchAll(/<row\b[^>]*?(?:\/>|>([\s\S]*?)<\/row>)/g)) {
    const out: string[] = [];
    let next = 0;
    for (const c of (row[1] ?? "").matchAll(/<c\b([^>]*?)(?:\/>|>([\s\S]*?)<\/c>)/g)) {
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
    rows.push(out);
  }
  // Writers leave styled but empty rows at the end.
  while (rows.length && rows[rows.length - 1].every((c) => c === "")) rows.pop();
  return rows;
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

/** The named entries of a zip, as text; read from the central directory. */
async function unzip(b: Uint8Array, names: string[]): Promise<Map<string, string>> {
  const view = new DataView(b.buffer, b.byteOffset, b.byteLength);
  let end = b.length - 22;
  while (end >= 0 && view.getUint32(end, true) !== 0x06054b50) end--;
  if (end < 0) throw new Error("xlsx: not a zip file");
  const count = view.getUint16(end + 10, true);
  let p = view.getUint32(end + 16, true);
  const out = new Map<string, string>();
  const utf8 = new TextDecoder();
  for (let i = 0; i < count; i++) {
    if (view.getUint32(p, true) !== 0x02014b50) throw new Error("xlsx: broken zip");
    const method = view.getUint16(p + 10, true);
    const size = view.getUint32(p + 20, true);
    const nameLen = view.getUint16(p + 28, true);
    const extra = view.getUint16(p + 30, true);
    const comment = view.getUint16(p + 32, true);
    const local = view.getUint32(p + 42, true);
    const name = utf8.decode(b.subarray(p + 46, p + 46 + nameLen));
    p += 46 + nameLen + extra + comment;
    if (!names.includes(name)) continue;
    const start = local + 30 + view.getUint16(local + 26, true) + view.getUint16(local + 28, true);
    const raw = b.subarray(start, start + size);
    if (method === 0) out.set(name, utf8.decode(raw));
    else if (method === 8) out.set(name, utf8.decode(await inflate(raw)));
    else throw new Error("xlsx: unsupported compression");
  }
  return out;
}

async function inflate(raw: Uint8Array): Promise<Uint8Array> {
  const stream = new ReadableStream<BufferSource>({ start(c) { c.enqueue(raw as Uint8Array<ArrayBuffer>); c.close(); } }).pipeThrough(new DecompressionStream("deflate-raw"));
  const parts: Uint8Array[] = [];
  const reader = stream.getReader();
  for (let r = await reader.read(); !r.done; r = await reader.read()) parts.push(r.value);
  const out = new Uint8Array(parts.reduce((n, x) => n + x.length, 0));
  let o = 0;
  for (const x of parts) { out.set(x, o); o += x.length; }
  return out;
}
