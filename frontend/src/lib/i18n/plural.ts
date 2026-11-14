/**
 * Fills "{n}" and settles the "(s)" endings a message writes for both counts:
 * "{n} registro(s) válido(s)" reads "1 registro válido" or "3 registros válidos".
 * "(ns)" turns a final "m" into "ns" (item → itens), "(ões)" a final "ão" into "ões"; "(es)" appends "es".
 */
export function plural(text: string, n: number, shown: string = String(n)): string {
  return text
    .replaceAll("{n}", shown)
    .replace(/(\p{L}+)\((s|es|ns|ões)\)/gu, (_, stem: string, end: string) => {
      if (n === 1) return stem;
      if (end === "ns") return stem.replace(/m$/, "") + "ns";
      return end === "ões" ? stem.replace(/ão$/, "ões") : stem + end;
    });
}
