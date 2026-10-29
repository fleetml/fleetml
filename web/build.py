#!/usr/bin/env python3
"""Builds the landing page of fleetml.dev into <site>/ (the docs go to <site>/docs/, built by MkDocs first).

Every command on the page is taken from the file it documents at build time, so the
page cannot drift from the docs: `{{snippet:<name>}}` in web/index.html becomes the
block named in SNIPPETS, HTML-escaped. `{{version}}` becomes the top version of
CHANGELOG.md. `--check` reads the built page back and compares each snippet with its
source, byte for byte.

Usage (from the repository root; `make website` runs both):
    python3 web/build.py site            # write site/index.html, 404.html and the assets
    python3 web/build.py --check site    # every snippet on the built page equals its source
"""

import html
import os
import re
import shutil
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WEB = os.path.join(ROOT, "web")
COPY = ["404.html", "robots.txt", "sitemap.xml", "logo.svg", "assets"]

# name -> (source file, how to find the block, lines to keep (1-based, inclusive) or None)
#   ("golden", id): the fenced block after `<!-- golden: id -->`
#   ("after", heading, lang): the first ```lang block after the line `heading`
SNIPPETS = {
    "cli": ("README.md", ("golden", "cli"), None),
    "android": ("README.md", ("after", "## Install the SDKs", "kotlin"), None),
    "ios": ("README.md", ("after", "## Install the SDKs", "text"), None),
    "python": ("README.md", ("after", "## Install the SDKs", "python"), None),
    "story": ("README.md", ("after", "## The 60-second story", "text"), None),
    "linux": ("docs/LINUX_AGENT.md", ("after", "## Install", "bash"), None),
    "helm": ("deploy/helm/fleetml/README.md", ("golden", "helm-install"), (1, 3)),
    "checksum": ("README.md", ("golden", "cli"), (4, 4)),
}


def fenced(lines, start):
    """The lines of the fenced block opening at or after index start, indentation removed."""
    i = start
    while not lines[i].lstrip().startswith("```"):
        i += 1
    indent = len(lines[i]) - len(lines[i].lstrip())
    out = []
    for line in lines[i + 1:]:
        if line.strip() == "```":
            return out
        out.append(line[indent:] if line[:indent].strip() == "" else line)
    raise ValueError("unterminated block")


def snippet(name):
    path, how, keep = SNIPPETS[name]
    with open(os.path.join(ROOT, path), encoding="utf-8") as f:
        lines = f.read().split("\n")
    if how[0] == "golden":
        start = next(i for i, l in enumerate(lines) if l.strip().startswith(f"<!-- golden: {how[1]} "))
        block = fenced(lines, start + 1)
    else:
        start = next(i for i, l in enumerate(lines) if l == how[1])
        start = next(i for i in range(start, len(lines)) if lines[i].strip() == "```" + how[2])
        block = fenced(lines, start)
    if keep:
        block = block[keep[0] - 1:keep[1]]
    return "\n".join(block)


def version():
    with open(os.path.join(ROOT, "CHANGELOG.md"), encoding="utf-8") as f:
        m = re.search(r"^## \[(\d+\.\d+\.\d+)\]", f.read(), re.M)
    return m.group(1)


def build(site):
    os.makedirs(site, exist_ok=True)
    with open(os.path.join(WEB, "index.html"), encoding="utf-8") as f:
        page = f.read()
    page = page.replace("{{version}}", version())
    page = re.sub(r"\{\{snippet:([a-z]+)\}\}", lambda m: html.escape(snippet(m.group(1)), quote=False), page)
    left = re.findall(r"\{\{[^}]*\}\}", page)
    if left:
        sys.exit(f"unfilled placeholders: {left}")
    with open(os.path.join(site, "index.html"), "w", encoding="utf-8") as f:
        f.write(page)
    for name in COPY:
        src, dst = os.path.join(WEB, name), os.path.join(site, name)
        if os.path.isdir(src):
            shutil.copytree(src, dst, dirs_exist_ok=True)
        else:
            shutil.copy2(src, dst)
    print(f"landing page v{version()} written to {site}/index.html")


def check(site):
    with open(os.path.join(site, "index.html"), encoding="utf-8") as f:
        page = f.read()
    found = re.findall(r'<code[^>]*\sdata-snippet="([a-z]+)"[^>]*>(.*?)</code>', page, re.S)
    bad = 0
    for name, body in found:
        same = html.unescape(body) == snippet(name)
        bad += not same
        path, how, keep = SNIPPETS[name]
        where = f"{path} ({' '.join(how)}{', lines %d-%d' % keep if keep else ''})"
        print(f"{'identical' if same else 'DIFFERS  '} {name:9} {where}")
    unused = set(SNIPPETS) - {n for n, _ in found}
    if unused:
        print(f"not on the page: {sorted(unused)}")
    print(f"{len(found)} snippets, {bad} different; version on the page: v{version()}"
          f" ({'present' if 'v' + version() in page else 'MISSING'})")
    return 1 if bad or not found or "v" + version() not in page else 0


if __name__ == "__main__":
    args = sys.argv[1:]
    if args and args[0] == "--check":
        sys.exit(check(args[1] if len(args) > 1 else "site"))
    build(args[0] if args else "site")
