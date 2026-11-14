import { describe, expect, it } from "vitest";
import { blocksOf, inline, looksLikeMarkdown } from "./markdown";

describe("markdown", () => {
  it("tells Markdown from plain text", () => {
    expect(looksLikeMarkdown("## Como funciona\nTexto")).toBe(true);
    expect(looksLikeMarkdown("Veja **isto**")).toBe(true);
    expect(looksLikeMarkdown("Uma linha comum, com 3 * 4 = 12.")).toBe(false);
  });

  it("splits the blocks", () => {
    const text = "# Título\n\nUm parágrafo\ncontinua.\n\n- um\n- dois\n\n| a | b |\n| --- | --- |\n| 1 | x \\| y |\n\n```pseudowired\nQuando\n```\n> nota";
    expect(blocksOf(text)).toEqual([
      { kind: "heading", level: 1, text: "Título" },
      { kind: "para", text: "Um parágrafo\ncontinua." },
      { kind: "list", ordered: false, items: ["um", "dois"] },
      { kind: "table", head: ["a", "b"], rows: [["1", "x | y"]] },
      { kind: "code", text: "Quando" },
      { kind: "quote", text: "nota" },
    ]);
  });

  it("keeps links to http(s) only", () => {
    const parts = inline("[ok](https://a.b) e [não](javascript:alert(1))");
    expect(parts).toHaveLength(2);
    expect(parts[1]).toBe(" e [não](javascript:alert(1))");
  });
});
