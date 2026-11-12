import { describe, expect, it } from "vitest";
import { passwordScore } from "./password";

describe("passwordScore", () => {
  it("grades by length and variety", () => {
    expect(passwordScore("short")).toBe(0);
    expect(passwordScore("abcdefgh")).toBe(1);
    expect(passwordScore("abcdefghijkl")).toBe(2);
    expect(passwordScore("Abcdefghijkl")).toBe(3);
    expect(passwordScore("Abcdefghijk1!")).toBe(4);
  });
});
