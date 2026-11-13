import { expect, test, type Page } from "@playwright/test";
import { formAlert, guestPage } from "./helpers";

const api = (page: Page, path: string, data: unknown) => page.request.post(`/api/proxy${path}`, { data });

// Conditions, grouping with totals, editing a cell in place and the board.
test("conditions, groups, inline edit and the board", async ({ page }) => {
  await page.goto("/");
  const schema = {
    type: "object",
    required: ["title"],
    properties: {
      title: { type: "string", title: "Título" },
      status: { title: "Status", enum: ["novo", "feito"] },
      valor: { type: "number", title: "Valor" },
    },
  };
  const made = await api(page, "/v1/collections", { name: `grade-${Date.now()}`, schema });
  expect(made.ok()).toBeTruthy();
  const id = (await made.json()).id as string;
  for (const [title, status, valor] of [["Alfa", "novo", 10], ["Beta", "novo", 30], ["Gama", "feito", 5]] as const) {
    expect((await api(page, "/v1/records", { collection_id: id, data: { title, status, valor } })).ok()).toBeTruthy();
  }
  await page.goto(`/collections/${id}`);
  await expect(page.locator("tbody tr")).toHaveCount(3);

  // A condition: valor greater than 8.
  await page.getByRole("button", { name: "Condições" }).click();
  const rules = page.getByRole("dialog", { name: "Condições" });
  await rules.getByRole("combobox", { name: "Campo da condição 1" }).click();
  await page.getByRole("option", { name: "Valor" }).click();
  await rules.getByRole("combobox", { name: "Operador da condição 1" }).click();
  await page.getByRole("option", { name: "maior que", exact: true }).click();
  await rules.getByLabel("Valor 1").fill("8");
  await rules.getByRole("button", { name: "Aplicar" }).click();
  await expect(page.locator("tbody tr")).toHaveCount(2);
  await expect(page.getByRole("row").filter({ hasText: "Gama" })).toHaveCount(0);
  // Kept in the address, so a reload keeps it.
  await page.reload();
  await expect(page.locator("tbody tr")).toHaveCount(2);
  await page.getByRole("button", { name: /^Remover condição/ }).click();
  await expect(page.locator("tbody tr")).toHaveCount(3);

  // Group by status: counts and sums per group.
  await page.getByRole("combobox", { name: "Agrupar" }).click();
  await page.getByRole("option", { name: "Agrupar: Status" }).click();
  const groups = page.getByRole("region", { name: "Por Status" });
  await expect(groups.getByRole("listitem").filter({ hasText: "novo" })).toContainText("2");
  await expect(groups.getByRole("listitem").filter({ hasText: "novo" })).toContainText("40");

  // Edit a cell in place.
  await page.goto(`/collections/${id}`);
  const cell = page.getByRole("row").filter({ hasText: "Gama" }).getByRole("button", { name: /^Editar: Valor/ });
  await cell.dblclick();
  const input = page.getByRole("spinbutton", { name: "Editar: Valor" });
  await input.fill("7");
  await input.press("Enter");
  await expect(page.getByRole("row").filter({ hasText: "Gama" })).toContainText("7");

  // The board: a lane per status.
  await page.getByRole("combobox", { name: "Visualização" }).click();
  await page.getByRole("option", { name: "Kanban" }).click();
  await expect(page.getByRole("listitem", { name: "novo (2)" })).toBeVisible();
  await expect(page.getByRole("listitem", { name: "feito (1)" })).toBeVisible();
});

// A public form adds records without a session; a read-only list shows them; a deleted link stops.
test("share links: a public form and a read-only list", async ({ page, browser }) => {
  await page.goto("/collections/new");
  await page.locator("#c-name").fill(`publico-${Date.now()}`);
  await page.getByRole("region", { name: "Nova coleção" }).getByRole("button", { name: "Criar" }).click();
  await expect(page).toHaveURL(/\/collections\/[0-9a-f-]{36}$/);
  const records = page.url();
  await page.goto(`${records}/integrations`);
  await page.getByRole("button", { name: "Criar formulário" }).click();
  await page.getByRole("button", { name: "Criar lista de leitura" }).click();
  const links = page.getByRole("list", { name: "Links públicos" }).getByRole("listitem");
  await expect(links).toHaveCount(2);
  const formUrl = await links.filter({ hasText: "Formulário público" }).locator("p.font-mono").innerText();
  const listUrl = await links.filter({ hasText: "Lista só de leitura" }).locator("p.font-mono").innerText();

  const guest = await guestPage(browser);
  await guest.goto(formUrl);
  await guest.getByRole("button", { name: "Enviar" }).click();
  await expect(guest.locator(formAlert)).toBeVisible(); // the title is required
  await guest.locator("#pub_title").fill("De fora");
  await guest.getByRole("button", { name: "Enviar" }).click();
  await expect(guest.getByText("Enviado. Obrigado!")).toBeVisible();

  await guest.goto(listUrl);
  await expect(guest.getByRole("cell", { name: "De fora" })).toBeVisible();
  await expect(guest.getByRole("searchbox")).toBeVisible();

  // A private field stays off both links, and the list stops searching.
  const colId = records.split("/").pop()!;
  const col = await (await page.request.get(`/api/proxy/v1/collections/${colId}`)).json();
  const schema = { ...col.schema, properties: { ...col.schema.properties, nota: { type: "string", title: "Nota interna", "x-private": true } } };
  expect((await page.request.patch(`/api/proxy/v1/collections/${colId}`, { data: { schema } })).ok()).toBeTruthy();
  await api(page, "/v1/records", { collection_id: colId, data: { title: "Com nota", nota: "segredo" } });
  await guest.reload();
  await expect(guest.getByRole("cell", { name: "Com nota" })).toBeVisible();
  await expect(guest.getByText("segredo")).toHaveCount(0);
  await expect(guest.getByRole("searchbox")).toHaveCount(0);
  await guest.goto(formUrl);
  await expect(guest.locator("#pub_title")).toBeVisible();
  await expect(guest.locator("#pub_nota")).toHaveCount(0);
  await guest.goto(listUrl);
  // A form link does not open as a list.
  await guest.goto(formUrl.replace("/f/", "/s/"));
  await expect(guest.getByText("Link não encontrado")).toBeVisible();

  await links.filter({ hasText: "Lista só de leitura" }).getByRole("button", { name: /^Revogar/ }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Revogar" }).click();
  await expect(links).toHaveCount(1);
  await guest.goto(listUrl);
  await expect(guest.getByText("Link não encontrado")).toBeVisible();
  await guest.close();
});

// A field holding several records of another collection; the target lists who points at it.
test("several references and the records linked to one", async ({ page }) => {
  await page.goto("/");
  const tags = await api(page, "/v1/collections", { name: `tags-${Date.now()}`, schema: { type: "object", properties: { nome: { type: "string", title: "Nome" } } } });
  const tagsId = (await tags.json()).id as string;
  const ids: string[] = [];
  for (const nome of ["Urgente", "Casa"]) ids.push((await (await api(page, "/v1/records", { collection_id: tagsId, data: { nome } })).json()).id);
  const posts = await api(page, "/v1/collections", {
    name: `posts-${Date.now()}`,
    schema: { type: "object", required: ["titulo"], properties: {
      titulo: { type: "string", title: "Título" },
      tags: { type: "array", title: "Tags", items: { type: "string", format: "uuid" }, uniqueItems: true, "x-ref": tagsId },
    } },
  });
  const postsId = (await posts.json()).id as string;

  await page.goto(`/records/new?collection=${postsId}`);
  await page.locator("#rec_titulo").fill("Consertar a pia");
  // The search stays open after a pick, for the next one.
  await page.getByRole("button", { name: /Adicionar registro/ }).click();
  for (const tag of ["Urgente", "Casa"]) {
    await page.getByPlaceholder("Buscar registro").fill(tag);
    await page.getByRole("button", { name: tag, exact: true }).click();
  }
  await expect(page.getByRole("list", { name: "Tags" }).getByRole("link")).toHaveText(["Urgente", "Casa"]);
  await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/collections/${postsId}$`));
  await expect(page.getByRole("row").filter({ hasText: "Consertar a pia" })).toContainText("Urgente, Casa");

  await page.goto(`/records/${ids[1]}`);
  const linked = page.getByRole("region", { name: /: Tags$/ });
  await expect(linked.getByRole("link", { name: "Consertar a pia" })).toBeVisible();
});

// An automation sets a field on new records that pass its condition; "run now" notifies.
test("automations act on matching records", async ({ page }) => {
  await page.goto("/");
  const made = await api(page, "/v1/collections", {
    name: `auto-${Date.now()}`,
    schema: { type: "object", properties: { titulo: { type: "string", title: "Título" }, prio: { type: "integer", title: "Prioridade" }, status: { title: "Status", enum: ["novo", "urgente"] } } },
  });
  const id = (await made.json()).id as string;
  await page.goto(`/collections/${id}/automations`);
  await page.getByRole("button", { name: "Nova automação" }).click();
  const form = page.getByRole("form", { name: "Nova automação" });
  await form.getByLabel("Nome").fill("Marcar urgentes");
  await form.getByRole("button", { name: "Condições" }).click();
  const rules = page.getByRole("dialog", { name: "Condições" });
  await rules.getByRole("combobox", { name: "Campo da condição 1" }).click();
  await page.getByRole("option", { name: "Prioridade" }).click();
  await rules.getByRole("combobox", { name: "Operador da condição 1" }).click();
  await page.getByRole("option", { name: "maior que", exact: true }).click();
  await rules.getByLabel("Valor 1").fill("5");
  await rules.getByRole("button", { name: "Aplicar" }).click();
  await form.getByLabel("Então").click();
  await page.getByRole("option", { name: "definir um campo" }).click();
  await form.getByLabel("Campo").click();
  await page.getByRole("option", { name: "Status" }).click();
  await form.getByLabel("Novo valor").click();
  await page.getByRole("option", { name: "urgente" }).click();
  await form.getByRole("button", { name: "Salvar" }).click();
  await expect(page.getByRole("list", { name: "Automações" })).toContainText("Marcar urgentes");

  const hot = (await (await api(page, "/v1/records", { collection_id: id, data: { titulo: "Quente", prio: 9 } })).json()).id as string;
  const cold = (await (await api(page, "/v1/records", { collection_id: id, data: { titulo: "Frio", prio: 1 } })).json()).id as string;
  // The job worker polls every 2 seconds.
  await expect.poll(async () => (await (await page.request.get(`/api/proxy/v1/records/${hot}`)).json()).data.status, { timeout: 15_000 }).toBe("urgente");
  expect((await (await page.request.get(`/api/proxy/v1/records/${cold}`)).json()).data.status).toBeUndefined();

  await page.reload();
  await page.getByRole("button", { name: "Executar agora" }).click();
  await expect(page.getByText("Automação executada.")).toBeVisible();
  await expect(page.getByRole("list", { name: "Automações" })).toContainText("2 no total");
});

// Fields the server fills, comments on a record, and an import that updates by a unique field.
test("auto fields, comments and an upsert import", async ({ page }) => {
  await page.goto("/");
  const made = await api(page, "/v1/collections", {
    name: `pedidos-${Date.now()}`,
    schema: { type: "object", properties: {
      codigo: { type: "string", title: "Código", "x-unique": true },
      preco: { type: "number", title: "Preço" },
      qtd: { type: "integer", title: "Qtd" },
      num: { type: "integer", title: "Nº", "x-auto": "number" },
      total: { type: "number", title: "Total", "x-formula": "preco * qtd" },
      autor: { type: "string", title: "Autor", "x-auto": "created_by" },
    } },
  });
  const id = (await made.json()).id as string;

  await page.goto(`/records/new?collection=${id}`);
  await expect(page.locator("#rec_total")).toHaveAttribute("readonly", "");
  await page.locator("#rec_codigo").fill("A1");
  await page.locator("#rec_preco").fill("2.5");
  await page.locator("#rec_qtd").fill("4");
  await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/collections/${id}$`));
  const list = await (await page.request.get(`/api/proxy/v1/records?collection_id=${id}`)).json();
  const rec = list.items[0];
  expect(rec.data).toMatchObject({ num: 1, total: 10 });
  expect(typeof rec.data.autor).toBe("string");

  await page.goto(`/records/${rec.id}`);
  const comments = page.locator("main");
  await comments.getByLabel("Escreva um comentário").fill("Cliente pediu urgência");
  await comments.getByRole("button", { name: "Comentar" }).click();
  await expect(comments.getByRole("list", { name: "Comentários" })).toContainText("Cliente pediu urgência");
  await comments.getByRole("button", { name: "Excluir comentário" }).click();
  await expect(comments.getByRole("list", { name: "Comentários" })).toHaveCount(0);

  // "Code" matches no field: it is mapped by hand. A1 exists, B2 is new.
  await page.goto(`/collections/${id}`);
  await page.getByRole("button", { name: "Importar" }).click();
  const dialog = page.getByRole("dialog", { name: "Importar" });
  await dialog.getByLabel("Registros que já existem").click();
  await page.getByRole("option", { name: "Código" }).click();
  await dialog.locator("#import-file").setInputFiles({ name: "pedidos.csv", mimeType: "text/csv", buffer: Buffer.from("Code,preco,qtd\nA1,3,3\nB2,1,2\n") });
  await dialog.getByRole("combobox", { name: "Campo da coluna Code" }).click();
  await page.getByRole("option", { name: "Código" }).click();
  await expect(dialog.getByRole("combobox", { name: "Campo da coluna Code" })).toContainText("Código");
  await dialog.getByRole("button", { name: "Verificar registros" }).click();
  await expect(dialog.getByRole("status")).toContainText("2 registro(s) válido(s), 0 com problema.");
  await dialog.getByRole("button", { name: "Importar 2" }).click();
  await expect(page.getByText("1 criados, 1 atualizados")).toBeVisible();
  const after = await (await page.request.get(`/api/proxy/v1/records?collection_id=${id}&sort=created_at&order=asc`)).json();
  expect(after.items.map((r: { data: Record<string, unknown> }) => [r.data.codigo, r.data.num, r.data.total])).toEqual([["A1", 1, 9], ["B2", 2, 2]]);
});

// A field holding several files: images show a thumbnail; the list names the first and counts the rest.
test("several files in one field", async ({ page }) => {
  await page.goto("/");
  const made = await api(page, "/v1/collections", {
    name: `fotos-${Date.now()}`,
    schema: { type: "object", properties: {
      titulo: { type: "string", title: "Título" },
      fotos: { type: "array", title: "Fotos", "x-file": true, maxItems: 20, items: { type: "object", required: ["id"], properties: { id: { type: "string" }, name: { type: "string" }, size: { type: "integer" }, type: { type: "string" } } } },
    } },
  });
  const id = (await made.json()).id as string;
  // A 1x1 PNG.
  const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=", "base64");

  await page.goto(`/records/new?collection=${id}`);
  await page.locator("#rec_titulo").fill("Viagem");
  const chooser = page.waitForEvent("filechooser");
  await page.getByRole("button", { name: "Adicionar arquivo" }).click();
  await (await chooser).setFiles([
    { name: "praia.png", mimeType: "image/png", buffer: png },
    { name: "roteiro.txt", mimeType: "text/plain", buffer: Buffer.from("dia 1") },
  ]);
  const list = page.getByRole("list", { name: "Fotos" });
  await expect(list.getByRole("listitem")).toHaveCount(2);
  await expect(list.locator("img")).toHaveCount(1);
  await list.getByRole("button", { name: "Remover arquivo: roteiro.txt" }).click();
  await expect(list.getByRole("listitem")).toHaveCount(1);
  const again = page.waitForEvent("filechooser");
  await page.getByRole("button", { name: "Adicionar arquivo" }).click();
  await (await again).setFiles([{ name: "mapa.png", mimeType: "image/png", buffer: png }]);
  await expect(list.getByRole("listitem")).toHaveCount(2);
  await page.getByRole("region", { name: "Novo registro" }).getByRole("button", { name: "Criar", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/collections/${id}$`));
  await expect(page.getByRole("row").filter({ hasText: "Viagem" })).toContainText("praia.png +1");
});
