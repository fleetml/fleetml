#!/usr/bin/env python3
"""Regenerates the website's images in web/assets/ from the repository's own files.

Run it after a dashboard screenshot or the logo changes; its output is committed,
so `make website` needs no image tools. Needs Pillow.

- shot-*.png: real dashboard screenshots from docs/screenshots/, cropped to
  the part with content and reduced to 128 colours (nothing is drawn or edited).
- og.png: the 1200x630 sharing image, the README's headline over the
  Deployments screenshot.
- icon-192.png and apple-touch-icon.png: web/logo.svg rasterised (the mark is
  rectangles only, drawn again here with the same coordinates).

Usage: python3 web/make-assets.py   (from the repository root)
"""

import os
import sys

from PIL import Image, ImageDraw, ImageFont

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "web", "assets")

# name -> (source screenshot, crop box (left, top, right, bottom) in its 1280x860 pixels)
SHOTS = {
    "deployments": ("dashboard-1280-deployments.png", (0, 0, 1280, 440)),
    "drift": ("dashboard-1280-drift.png", (0, 0, 1280, 500)),
    "rollback": ("dashboard-1280-rollback-confirm.png", (0, 0, 1280, 600)),
}
HEADLINE = ("Ship ML models to phones and edge devices in stages, watch how each version "
            "behaves, and roll back automatically when one is worse.")
FONTS = ["/System/Library/Fonts/Supplemental/Arial Bold.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"]
INDIGO, TEAL, WHITE, INK = (79, 70, 229), (94, 234, 212), (255, 255, 255), (17, 24, 39)


def font(size):
    for path in FONTS:
        if os.path.exists(path):
            return ImageFont.truetype(path, size)
    sys.exit("no bold TrueType font found; add one to FONTS")


def mark(size):
    """The logo, rasterised: web/logo.svg's rectangles on a 64-unit grid."""
    s = size / 64
    im = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    d.rounded_rectangle((0, 0, size - 1, size - 1), radius=14 * s, fill=INDIGO)
    for x, y, h, colour in ((13, 38, 13, WHITE), (27, 27, 24, WHITE), (41, 14, 37, TEAL)):
        d.rounded_rectangle((x * s, y * s, (x + 10) * s, (y + h) * s), radius=2 * s, fill=colour)
    return im


def wrap(draw, text, fnt, width):
    lines, line = [], ""
    for word in text.split():
        trial = f"{line} {word}".strip()
        if draw.textlength(trial, font=fnt) <= width:
            line = trial
        else:
            lines.append(line)
            line = word
    return lines + [line]


def main():
    os.makedirs(OUT, exist_ok=True)
    for name, (src, box) in SHOTS.items():
        im = Image.open(os.path.join(ROOT, "docs", "screenshots", src)).convert("RGB").crop(box)
        im.quantize(colors=128, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE).save(
            os.path.join(OUT, f"shot-{name}.png"), optimize=True)

    og = Image.new("RGB", (1200, 630), WHITE)
    d = ImageDraw.Draw(og)
    shot = Image.open(os.path.join(ROOT, "docs", "screenshots", SHOTS["deployments"][0])).convert("RGB")
    shot = shot.crop((0, 0, 1280, 440)).resize((1040, 358), Image.LANCZOS)
    og.paste(shot, (80, 252))
    d.rectangle((80, 252, 1119, 609), outline=(209, 213, 219), width=2)
    og.paste(mark(64), (80, 44), mark(64))
    d.text((160, 58), "FleetML", font=font(34), fill=INK)
    fnt = font(30)
    for i, line in enumerate(wrap(d, HEADLINE, fnt, 1040)):
        d.text((80, 128 + i * 40), line, font=fnt, fill=INK)
    og.save(os.path.join(OUT, "og.png"), optimize=True)

    mark(192).save(os.path.join(OUT, "icon-192.png"), optimize=True)
    touch = Image.new("RGB", (180, 180), INDIGO)
    touch.paste(mark(180), (0, 0), mark(180))
    touch.save(os.path.join(OUT, "apple-touch-icon.png"), optimize=True)
    for n in sorted(os.listdir(OUT)):
        print(f"{n:28} {os.path.getsize(os.path.join(OUT, n)):>8} bytes")


if __name__ == "__main__":
    main()
