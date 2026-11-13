import { describe, expect, it } from "vitest";
import { parseXlsx } from "@/lib/xlsx";

async function deflate(data: Uint8Array): Promise<Uint8Array> {
  const stream = new ReadableStream<BufferSource>({ start(c) { c.enqueue(data as Uint8Array<ArrayBuffer>); c.close(); } }).pipeThrough(new CompressionStream("deflate-raw"));
  const parts: Uint8Array[] = [];
  const reader = stream.getReader();
  for (let r = await reader.read(); !r.done; r = await reader.read()) parts.push(r.value);
  return new Uint8Array(parts.flatMap((p) => [...p]));
}

/** A minimal zip (no CRCs: the reader does not check them). */
async function zip(files: Record<string, string>, compress: boolean): Promise<ArrayBuffer> {
  const enc = new TextEncoder();
  const locals: number[] = [];
  const central: number[] = [];
  const u16 = (n: number) => [n & 255, (n >> 8) & 255];
  const u32 = (n: number) => [...u16(n & 0xffff), ...u16(n >>> 16)];
  for (const [name, text] of Object.entries(files)) {
    const raw = enc.encode(text);
    const data = compress ? await deflate(raw) : raw;
    const n = enc.encode(name);
    const offset = locals.length;
    const head = [...u16(20), ...u16(0), ...u16(compress ? 8 : 0), ...u32(0), ...u32(0), ...u32(data.length), ...u32(raw.length), ...u16(n.length)];
    locals.push(...u32(0x04034b50), ...head, ...u16(0), ...n, ...data);
    central.push(...u32(0x02014b50), ...u16(20), ...head, ...u16(0), ...u16(0), ...u16(0), ...u16(0), ...u32(0), ...u32(offset), ...n);
  }
  const count = Object.keys(files).length;
  const end = [...u32(0x06054b50), ...u16(0), ...u16(0), ...u16(count), ...u16(count), ...u32(central.length), ...u32(locals.length), ...u16(0)];
  return new Uint8Array([...locals, ...central, ...end]).buffer;
}

const files = {
  "xl/sharedStrings.xml": `<sst><si><t>Name</t></si><si><t>Price</t></si><si><r><t>Caf</t></r><r><t xml:space="preserve">é &amp; pão</t></r></si></sst>`,
  "xl/worksheets/sheet1.xml": `<worksheet><sheetData>
    <row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="C1" t="inlineStr"><is><t>Ok</t></is></c></row>
    <row r="2"><c r="A2" t="s"><v>2</v></c><c r="B2"><v>3.5</v></c><c r="D2" t="b"><v>1</v></c></row>
    <row r="3" spans="1:3"/>
  </sheetData></worksheet>`,
};

describe("parseXlsx", () => {
  for (const compress of [false, true]) {
    it(`reads the first sheet (${compress ? "deflated" : "stored"})`, async () => {
      expect(await parseXlsx(await zip(files, compress))).toEqual([
        ["Name", "Price", "Ok"],
        ["Café & pão", "3.5", "", "true"],
      ]);
    });
  }

  it("refuses a file that is not a zip", async () => {
    await expect(parseXlsx(new TextEncoder().encode("a,b\n1,2").buffer)).rejects.toThrow("not a zip");
  });
});
