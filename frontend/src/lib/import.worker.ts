/// <reference lib="webworker" />
import { headerOf, inferFile, recordBatches } from "@/lib/import-stream";
import type { WorkerReply, WorkerRequest } from "@/lib/import-client";

// Reads an import file off the page's thread, so the page keeps answering
// while a large file is parsed. Batches wait for "next": one at a time in memory.
const post = (m: WorkerReply) => postMessage(m);
let resume: (() => void) | null = null;
let last = 0;
const progress = (read: number) => {
  const now = Date.now();
  if (now - last > 150) { last = now; post({ type: "progress", read }); }
};

addEventListener("message", async (e: MessageEvent<WorkerRequest>) => {
  const m = e.data;
  if (m.op === "next") { resume?.(); return; }
  try {
    if (m.op === "header") post({ type: "header", header: await headerOf(m.file) });
    else if (m.op === "infer") post({ type: "schema", schema: await inferFile(m.file, progress) });
    else {
      for await (const batch of recordBatches(m.file, m.opts, progress)) {
        post({ type: "batch", batch });
        await new Promise<void>((r) => { resume = r; });
      }
      post({ type: "end" });
    }
  } catch (err) {
    post({ type: "error", message: err instanceof Error ? err.message : String(err) });
  }
});
