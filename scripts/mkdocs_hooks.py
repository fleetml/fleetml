"""MkDocs hooks for the FleetML docs site.

README.md and CHANGELOG.md live at the repository root, outside docs/: they
are added as the site's home page and changelog; the home page leaves out the
README's CI badge, an image from github.com, so the site loads nothing from
other hosts, and web/logo.svg becomes the header logo and favicon. Links from
any page to files outside docs/ (source, deploy files, SECURITY.md) are
rewritten to the repository on GitHub, so the site has no broken relative
links.
"""

import os
import re

from mkdocs.structure.files import File

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
# site page -> repository file
ROOT_PAGES = {"index.md": "README.md", "CHANGELOG.md": "CHANGELOG.md"}
# Files under docs/ that the site serves; anything else (scripts) links to the repository.
SITE_EXTENSIONS = (".md", ".png", ".gif", ".jpg", ".svg")
EXCLUDED = ("docs/screenshots/demo/",)
REPO_PAGES = {v: k for k, v in ROOT_PAGES.items()}
BADGE = re.compile(r"^\[!\[CI\]\([^)]*\)\]\([^)]*\)\n\n", re.M)
LINK = re.compile(r"(!?\[[^\]]*\]\()([^)\s]+)(\))")


def on_files(files, config):
    for page, source in ROOT_PAGES.items():
        with open(os.path.join(ROOT, source), encoding="utf-8") as f:
            content = f.read()
        if page == "index.md":
            content = BADGE.sub("", content, count=1)
        files.append(File.generated(config, page, content=content))
    with open(os.path.join(ROOT, "web", "logo.svg"), encoding="utf-8") as f:
        files.append(File.generated(config, "assets/fleetml-logo.svg", content=f.read()))
    return files


def site_link(target, page_dir, repo_url):
    """Maps a relative link in a page whose source is in page_dir (repo-relative)."""
    if re.match(r"^([a-z][a-z0-9+.-]*:|#|/)", target):
        return target
    path, hash_, frag = target.partition("#")
    repo_path = os.path.normpath(os.path.join(page_dir, path))
    if repo_path in REPO_PAGES:
        return REPO_PAGES[repo_path] + hash_ + frag
    is_dir = os.path.isdir(os.path.join(ROOT, repo_path))
    served = repo_path.endswith(SITE_EXTENSIONS) and not repo_path.startswith(EXCLUDED)
    if repo_path.startswith("docs/") and not is_dir and served:
        return repo_path[len("docs/"):] + hash_ + frag
    kind = "tree" if is_dir else "blob"
    return f"{repo_url.rstrip('/')}/{kind}/main/{repo_path}" + hash_ + frag


def on_page_markdown(markdown, page, config, files):
    src = page.file.src_uri
    page_dir = "" if src in ROOT_PAGES else os.path.dirname(os.path.join("docs", src))
    return LINK.sub(lambda m: m.group(1) + site_link(m.group(2), page_dir, config["repo_url"]) + m.group(3), markdown)
