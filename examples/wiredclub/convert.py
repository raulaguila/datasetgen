#!/usr/bin/env python3
"""
Usually run through `make wireds` (see README.md beside this file).

Turns the Wired Club docs (github.com/wiredclub/docs) into two JSONL files for
the record import: one record per wired (wireds.jsonl, keyed by classname) and
one per guide or reference article (guias.jsonl, keyed by url).

    git clone --depth 1 https://github.com/wiredclub/docs.git /tmp/wiredclub
    curl -o /tmp/furnidata.json https://www.habbo.com.br/gamedata/furnidata_json/1
    python3 examples/wiredclub/convert.py /tmp/wiredclub out/ --furnidata /tmp/furnidata.json --texts /tmp/texts.txt

The furnidata (optional) gives each wired, and its negative version, the name
the game shows ("EFEITO WIRED: Dar Pontos"); the docs shorten it. The client
texts (optional, habbo.com.br/gamedata/external_flash_texts/1) give each wired
listed in parametros.json the settings its window shows, with their options.
Import each file into the collection made from the schema beside it, with
"update by" set to classname (wireds) or url (guias): a later run updates the
same records. Needs PyYAML.
"""
import argparse
import json
import os
import re

import yaml

SITE = "https://docs.wiredclub.com.br"
DOCS = "src/content/docs"
ASSETS = "src"  # "@assets/x" resolves to src/assets/x
# Folders of the reference whose pages are one wired each; their "visao-geral" is an article.
SECTIONS = {
    "ativadores": "Ativadores", "condicoes": "Condições", "efeitos": "Efeitos", "seletores": "Seletores",
    "variaveis": "Variáveis", "extras": "Extras", "outros": "Outros", "contratos": "Contratos",
    "ferramentas-do-criador": "Ferramentas do criador",
}
GUIDE_SECTIONS = {"vamos-comecar": "Vamos começar", "guias-praticos": "Guias práticos", "referencia": "Referência", "sobre-nos": "Sobre nós"}
# Pages that are site plumbing, not knowledge.
SKIP = {"index.mdx", "sobre-nos/contribuidores.mdx"}
# Negative versions the game has but the docs' infobox leaves out (from the furnidata).
MISSING_NEGATIVES = {
    "wf_cnd_has_handitem": {"name": "Wired Condição Negativa: Habbo NÃO tem item de mão", "classname": "wf_cnd_not_has_handitem", "revision": 69021},
}


def front(text):
    m = re.match(r"---\n(.*?)\n---\n", text, re.S)
    return (yaml.safe_load(m.group(1)) or {}, text[m.end():]) if m else ({}, text)


def vtt_text(path):
    """The spoken descriptions of a video, cue text only."""
    try:
        lines = open(path, encoding="utf-8").read().splitlines()
    except OSError:
        return ""
    out = []
    for line in lines:
        line = line.strip()
        if not line or line == "WEBVTT" or "-->" in line or line.isdigit() or line.startswith(("NOTE", "STYLE")):
            continue
        out.append(re.sub(r"<[^>]+>", "", line))
    return " ".join(out)


def cell_text(html):
    return re.sub(r"\s+", " ", re.sub(r"<[^>]+>", "", html)).replace("|", "\\|").strip()


def table(m):
    rows = re.findall(r"<tr\b[^>]*>(.*?)</tr>", m.group(0), re.S)
    grid = [[cell_text(c) for c in re.findall(r"<t[hd]\b[^>]*>(.*?)</t[hd]>", r, re.S)] for r in rows]
    grid = [r for r in grid if r]
    if not grid:
        return ""
    width = max(len(r) for r in grid)
    grid = [r + [""] * (width - len(r)) for r in grid]
    lines = ["| " + " | ".join(grid[0]) + " |", "|" + " --- |" * width] + ["| " + " | ".join(r) + " |" for r in grid[1:]]
    return "\n" + "\n".join(lines) + "\n"


def html_to_markdown(body):
    """The HTML the pages write by hand: tables, images and inline tags."""
    body = re.sub(r"<table\b.*?</table>", table, body, flags=re.S)
    body = re.sub(r"<img\b[^>]*?/?>", lambda m: (lambda a: f"[Imagem: {a.group(1)}]" if a else "")(re.search(r'alt="([^"]*)"', m.group(0))), body, flags=re.S)
    body = re.sub(r"<figcaption\b[^>]*>(.*?)</figcaption>", r"\n\1", body, flags=re.S)
    body = re.sub(r"<code\b[^>]*>(.*?)</code>", r"`\1`", body, flags=re.S)
    body = re.sub(r"<(b|strong)\b[^>]*>(.*?)</\1>", r"**\2**", body, flags=re.S)
    body = re.sub(r"<(i|em)\b[^>]*>(.*?)</\1>", r"*\2*", body, flags=re.S)
    body = re.sub(r"<br\s*/?>", "\n", body)
    # Layout tags (div, p, span, figure, u…) go; their text stays. "<efeitos>" in prose is not a tag.
    return re.sub(r"</?(div|p|span|figure|u|section|small)\b[^>]*>", "", body)


def body_markdown(body, repo):
    """MDX to plain Markdown: components become text, site links absolute."""
    # import X from "@assets/....vtt?url" -> the file, for the videos' descriptions.
    assets = {}
    for name, src in re.findall(r'^import\s+(\w+)\s+from\s+["\']([^"\']+)["\'];?\s*$', body, re.M):
        if src.startswith("@assets/"):
            assets[name] = os.path.join(repo, ASSETS, "assets", src[len("@assets/"):].split("?")[0])
    body = re.sub(r"^(import|export)\s.*$", "", body, flags=re.M)
    body = re.sub(r"(?<!`)\{/\*.*?\*/\}(?!`)", "", body, flags=re.S)  # MDX comments; `{/* x */}` in code stays

    def video(m):
        tag = m.group(0)
        title = re.search(r'title="([^"]*)"', tag)
        desc = re.search(r"descriptionsSrc=\{(\w+)\}", tag)
        text = vtt_text(assets[desc.group(1)]) if desc and desc.group(1) in assets else ""
        head = f"[Vídeo: {title.group(1)}]" if title else "[Vídeo]"
        return f"{head}\n{text}".strip()

    body = re.sub(r"<AccessibleVideo\b.*?/>", video, body, flags=re.S)
    body = re.sub(r"<PseudoWiredCodeBlock\s+code=\{`(.*?)`\}\s*/>", lambda m: f"```pseudowired\n{m.group(1).strip()}\n```", body, flags=re.S)

    def aside(m):
        title = re.search(r'title="([^"]*)"', m.group(1))
        inner = m.group(2).strip()
        lead = f"**{title.group(1)}:** " if title else ""
        return "\n".join("> " + line if line else ">" for line in (lead + inner).splitlines())

    body = re.sub(r"<Aside\b([^>]*)>(.*?)</Aside>", aside, body, flags=re.S)
    # The site's lists of wireds, filled in by main() once every wired is read.
    body = re.sub(r'<WiredGrid\s+type="([^"]*)"\s*/>', lambda m: f"\x00grid:type:{m.group(1)}\x00", body)
    body = re.sub(r"<WiredGrid\s+classname_list=\{\[(.*?)\]\}\s*/>",
                  lambda m: "\x00grid:classes:" + ",".join(re.findall(r'"([^"]+)"', m.group(1))) + "\x00", body, flags=re.S)
    body = re.sub(r'<TabItem\s+label="([^"]*)"[^>]*>', r"**\1**", body)
    body = re.sub(r'<Card\s+title="([^"]*)"[^>]*>', r"**\1**", body)
    # Every other component tag goes; its children stay.
    body = re.sub(r"</?[A-Z][\w.]*\b[^>]*?/?>", "", body, flags=re.S)
    body = html_to_markdown(body)
    body = re.sub(r"\]\((/[^)\s]*)\)", lambda m: f"]({SITE}{m.group(1)})", body)
    body = re.sub(r"!\[([^\]]*)\]\([^)]*\)", lambda m: f"[Imagem: {m.group(1)}]" if m.group(1) else "", body)
    return re.sub(r"\n{3,}", "\n\n", body).strip()


def fill_grids(rows, wireds):
    """Each placeholder body_markdown left for a <WiredGrid> becomes a list of links to those wireds,
    by type or by classname (a negative version links to its positive's page)."""
    by_class = {}
    for w in wireds:
        by_class[w["classname"]] = (w["nome"], w["url"])
        neg = w.get("versao_negativa") or {}
        if neg.get("classname"):
            by_class.setdefault(neg["classname"], (neg.get("nome") or neg["classname"], w["url"]))

    def grid(m):
        kind, arg = m.group(1), m.group(2)
        items = ([(w["nome"], w["url"]) for w in sorted(wireds, key=lambda w: w["nome"]) if w.get("tipo") == arg] if kind == "type"
                 else [by_class[c] for c in arg.split(",") if c in by_class])
        return "\n".join(f"- [{n}]({u})" for n, u in items)

    for r in rows:
        if "conteudo" in r:
            r["conteudo"] = re.sub(r"\n{3,}", "\n\n", re.sub(r"\x00grid:(type|classes):([^\x00]*)\x00", grid, r["conteudo"])).strip()


def page_url(rel):
    slug = re.sub(r"\.mdx?$", "", rel)
    slug = re.sub(r"(^|/)index$", "", slug)
    return f"{SITE}/{slug}/".replace("//", "/").replace("https:/", "https://")


def furni_names(path):
    """classname -> name from a furnidata JSON; its "<classname> name" placeholders are skipped."""
    data = json.load(open(path, encoding="utf-8"))
    names = {}
    for kind in ("roomitemtypes", "wallitemtypes"):
        for f in (data.get(kind) or {}).get("furnitype", []):
            c, n = str(f.get("classname", "")).lower(), str(f.get("name") or "").strip()
            if c and n and n.lower() != f"{c} name":
                names[c] = n
    return names


def client_texts(path):
    """key -> text from the client's external texts, "${other.key}" references filled in."""
    texts = {}
    for line in open(path, encoding="utf-8"):
        key, sep, value = line.rstrip("\n").partition("=")
        if sep:
            texts[key] = value
    ref = re.compile(r"\$\{([^}]+)\}")
    for _ in range(3):  # references nest a level or two
        texts = {k: ref.sub(lambda m: texts.get(m.group(1), m.group(0)), v) for k, v in texts.items()}
    return texts


PARAMS = "wiredfurni.params."


def setting_groups(texts):
    """Each wiredfurni.params group as settings: a key with numbered children is a choice
    (its text the label, the children the options); a group's own key without them is a field."""
    by_group = {}
    for k in texts:
        if k.startswith(PARAMS):
            by_group.setdefault(k[len(PARAMS):].split(".")[0], []).append(k[len(PARAMS):])
    groups = {}
    for g, keys in by_group.items():
        keys = set(keys)
        num = lambda k: k.rsplit(".", 1)[-1].isdigit()
        parents = sorted({k.rsplit(".", 1)[0] for k in keys if "." in k and num(k)})
        settings = []
        if g in keys and g not in parents:
            settings.append({"chave": g, "rotulo": texts[PARAMS + g]})
        for p in parents:
            kids = sorted((k for k in keys if k.rsplit(".", 1)[0] == p and num(k)), key=lambda k: int(k.rsplit(".", 1)[1]))
            label = texts.get(PARAMS + p) or texts.get(PARAMS + p + "_selection") or texts.get(PARAMS + p.rsplit(".", 1)[0]) or ""
            settings.append(drop_empty({"chave": p, "rotulo": label, "opcoes": [texts[PARAMS + k] for k in kids]}))
        if not settings:
            # A group of named fields only ("bot.name", "link.key"): each field, notes aside.
            notes = ("info", "usage_info", "tooltip", "placeholder", "warning", "invalid_number")
            leaves = [k for k in keys if "." in k and not any(o.startswith(k + ".") for o in keys) and not k.rsplit(".", 1)[1].endswith(notes)]
            settings = [{"chave": k, "rotulo": texts[PARAMS + k]} for k in sorted(leaves)]
        info = texts.get(PARAMS + g + ".usage_info") or texts.get(PARAMS + g + ".info")
        if info:
            for s in settings:
                s.setdefault("descricao", info.replace("\\n", "\n"))
        groups[g] = settings
    return groups


def wired_settings(classname, kind, groups, mapping, texts):
    names = list(mapping.get(classname, []))
    # Every effect has a delay and every selector can filter the current selection.
    names = (["delay"] if kind == "Efeito" else ["selector_option"] if kind == "Seletor" else []) + names

    def pick(name):
        # "time" takes the whole group; "time.hour_selection" only that setting or field.
        if "." not in name:
            return groups.get(name, [])
        hit = [s for s in groups.get(name.split(".")[0], []) if s["chave"] == name]
        return hit or ([{"chave": name, "rotulo": texts[PARAMS + name]}] if PARAMS + name in texts else [])

    return [s for n in names for s in pick(n)]


# The docs' wording of each source option -> its wiredfurni.params.sources key.
SOURCE_OPTIONS = [
    ("item de ativação", "furni.0"), ("mobis escolhidos", "furni.100"), ("mobis do seletor", "furni.200"),
    ("mobis do sinal", "furni.201"), ("bot especificado", "users.100"), ("habbo especificado", "users.101"),
    ("do seletor", "users.200"), ("do sinal", "users.201"), ("acionador", "users.0"),
]


def wired_sources(body, texts):
    """The "Configurações avançadas" lists of a wired's page as source blocks, in the client's words:
    each bold line opens a block, each bullet is an option."""
    m = re.search(r"^## Configurações avançadas:?\s*\n(.*?)(?=^## |\Z)", body, re.S | re.M)
    if not m:
        return []
    src = "wiredfurni.params.sources."
    blocks, current = [], None
    for line in m.group(1).splitlines():
        line = line.strip()
        head = re.fullmatch(r"\*\*(.+?)\*\*", line)
        if head or (line.startswith(("-", "*")) and current is None):
            current = {"titulo_docs": head.group(1) if head else "", "keys": []}
            blocks.append(current)
        if line.startswith(("-", "*")) and not head:
            text = line.lstrip("-* ").lower()
            key = next((k for w, k in SOURCE_OPTIONS if w in text), None)
            if key and key not in current["keys"]:
                current["keys"].append(key)
    out = []
    for b in blocks:
        if not b["keys"]:
            continue
        users = all(k.startswith("users.") for k in b["keys"])
        bots = "users.100" in b["keys"] or "bot" in b["titulo_docs"].lower()
        title = src + ("users.title.bots" if bots else "users.title" if users else "furni.title")
        out.append({
            "bloco": "Usuário" if users else "Mobi",
            "titulo": texts.get(title, b["titulo_docs"]),
            "opcoes": [texts.get(src + k, k) for k in sorted(b["keys"], key=lambda k: int(k.split(".")[1]))],
        })
    return out


def drop_empty(d):
    return {k: v for k, v in d.items() if v not in (None, "", [], {})}


def wired(fm, body, rel, repo, names):
    ib = fm.get("infobox") or {}
    real = lambda classname, fallback: names.get(str(classname or "").lower()) or fallback
    price = ib.get("price")
    neg = ib.get("negative_version") or MISSING_NEGATIVES.get(ib.get("classname"), {})
    badge = ((fm.get("sidebar") or {}).get("badge") or {})
    return drop_empty({
        "nome": real(ib.get("classname"), str(ib.get("name") or fm.get("title") or "").strip()),
        "classname": ib.get("classname"),
        "tipo": ib.get("type"),
        "secao": SECTIONS[rel.split("/")[1]],
        "descricao": (ib.get("description") or fm.get("description") or "").strip(),
        "disponibilidade": ib.get("availability"),
        "preco": price.get("credits") if isinstance(price, dict) else price,
        "preco_diamantes": price.get("diamonds") if isinstance(price, dict) else None,
        "lancamento": str(ib["release_date"]) if ib.get("release_date") else None,
        "revisao": ib.get("revision"),
        "nome_na_loja": ib.get("product_name"),
        "requer_mobi": bool(ib.get("requires_furni")),
        "requer_bot": bool(ib.get("requires_bot")),
        "requer_bau": bool(ib.get("requires_chest")),
        "requer_contrato": bool(ib.get("requires_contract")),
        "requer_antena": bool(ib.get("requires_antena")),
        "versao_negativa": drop_empty({
            "nome": real(neg.get("classname"), neg.get("name")), "classname": neg.get("classname"),
            "descricao": neg.get("description"), "revisao": neg.get("revision"),
        }),
        "rascunho": str(badge.get("text", "")).upper() == "RASCUNHO",
        "conteudo": body_markdown(body, repo),
        "url": page_url(rel),
    })


def guide(fm, body, rel, repo):
    return drop_empty({
        "titulo": str(fm.get("title") or "").strip().strip("-").strip(),  # some titles wear "-…-" as decoration
        "secao": GUIDE_SECTIONS.get(rel.split("/")[0], "Outros"),
        "descricao": (fm.get("description") or "").strip(),
        "conteudo": body_markdown(body, repo),
        "url": page_url(rel),
    })


# The client's text placeholders, shown as a reader would say them.
PLACEHOLDERS = {"tileX": "X", "tileY": "Y", "error": "[erro]"}
GENERATED = "Gerado (revisar)"


def short_name(nome):
    """'EFEITO WIRED: Dar Pontos' -> 'dar pontos', the name pages write in bold."""
    return re.sub(r"^[^:]*:\s*", "", nome).strip().lower()


def enrich(wireds, guides, here):
    """What the docs leave out: texts for draft pages (textos/*.yaml, generated and marked for
    review), negative versions as records of their own, color/model variants, readable
    placeholders, absolute anchors and the links between wireds and guides."""
    texts = {}
    for f in sorted(os.listdir(os.path.join(here, "textos"))):
        if f.endswith(".yaml"):
            texts.update(yaml.safe_load(open(os.path.join(here, "textos", f), encoding="utf-8")) or {})
    have = {w["classname"] for w in wireds}
    for w in list(wireds):
        neg = w.get("versao_negativa") or {}
        if neg.get("classname") and neg["classname"] not in have:
            have.add(neg["classname"])
            wireds.append(drop_empty({
                "nome": neg.get("nome") or neg["classname"], "classname": neg["classname"], "versao_positiva": w["classname"],
                "tipo": w["tipo"], "secao": w["secao"], "descricao": neg.get("descricao"), "revisao": neg.get("revisao"),
                "disponibilidade": w.get("disponibilidade"), "rascunho": w.get("rascunho"), "url": w["url"],
            }))
    for w in wireds:
        c = w["classname"]
        m = re.search(r"\*(\d+)$", c) or re.fullmatch(r"wf_storage_(?:coins|furni)(\d)", c)
        if m:
            w["variante"] = int(m.group(1))
        t = texts.get(c) or texts.get(re.sub(r"\*\d+$", "", c)) or {}
        generated = False
        if t.get("descricao") and w.get("descricao", c) in ("", c):
            w["descricao"], generated = t["descricao"].strip(), True
        if t.get("conteudo") and not w.get("conteudo"):
            w["conteudo"], generated = t["conteudo"].strip(), True
        w["fonte_texto"] = GENERATED if generated else "Documentação"
        for p in w.get("parametros", []):
            fix = lambda s: re.sub(r"%(\w+)%", lambda m: PLACEHOLDERS.get(m.group(1), "N"), s)
            for k in ("rotulo", "descricao"):
                if k in p:
                    p[k] = fix(p[k])
            if "opcoes" in p:
                p["opcoes"] = [fix(o) for o in p["opcoes"]]
    for r in wireds + guides:
        if r.get("conteudo"):
            r["conteudo"] = re.sub(r"\]\((#[^)\s]+)\)", lambda m: f"]({r['url']}{m.group(1)})", r["conteudo"])
    by_url = {w["url"]: w["classname"] for w in wireds if not w.get("versao_positiva")}
    by_name = {}
    for w in sorted(wireds, key=lambda w: w["classname"]):  # a shared name goes to its first model or color
        by_name.setdefault(short_name(w["nome"]), w["classname"])
    linked = lambda text: re.findall(r"\]\((" + re.escape(SITE) + r"/[^)#\s]*)", text or "")
    for w in wireds:
        text = w.get("conteudo", "")
        refs = [by_url[u] for u in linked(text) if u in by_url] + [by_name[n.lower()] for n in re.findall(r"\*\*([^*]+)\*\*", text) if n.lower() in by_name]
        w["relacionados"] = sorted(set(refs) - {w["classname"]}) or None
    cites = {}
    for g in guides:
        for u in linked(g.get("conteudo")):
            if u in by_url:
                cites.setdefault(by_url[u], set()).add(g["url"])
    for w in wireds:
        w["guias"] = sorted(cites.get(w["classname"], ())) or None
    wireds[:] = [drop_empty(w) for w in wireds]


def main(repo, out, furnidata=None, texts=None):
    names = furni_names(furnidata) if furnidata else {}
    texts = client_texts(texts) if texts else {}
    groups = setting_groups(texts)
    mapping = json.load(open(os.path.join(os.path.dirname(os.path.abspath(__file__)), "parametros.json"), encoding="utf-8"))
    docs = os.path.join(repo, DOCS)
    wireds, guides = [], []
    for d, _, files in os.walk(docs):
        for f in sorted(files):
            if not f.endswith((".md", ".mdx")):
                continue
            path = os.path.join(d, f)
            rel = os.path.relpath(path, docs).replace(os.sep, "/")
            # The changelog mirrors every page with older dates: history, not reference.
            if rel in SKIP or rel.startswith("mudancas-recentes/"):
                continue
            fm, body = front(open(path, encoding="utf-8").read())
            parts = rel.split("/")
            if fm.get("infobox") and parts[0] == "referencia" and len(parts) >= 3 and parts[1] in SECTIONS:
                w = wired(fm, body, rel, repo, names)
                if groups:
                    w["parametros"] = wired_settings(w["classname"], w["tipo"], groups, mapping, texts) or None
                    w["fontes"] = wired_sources(body, texts) or None
                wireds.append(drop_empty(w))
            else:
                guides.append(guide(fm, body, rel, repo))
    fill_grids(wireds + guides, wireds)
    enrich(wireds, guides, os.path.dirname(os.path.abspath(__file__)))
    os.makedirs(out, exist_ok=True)
    for name, rows, key in (("wireds", wireds, "classname"), ("guias", guides, "url")):
        rows.sort(key=lambda r: r.get(key) or "")
        with open(os.path.join(out, f"{name}.jsonl"), "w", encoding="utf-8") as fh:
            for r in rows:
                fh.write(json.dumps(r, ensure_ascii=False) + "\n")
        print(f"{name}: {len(rows)} records")


if __name__ == "__main__":
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("repo")
    ap.add_argument("out")
    ap.add_argument("--furnidata")
    ap.add_argument("--texts")
    a = ap.parse_args()
    main(a.repo, a.out, a.furnidata, a.texts)
