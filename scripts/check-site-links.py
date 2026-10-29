#!/usr/bin/env python3
"""Checks the links of the built docs site (site/) without network access.

Every relative href and src must resolve to a file in the site, and every
#fragment to an id on the target page. External links are counted, not
fetched. Usage: check-site-links.py [site_dir]
"""

import os
import sys
from html.parser import HTMLParser
from urllib.parse import unquote, urlsplit


class Page(HTMLParser):
    def __init__(self):
        super().__init__()
        self.links, self.ids = [], set()

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if a.get("id"):
            self.ids.add(a["id"])
        for key in ("href", "src"):
            if a.get(key) and not (tag == "link" and a.get("rel") in ("canonical", "alternate")):
                self.links.append(a[key])


def parse(path):
    p = Page()
    with open(path, encoding="utf-8") as f:
        p.feed(f.read())
    return p


def main():
    site = os.path.abspath(sys.argv[1] if len(sys.argv) > 1 else "site")
    pages, errors, external = {}, [], set()
    for d, _, names in os.walk(site):
        for n in names:
            if n.endswith(".html"):
                pages[os.path.join(d, n)] = parse(os.path.join(d, n))
    checked = fragments = 0
    for path, page in pages.items():
        for link in page.links:
            u = urlsplit(link)
            if u.scheme in ("http", "https"):
                external.add(link)
                continue
            if u.scheme or link.startswith("//"):
                continue  # mailto:, data:, javascript:
            if not u.path:
                target = path
            elif u.path.startswith("/"):  # root-relative, as on the 404 page
                target = os.path.normpath(os.path.join(site, unquote(u.path).lstrip("/")))
            else:
                target = os.path.normpath(os.path.join(os.path.dirname(path), unquote(u.path)))
            if os.path.isdir(target):
                target = os.path.join(target, "index.html")
            checked += 1
            if not os.path.exists(target):
                errors.append(f"{os.path.relpath(path, site)}: {link}: no such file")
            elif u.fragment and target.endswith(".html"):
                fragments += 1
                ids = pages[target].ids if target in pages else parse(target).ids
                if unquote(u.fragment) not in ids:
                    errors.append(f"{os.path.relpath(path, site)}: {link}: no #{u.fragment} on the page")
    print(f"{len(pages)} pages, {checked} internal links checked ({fragments} with a #fragment, each found"
          f" among the ids of its page), {len(external)} external links (not fetched)")
    for e in sorted(set(errors)):
        print("BROKEN", e)
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main())
