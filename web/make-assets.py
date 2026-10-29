#!/usr/bin/env python3
"""Regenerates the website's images in web/assets/ from the repository's own files.

Run it after a dashboard screenshot or the logo changes; its output is committed,
so `make website` needs no image tools. Needs Pillow.

- shot-*.png: real dashboard screenshots from docs/screenshots/, cropped to
  the part with content and reduced to 128 colours (nothing is drawn or edited);
  hero.webp and hero.png: the Deployments screenshot for the top of the page,
  without the sidebar, lossless WebP with the PNG as the fallback.
- app-ios.png and app-android.png: the sample apps' screenshots from
  docs/screenshots/, cropped to their status and inference part (the Android
  one shows the enrollment key in clear above that part).
- og.png: the 1200x630 sharing image, the README's headline over the
  Deployments screenshot.
- icon-192.png and apple-touch-icon.png: web/logo.svg rasterised (the mark is
  a rounded square, a line and a dot, drawn again here with the same coordinates).

Usage: python3 web/make-assets.py   (from the repository root)
"""

import os
import sys

from PIL import Image, ImageDraw, ImageFont

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "web", "assets")

# name -> (source screenshot, crop box (left, top, right, bottom) in its 1280x860 pixels)
# The hero: the Deployments table without the dashboard's sidebar, so it reads at half width.
HERO = ("dashboard-1280-deployments.png", (232, 0, 1264, 420))
SHOTS = {
    "drift": ("dashboard-1280-drift.png", (0, 0, 1280, 500)),
    "rollback": ("dashboard-1280-rollback-confirm.png", (0, 0, 1280, 600)),
    "experiments": ("dashboard-1280-experiments.png", (0, 0, 1280, 500)),
}
# name -> (source screenshot, crop box in its pixels)
APPS = {
    "ios": ("ios-sample.png", (0, 500, 460, 870)),
    "android": ("android-sample.png", (0, 345, 540, 602)),
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
    """The logo, rasterised: web/logo.svg's square, staircase and dot on a 64-unit grid."""
    k = 4  # drawn 4x larger, then reduced, for smooth edges
    s = size * k / 64
    im = Image.new("RGBA", (size * k, size * k), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    d.rounded_rectangle((0, 0, size * k - 1, size * k - 1), radius=14 * s, fill=INDIGO)
    points = [(12, 48), (24, 48), (24, 36), (36, 36), (36, 24), (44, 24)]
    w = 7 * s
    for (x1, y1), (x2, y2) in zip(points, points[1:]):
        d.line((x1 * s, y1 * s, x2 * s, y2 * s), fill=WHITE, width=round(w))
    for x, y in points:
        d.ellipse((x * s - w / 2, y * s - w / 2, x * s + w / 2, y * s + w / 2), fill=WHITE)
    d.ellipse(((47 - 7) * s, (19 - 7) * s, (47 + 7) * s, (19 + 7) * s), fill=TEAL)
    return im.resize((size, size), Image.LANCZOS)


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
    hero = Image.open(os.path.join(ROOT, "docs", "screenshots", HERO[0])).convert("RGB").crop(HERO[1])
    hero.save(os.path.join(OUT, "hero.webp"), lossless=True, method=6)
    hero.quantize(colors=128, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE).save(
        os.path.join(OUT, "hero.png"), optimize=True)
    for name, (src, box) in APPS.items():
        im = Image.open(os.path.join(ROOT, "docs", "screenshots", src)).convert("RGB").crop(box)
        im.quantize(colors=128, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE).save(
            os.path.join(OUT, f"app-{name}.png"), optimize=True)

    og = Image.new("RGB", (1200, 630), WHITE)
    d = ImageDraw.Draw(og)
    shot = Image.open(os.path.join(ROOT, "docs", "screenshots", HERO[0])).convert("RGB")
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
