# Wired Club

Turns the [Wired Club documentation](https://docs.wiredclub.com.br/) ([source](https://github.com/wiredclub/docs)) into records for two collections:

- **wireds.jsonl**, one record per wired, for a collection made from [wireds.schema.json](wireds.schema.json). Key: `classname`.
- **guias.jsonl**, one record per guide or reference article, for a collection made from [guias.schema.json](guias.schema.json). Key: `url`.

Each wired keeps the infobox fields: type, section, availability, prices, release, revision, requirements and negative version. Its page becomes Markdown in `conteudo`. Names come from the Habbo furnidata, the names the game shows ("EFEITO WIRED: Dar Pontos"); the docs shorten them. A wired whose furnidata name is only a placeholder ("wf_var_dailytask name") keeps the docs name.

`parametros` lists the settings of the wired's window, from the Habbo client texts (`wiredfurni.params.*`): for each one its key, label, options and, when the client has one, a usage note. For example `wf_act_move_rotate` gets the delay, "Mover Mobi:" (Nenhum movimento, Mover aleatoriamente) and "Girar Mobi:" with its four rotations. The client groups these texts by setting, not by wired, so [parametros.json](parametros.json) says which groups each wired shows; every Efeito also gets the delay and every Seletor the "filter existing selection" choice. 139 of the 221 wireds have settings. Wireds whose settings live in the shared variable and source groups, and the configurators, leaderboards and chests, have none yet.

`fontes` holds the advanced settings: where the wired takes its furni or users from (the triggering item, picked furni, the selector, the signal…). Each block has a kind (Mobi or Usuário), the client's title ("Selecione a fonte do Mobi:", "Selecione Bot fonte:") and the options the wired offers, in the client's words. The client does not say which options each wired offers; only the 17 docs pages with a "Configurações avançadas" list do, so only those wireds come filled in. The schema lists every option the client has, so the others can be completed on the record page.

The docs' lists of wireds (the "Lista Completa" of each overview page, the picks in "Tipos de Wireds") are drawn by a site component; they become Markdown lists of links to each wired's page, in the game's names.

The recent-changes pages are left out, since they repeat the others with older dates.

## Generate

Needs `git`, `curl`, Python 3 and PyYAML (`pip install pyyaml`). From this folder:

```bash
make wireds
```

This clones the docs into `.cache/` (or updates the clone), downloads the furnidata and the client texts, and writes `out/wireds.jsonl` and `out/guias.jsonl`.

habbo.com.br may refuse downloads from a script (HTTP 463). The target then stops and prints what to do: open each page in a browser, save it, and pass the files:

- https://www.habbo.com.br/gamedata/furnidata_json/1 → `FURNIDATA`
- https://www.habbo.com.br/gamedata/external_flash_texts/1 → `TEXTS`

```bash
make wireds FURNIDATA=~/Downloads/furnidata.json TEXTS=~/Downloads/external_flash_texts.txt
```

Both are required: an import replaces each record, so a run without them would drop the game names or the settings already in the collection.

`make clean` removes `.cache/` and `out/`; neither is tracked.

## Import

1. Create each collection once and paste its schema under **Fields → Advanced: edit the JSON Schema**.
2. On the collection's records, open **More → Import** and pick the JSONL file.
3. Under **Records that already exist**, pick `classname` (wireds) or `url` (guides).

When a run adds a field, such as `parametros`, paste the new schema into the collection first.

With that key, a later import updates the same records instead of adding copies, so running `make wireds` again and importing keeps the collections in step with the docs.

## Adding data the docs lack

To map a wired's settings, add its classname to [parametros.json](parametros.json) with the `wiredfurni.params` groups its window shows (the part after `wiredfurni.params.`, for example `show_message`). Search the saved client texts for `wiredfurni.params.` to see the groups.

`MISSING_NEGATIVES` in [convert.py](convert.py) adds a negative version the game has but the docs' infobox leaves out (today `wf_cnd_not_has_handitem`).
