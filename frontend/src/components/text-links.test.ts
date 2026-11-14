import { describe, expect, it } from "vitest";
import { chunks, linkTargets } from "./text-links";

describe("record links", () => {
  it("finds each Markdown link once, without its #part, nested values too", () => {
    const data = { a: "see [A](https://x.test/a/) and [A again](https://x.test/a/#top)", b: [{ c: "[B](https://x.test/b/)" }], d: "https://x.test/plain" };
    expect(linkTargets(data)).toEqual(["https://x.test/a/", "https://x.test/b/"]);
  });

  it("splits values into lists that fit a rule", () => {
    const v = Array.from({ length: 5 }, (_, i) => `https://x.test/${i}`);
    expect(chunks(v, 60).map((c) => c.length)).toEqual([3, 2]);
    expect(chunks(v).flat()).toEqual(v);
  });
});
