import { ApiError } from "@/lib/api/client";
import { headerOf, inferFile, recordBatches, type Batch } from "@/lib/import-stream";

/** Messages to and from lib/import.worker.ts. */
type BatchOpts = { schema: Record<string, unknown> | null; columns?: string[]; upsertKey?: string };
export type WorkerRequest =
  | { op: "header"; file: File }
  | { op: "infer"; file: File }
  | { op: "batches"; file: File; opts: BatchOpts }
  | { op: "next" };
export type WorkerReply =
  | { type: "progress"; read: number }
  | { type: "header"; header: string[] }
  | { type: "schema"; schema: Record<string, unknown> | null }
  | { type: "batch"; batch: Batch }
  | { type: "end" }
  | { type: "error"; message: string };

/** Thrown when the person cancels; callers stop quietly. */
export class Cancelled extends Error {
  constructor() { super("cancelled"); }
}

const hasWorker = () => typeof Worker !== "undefined";
const spawn = () => new Worker(new URL("./import.worker.ts", import.meta.url), { type: "module" });

/** Runs one request in a fresh worker; cancelling the signal ends the worker at once. */
function run(req: Exclude<WorkerRequest, { op: "next" }>, signal: AbortSignal | undefined, onReply: (m: WorkerReply, next: () => void) => void | Promise<void>): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) return reject(new Cancelled());
    const w = spawn();
    const done = (err?: unknown) => { w.terminate(); signal?.removeEventListener("abort", abort); if (err) reject(err); else resolve(); };
    const abort = () => done(new Cancelled());
    signal?.addEventListener("abort", abort);
    w.onerror = (e) => done(new Error(e.message || "worker failed"));
    w.onmessage = async (e: MessageEvent<WorkerReply>) => {
      const m = e.data;
      if (m.type === "error") return done(new Error(m.message));
      try {
        await onReply(m, () => w.postMessage({ op: "next" } satisfies WorkerRequest));
      } catch (err) {
        return done(err);
      }
      if (m.type === "header" || m.type === "schema" || m.type === "end") done();
    };
    w.postMessage(req);
  });
}

/** The header row of a CSV or sheet. */
export async function readHeader(file: File, signal?: AbortSignal): Promise<string[]> {
  if (!hasWorker()) return headerOf(file);
  let header: string[] = [];
  await run({ op: "header", file }, signal, (m) => { if (m.type === "header") header = m.header; });
  return header;
}

/** Fields guessed from the whole file; progress gets the bytes read. */
export async function inferSchema(file: File, progress?: (read: number) => void, signal?: AbortSignal): Promise<Record<string, unknown> | null> {
  if (!hasWorker()) return inferFile(file, progress);
  let schema: Record<string, unknown> | null = null;
  await run({ op: "infer", file }, signal, (m) => {
    if (m.type === "progress") progress?.(m.read);
    if (m.type === "schema") schema = m.schema;
  });
  return schema;
}

/**
 * Hands the file's batches to each in turn: the next batch is read only once
 * each has finished with this one, so memory holds one batch whatever the file's size.
 */
export async function eachBatch(file: File, opts: BatchOpts, each: (b: Batch) => Promise<void>, progress?: (read: number) => void, signal?: AbortSignal): Promise<void> {
  if (!hasWorker()) {
    for await (const b of recordBatches(file, opts, progress)) {
      if (signal?.aborted) throw new Cancelled();
      await each(b);
    }
    return;
  }
  await run({ op: "batches", file, opts }, signal, async (m, next) => {
    if (m.type === "progress") progress?.(m.read);
    if (m.type === "batch") { await each(m.batch); next(); }
  });
}

/** Why a request is being tried again: the per-minute limit, or a failure that may pass (network, 5xx). */
export type RetryWait = { seconds: number; reason: "limit" | "failure" };

const transient = (err: unknown) =>
  err instanceof TypeError || (err instanceof ApiError && (err.status === 408 || err.status >= 500));

/**
 * Calls fn again while the API answers 429 (after the wait it asks for) or
 * fails in a way that may pass, after a growing pause. Only for requests that
 * are safe to repeat: a dry run, or a batch sent with its batch_key.
 * onWait shows the seconds left; 0 when the wait ends.
 */
export async function withRetry<T>(fn: () => Promise<T>, onWait: (w: RetryWait | null) => void, signal?: AbortSignal): Promise<T> {
  let failures = 0;
  for (let limited = 0; ; ) {
    try {
      return await fn();
    } catch (err) {
      if (signal?.aborted) throw new Cancelled();
      let wait: RetryWait;
      if (err instanceof ApiError && err.status === 429 && limited++ < 30) wait = { seconds: Math.min(err.retryAfter ?? 5, 60), reason: "limit" };
      else if (transient(err) && failures++ < 8) wait = { seconds: Math.min(2 ** failures, 60), reason: "failure" };
      else throw err;
      for (let s = wait.seconds; s > 0; s--) {
        if (signal?.aborted) throw new Cancelled();
        onWait({ ...wait, seconds: s });
        await new Promise((r) => setTimeout(r, 1000));
      }
      onWait(null);
    }
  }
}

/**
 * Where a send of one file into one collection stopped, kept in this browser so
 * choosing the same file again continues it: fid names its batches' keys, sent
 * counts the batches the server confirmed. A week, like the server's keys.
 */
export type SendProgress = { fid: string; sent: number; created: number; updated: number; at: number };
const PROGRESS = "dataset.import-progress";
const KEPT = 7 * 24 * 3600 * 1000;

/** One import's identity: same collection, file, columns and upsert key give the same batches. */
export const importId = (collectionId: string, f: File, columns: string[] | undefined, upsertKey: string | undefined) =>
  JSON.stringify([collectionId, f.name, f.size, f.lastModified, columns ?? null, upsertKey ?? null]);

function all(): Record<string, SendProgress> {
  try {
    const v = JSON.parse(localStorage.getItem(PROGRESS) ?? "{}") as Record<string, SendProgress>;
    return Object.fromEntries(Object.entries(v).filter(([, p]) => Date.now() - p.at < KEPT));
  } catch {
    return {};
  }
}

export function loadProgress(id: string): SendProgress | null {
  return all()[id] ?? null;
}

export function saveProgress(id: string, p: SendProgress | null) {
  try {
    const v = all();
    if (p) v[id] = { ...p, at: Date.now() };
    else delete v[id];
    localStorage.setItem(PROGRESS, JSON.stringify(v));
  } catch { /* private mode: the send still runs, it just cannot resume */ }
}
