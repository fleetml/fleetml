# The fleetml.dev website

`make website` builds the whole site into `site/` (not committed):

- `site/index.html`, `site/404.html` and `site/assets/`: the landing page, from
  this folder, by `web/build.py`;
- `site/docs/`: the documentation, the same MkDocs build as `make docs-site`
  (MkDocs Material, pinned by `MKDOCS_MATERIAL_VERSION` in the Makefile,
  installed into `.website-venv/` with pip), with `site_url` `https://fleetml.dev/docs/`.

It then checks every internal link and anchor of `site/` (`scripts/check-site-links.py`)
and that every command on the landing page equals the file it comes from
(`web/build.py --check`). Running it again gives the same `site/`.

## Preview

```bash
make website
cd site && python3 -m http.server 8000
```

Then open `http://localhost:8000/` (the docs are at `/docs/`).

## How the page stays true

- Commands are not copied into `index.html`: each `{{snippet:<name>}}` is
  filled at build time from the README or the guide it names (`SNIPPETS` in
  `build.py`), HTML-escaped. `python3 web/build.py --check site` compares each one with its
  source, byte for byte.
- `{{version}}` and `{{release-date}}` are the top version of `CHANGELOG.md`
  and its date, read at build time.
- The rollout replay plays a recorded run of `scripts/demo.sh`:
  `demo-transcript.txt` is its output (without the setup line that names the
  local ports). `REPLAY` in `build.py` picks the lines, and the build writes
  them to `assets/replay.json` for `replay.js` and, as the static copy for
  readers without JavaScript or with reduced motion, into the page; the
  status strip's values are read from the same lines. `--check` compares the
  JSON and the static copy with the transcript. To record it again:
  `./scripts/demo.sh > web/demo-transcript.txt` without a terminal (no
  colours), then remove the `server ready on REST ...` line.
- The page loads nothing from other sites: no fonts, scripts, analytics or
  cookies. Its CSS and the JavaScript of the copy buttons and the screenshot
  tabs are in `index.html`; the replay is `replay.js`.

## Images

`web/make-assets.py` (Pillow) writes `web/assets/`, which is committed:

- `shot-*.png`, `hero.webp` and `hero.png`: the dashboard screenshots of
  `docs/screenshots/`, cropped to their content, nothing redrawn;
- `app-*.png`: the sample apps' screenshots, cropped to their status and
  inference part;
- `og.png`: the 1200x630 sharing image (the README's headline over the
  Deployments screenshot);
- `icon-192.png`, `apple-touch-icon.png`: the logo as PNG.

Run it again (`python3 web/make-assets.py`) when a screenshot or the logo changes.

`logo.svg` is the mark: a staged rollout as a staircase, up to a teal dot,
the whole fleet; indigo `#4f46e5` (the docs' primary colour) and teal
`#5eead4`. It is also the docs' header logo and favicon
(`scripts/mkdocs_hooks.py` adds it to the docs).

## The docs' theme

`docs-theme/` is MkDocs Material's `custom_dir`: one partial,
`partials/source.html`, the header's repository link without the call to
the GitHub API for its stars, so the docs load nothing from other sites.

## Hosting

Cloudflare Pages builds `main` with `make website` and serves `site/` at
https://fleetml.dev/ (`404.html` for unknown paths).
