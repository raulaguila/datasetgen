#!/usr/bin/env python3
"""
Turns the Wired Club docs (github.com/wiredclub/docs) into two JSONL files for
the record import: one record per wired (wireds.jsonl, keyed by classname) and
one per guide or reference article (guias.jsonl, keyed by url).

    git clone --depth 1 https://github.com/wiredclub/docs.git /tmp/wiredclub
    curl -o /tmp/furnidata.json https://www.habbo.com.br/gamedata/furnidata_json/1
    python3 examples/wiredclub/convert.py /tmp/wiredclub out/ /tmp/furnidata.json

The furnidata (optional) gives each wired, and its negative version, the name
the game shows ("EFEITO WIRED: Dar Pontos"); the docs shorten it.
Import each file into the collection made from the schema beside it, with
"update by" set to classname (wireds) or url (guias): a later run updates the
same records. Needs PyYAML.
"""
import json
import os
import re
import sys

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
    body = re.sub(r'<TabItem\s+label="([^"]*)"[^>]*>', r"**\1**", body)
    body = re.sub(r'<Card\s+title="([^"]*)"[^>]*>', r"**\1**", body)
    # Every other component tag goes; its children stay.
    body = re.sub(r"</?[A-Z][\w.]*\b[^>]*?/?>", "", body, flags=re.S)
    body = html_to_markdown(body)
    body = re.sub(r"\]\((/[^)\s]*)\)", lambda m: f"]({SITE}{m.group(1)})", body)
    body = re.sub(r"!\[([^\]]*)\]\([^)]*\)", lambda m: f"[Imagem: {m.group(1)}]" if m.group(1) else "", body)
    return re.sub(r"\n{3,}", "\n\n", body).strip()


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
        "titulo": str(fm.get("title") or "").strip(),
        "secao": GUIDE_SECTIONS.get(rel.split("/")[0], "Outros"),
        "descricao": (fm.get("description") or "").strip(),
        "conteudo": body_markdown(body, repo),
        "url": page_url(rel),
    })


def main(repo, out, furnidata=None):
    names = furni_names(furnidata) if furnidata else {}
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
                wireds.append(wired(fm, body, rel, repo, names))
            else:
                guides.append(guide(fm, body, rel, repo))
    os.makedirs(out, exist_ok=True)
    for name, rows, key in (("wireds", wireds, "classname"), ("guias", guides, "url")):
        rows.sort(key=lambda r: r.get(key) or "")
        with open(os.path.join(out, f"{name}.jsonl"), "w", encoding="utf-8") as fh:
            for r in rows:
                fh.write(json.dumps(r, ensure_ascii=False) + "\n")
        print(f"{name}: {len(rows)} records")


if __name__ == "__main__":
    if len(sys.argv) not in (3, 4):
        sys.exit(__doc__)
    main(*sys.argv[1:])
