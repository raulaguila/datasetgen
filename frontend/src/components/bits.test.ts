import { describe, expect, it } from "vitest";
import { initials } from "./bits";

describe("initials", () => {
  it("takes the first and last word, skipping dashes", () => {
    expect(initials("Wired Club – guias")).toBe("WG");
    expect(initials("Wired Club – referência")).toBe("WR");
    expect(initials("Raul del Aguila")).toBe("RA");
  });
  it("uses two letters of a single word", () => {
    expect(initials("dataset")).toBe("DA");
    expect(initials("  ")).toBe("");
  });
});
