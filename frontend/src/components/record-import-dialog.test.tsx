import { importId, loadProgress } from "@/lib/import-client";
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

it("replays the first unacknowledged batch after reopening the dialog", async () => {
  const schema = { type: "object", properties: { sku: { type: "string", "x-unique": true } } };
  const file = new File(['{"sku":"A"}'], "items.jsonl");
  Object.defineProperty(file, "text", { value: async () => '{"sku":"A"}' });
  const id = importId("resume-test", schema, file, undefined, undefined);
  const onClose = vi.fn();
  const mount = () => render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nProvider initial="pt-BR">
        <RecordImportDialog collectionId="resume-test" schema={schema} open onOpenChange={onClose} />
      </I18nProvider>
    </QueryClientProvider>,
  );
  const pick = () => fireEvent.change(document.getElementById("import-file")!, { target: { files: [file] } });
  let pendingKey: string | undefined;
  const request = vi.spyOn(records, "importBatch").mockImplementation(async (body) => {
    if (body.dry_run) return { created: 0, updated: 0, invalid: 0, errors: [] };
    const checkpoint = loadProgress(id);
    expect(checkpoint?.pending).toBe(0);
    expect(body.batch_key).toBe(`${checkpoint?.fid}:0`);
    if (!pendingKey) {
      pendingKey = body.batch_key;
      throw new Error("acknowledgement lost");
    }
    expect(body.batch_key).toBe(pendingKey);
    return { created: 1, updated: 0, invalid: 0, errors: [], replayed: true };
  });
  const first = mount();
  pick();
  fireEvent.click(await screen.findByRole("button", { name: "Importar 1" }));
  await waitFor(() => expect(pendingKey).toBeTruthy());
  first.unmount();
  mount();
  pick();
  fireEvent.click(await screen.findByRole("button", { name: /retomar|continuar/i }));
  await waitFor(() => expect(onClose).toHaveBeenCalledWith(false));
  expect(request).toHaveBeenCalledTimes(3);
  expect(loadProgress(id)).toBeNull();
});
