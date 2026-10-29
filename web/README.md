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
- `{{version}}` is the top version of `CHANGELOG.md`, read at build time.
- The page loads nothing from other sites: no fonts, scripts, analytics or
  cookies. Its CSS and the few lines of JavaScript (copy buttons, the
  screenshot tabs) are in `index.html`.

## Images

`web/make-assets.py` (Pillow) writes `web/assets/`, which is committed:

- `shot-*.png`: the dashboard screenshots of `docs/screenshots/`, cropped to
  their content, nothing redrawn;
- `og.png`: the 1200x630 sharing image (the README's headline over the
  Deployments screenshot);
- `icon-192.png`, `apple-touch-icon.png`: the logo as PNG.

Run it again (`python3 web/make-assets.py`) when a screenshot or the logo changes.

`logo.svg` is the mark: three bars for the stages of a rollout, the last one,
every device, in the accent colour; indigo `#4f46e5` (the docs' primary
colour) and teal `#5eead4`.

## Hosting

Cloudflare Pages builds `main` with `make website` and serves `site/` at
https://fleetml.dev/ (`404.html` for unknown paths).
