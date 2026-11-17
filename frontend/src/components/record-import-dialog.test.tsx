import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { Validator } from "@cfworker/json-schema";
import { RecordImportDialog } from "./record-import-dialog";
import { SchemaForm } from "./schema-form";
import { records } from "@/lib/api/endpoints";
import { schemaFromFile } from "@/lib/infer-schema";
import { I18nProvider } from "@/lib/i18n/provider";

afterEach(() => vi.restoreAllMocks());

it("shows inferred subfields and sends objects and lists intact through validation and import", async () => {
  const data = { grupo: { nome: "Ana", ativo: false }, itens: [{ chave: "delay", qtd: 0, opcoes: ["a", "b"] }] };
  const csv = Object.keys(data).join(",") + "\n" + Object.values(data).map((v) => '"' + JSON.stringify(v).replaceAll('"', '""') + '"').join(",");
  const schema = schemaFromFile("dados.csv", csv)!;
  const validator = new Validator(schema, "2020-12", false);
  const importBatch = vi.spyOn(records, "importBatch").mockImplementation(async (body) => {
    expect(body.records).toEqual([data]);
    expect(body.records.every((r) => validator.validate(r).valid)).toBe(true);
    return { created: body.dry_run ? 0 : 1, updated: 0, invalid: 0, errors: [] };
  });
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <I18nProvider initial="pt-BR">
        <SchemaForm schema={schema} data={data} />
        <RecordImportDialog collectionId="collection" schema={schema} open onOpenChange={onOpenChange} />
      </I18nProvider>
    </QueryClientProvider>,
  );
  expect(screen.getByDisplayValue("Ana")).toBeInTheDocument();
  expect(screen.getByDisplayValue("delay")).toBeInTheDocument();
  const file = new File([csv], "dados.csv", { type: "text/csv" });
  Object.defineProperty(file, "text", { value: async () => csv });
  fireEvent.change(document.getElementById("import-file")!, { target: { files: [file] } });
  await waitFor(() => expect(screen.getByRole("button", { name: "Importar 1" })).toBeEnabled());
  expect(importBatch).toHaveBeenNthCalledWith(1, expect.objectContaining({ dry_run: true, records: [data] }));
  fireEvent.click(screen.getByRole("button", { name: "Importar 1" }));
  await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  expect(importBatch).toHaveBeenNthCalledWith(2, expect.objectContaining({ skip_invalid: true, records: [data] }));
});
