// @vitest-environment node
import { afterEach, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));
const { refreshSession } = await import("./upstream");

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

it("a request with the old token shortly after the rotation gets the same tokens", async () => {
  vi.useFakeTimers();
  const fetch = vi.fn(async () => Response.json({ access_token: "a2", refresh_token: "r2" }));
  vi.stubGlobal("fetch", fetch);
  const old = { access_token: "a1", refresh_token: "r1" };

  const first = await refreshSession(old, {});
  await vi.advanceTimersByTimeAsync(1_000);
  const late = await refreshSession(old, {});
  expect(late).toEqual(first);
  expect(fetch).toHaveBeenCalledTimes(1);

  await vi.advanceTimersByTimeAsync(10_000);
  await refreshSession(old, {});
  expect(fetch).toHaveBeenCalledTimes(2);
});
