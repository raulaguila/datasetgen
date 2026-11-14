# Wired Club

Turns the [Wired Club documentation](https://docs.wiredclub.com.br/) ([source](https://github.com/wiredclub/docs)) into records for two collections:

- **wireds.jsonl**, one record per wired, for a collection made from [wireds.schema.json](wireds.schema.json). Key: `classname`.
- **guias.jsonl**, one record per guide or reference article, for a collection made from [guias.schema.json](guias.schema.json). Key: `url`.

Each wired keeps the infobox fields: type, section, availability, prices, release, revision, requirements and negative version. Its page becomes Markdown in `conteudo`. Names come from the Habbo furnidata, the names the game shows ("EFEITO WIRED: Dar Pontos"); the docs shorten them. A wired whose furnidata name is only a placeholder ("wf_var_dailytask name") keeps the docs name. The recent-changes pages are left out, since they repeat the others with older dates.

## Generate

Needs `git`, `curl`, Python 3 and PyYAML (`pip install pyyaml`). From this folder:

```bash
make wireds
```

This clones the docs into `.cache/` (or updates the clone), downloads the furnidata and writes `out/wireds.jsonl` and `out/guias.jsonl`.

habbo.com.br may refuse the download from a script (HTTP 463). The target then stops and prints what to do: open https://www.habbo.com.br/gamedata/furnidata_json/1 in a browser, save the page, and pass the file:

```bash
make wireds FURNIDATA=~/Downloads/furnidata.json
```

`make clean` removes `.cache/` and `out/`; neither is tracked.

## Import

1. Create each collection once and paste its schema under **Fields → Advanced: edit the JSON Schema**.
2. On the collection's records, open **More → Import** and pick the JSONL file.
3. Under **Records that already exist**, pick `classname` (wireds) or `url` (guides).

With that key, a later import updates the same records instead of adding copies, so running `make wireds` again and importing keeps the collections in step with the docs.

## Adding data the docs lack

`MISSING_NEGATIVES` in [convert.py](convert.py) adds a negative version the game has but the docs' infobox leaves out (today `wf_cnd_not_has_handitem`).
