#!/usr/bin/env python3
"""Builds the landing page of fleetml.dev into <site>/ (the docs go to <site>/docs/, built by MkDocs first).

Every command on the page is taken from the file it documents at build time, so the
page cannot drift from the docs: `{{snippet:<name>}}` in web/index.html becomes the
block named in SNIPPETS, HTML-escaped. `{{version}}` becomes the top version of
CHANGELOG.md and `{{release-date}}` its date. `--check` reads the built page back
and compares each snippet with its source, byte for byte.

The rollout replay is a recorded run of scripts/demo.sh (web/demo-transcript.txt):
REPLAY picks its lines, and the build writes them to assets/replay.json for
web/replay.js and, as the static version, into the page (`{{replay}}`, with
the status strip's last state in `{{replay-strip}}`); the whole run is
`{{transcript}}`. The strip's numbers are read from the lines; `--check`
compares the JSON and the page with the transcript, line by line.

Usage (from the repository root; `make website` runs both):
    python3 web/build.py site            # write site/index.html, 404.html and the assets
    python3 web/build.py --check site    # every snippet on the built page equals its source
"""

import html
import json
import os
import re
import shutil
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
WEB = os.path.join(ROOT, "web")
COPY = ["404.html", "robots.txt", "sitemap.xml", "logo.svg", "replay.js", "assets"]
TRANSCRIPT = os.path.join(WEB, "demo-transcript.txt")

# name -> (source file, how to find the block, lines to keep (1-based, inclusive) or None)
#   ("golden", id): the fenced block after `<!-- golden: id -->`
#   ("after", heading, lang): the first ```lang block after the line `heading`
SNIPPETS = {
    "cli": ("README.md", ("golden", "cli"), None),
    "android": ("README.md", ("after", "## Install the SDKs", "kotlin"), None),
    "ios": ("README.md", ("after", "## Install the SDKs", "text"), None),
    "python": ("README.md", ("after", "## Install the SDKs", "python"), None),
    "linux": ("docs/LINUX_AGENT.md", ("after", "## Install", "bash"), None),
    "helm": ("deploy/helm/fleetml/README.md", ("golden", "helm-install"), (1, 3)),
}


# The replay: (pattern for the next transcript line to show, kind). Kinds: step
# (a ▶ heading), cmd (typed), out, and the three moments: canary, gate, back.
REPLAY = [
    (r"▶ 1\. ", "step"),
    (r"\$ fleetml push .* --version 1\.0\.0$", "cmd"),
    (r"✓ Pushed .* v1\.0\.0 ", "out"),
    (r"\$ fleetml push .* --version 2\.0\.0$", "cmd"),
    (r"✓ Pushed .* v2\.0\.0 ", "out"),
    (r"each file was sha256-verified", "out"),
    (r"▶ 2\. ", "step"),
    (r"\$ fleetml simulate ", "cmd"),
    (r"\$ POST /api/v1/deployments .*version: 1\.0\.0", "cmd"),
    (r"fleet: iris 1\.0\.0 on 20$", "out"),
    (r"▶ 4\. ", "step"),
    (r"\$ POST /api/v1/deployments .*CANARY", "cmd"),
    (r"fleet: .*\(the canary cohort only\)$", "canary"),
    (r"deployment ROLLED_BACK: ", "gate"),
    (r"fleet: iris 1\.0\.0 on 20, ", "back"),
]
STRIP = ["Deployment", "Status", "Stage", "Health gate", "Fleet"]


def transcript():
    with open(TRANSCRIPT, encoding="utf-8") as f:
        return f.read().rstrip("\n").split("\n")


def strip_update(kind, text):
    """What a line changes in the status strip, read from the line itself."""
    t = text.strip()
    if kind == "cmd" and "CANARY" in t:
        version = re.search(r"version: ([\d.]+)", t).group(1)
        stages = re.search(r"CANARY ([\d% >-]+?),", t).group(1).split(" -> ")
        gate = re.search(r"gate: (.*?), else", t).group(1)
        return {"Deployment": f"iris {version} canary", "Status": "ROLLING_OUT",
                "Stage": f"1 of {len(stages)}, {stages[0]}", "Health gate": gate.replace("<=", "≤")}
    if kind == "out" and t.startswith("fleet: "):
        return {"Fleet": t[len("fleet: "):]}
    if kind == "canary":
        return {"Fleet": re.sub(r"\s+\(.*\)$", "", t[len("fleet: "):])}
    if kind == "gate":
        m = re.search(r"error rate ([\d.]+%) exceeds threshold ([\d.]+%) \((\d+/\d+) events", t)
        return {"Status": "ROLLED_BACK", "Health gate": f"tripped: errors {m.group(1)} > {m.group(2)} ({m.group(3)} events)"}
    if kind == "back":
        return {"Fleet": t[len("fleet: "):].split(",")[0]}
    return {}


def replay():
    """The REPLAY lines of the transcript, in order, with their strip updates."""
    lines, i, out = transcript(), 0, []
    for pattern, kind in REPLAY:
        while not re.match(pattern, lines[i].strip()):
            i += 1
        out.append({"text": lines[i], "kind": kind, "strip": strip_update(kind, lines[i])})
        i += 1
    return out


def replay_html(lines):
    return "\n".join(f'<span class="l {l["kind"]}">{html.escape(l["text"], quote=False)}</span>' for l in lines)


def strip_html(lines):
    state = {}
    for l in lines:
        state.update(l["strip"])
    return "".join(f'<div><dt>{k}</dt><dd data-strip="{k}">{html.escape(state.get(k, "-"))}</dd></div>' for k in STRIP)


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


def release():
    """The top version of CHANGELOG.md and its date, as written there."""
    with open(os.path.join(ROOT, "CHANGELOG.md"), encoding="utf-8") as f:
        m = re.search(r"^## \[(\d+\.\d+\.\d+)\] - (\d{4}-\d{2}-\d{2})$", f.read(), re.M)
    return m.group(1), m.group(2)


def version():
    return release()[0]


def build(site):
    os.makedirs(site, exist_ok=True)
    with open(os.path.join(WEB, "index.html"), encoding="utf-8") as f:
        page = f.read()
    page = page.replace("{{version}}", version()).replace("{{release-date}}", release()[1])
    lines = replay()
    page = page.replace("{{replay}}", replay_html(lines)).replace("{{replay-strip}}", strip_html(lines))
    page = page.replace("{{transcript}}", html.escape("\n".join(transcript()), quote=False))
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
    with open(os.path.join(site, "assets", "replay.json"), "w", encoding="utf-8") as f:
        json.dump({"source": "scripts/demo.sh, recorded in web/demo-transcript.txt", "strip": STRIP, "lines": lines},
                  f, ensure_ascii=False, indent=1)
        f.write("\n")
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
    # The replay: every JSON line and every line of the page's static replay is a
    # line of the transcript, in the transcript's order.
    with open(os.path.join(site, "assets", "replay.json"), encoding="utf-8") as f:
        data = [l["text"] for l in json.load(f)["lines"]]
    static = [html.unescape(t) for t in re.findall(r'<span class="l [a-z]+">(.*?)</span>', page)]
    rec, missing, pos = transcript(), 0, -1
    for t in data:
        nxt = next((i for i in range(pos + 1, len(rec)) if rec[i] == t), None)
        missing += nxt is None
        pos = nxt if nxt is not None else pos
    same = static == data
    print(f"replay: {len(data)} lines, {missing} not in the transcript in order; static copy on the page"
          f" {'identical' if same else 'DIFFERENT'}")
    return 1 if bad or not found or "v" + version() not in page or missing or not same else 0


if __name__ == "__main__":
    args = sys.argv[1:]
    if args and args[0] == "--check":
        sys.exit(check(args[1] if len(args) > 1 else "site"))
    build(args[0] if args else "site")
