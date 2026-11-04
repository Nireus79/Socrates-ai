#!/usr/bin/env python3
"""Makes every icon and store image from the two originals in assets/brand/ (needs Pillow only). Run from anywhere:

    python3 assets/make-icons.py

The launcher icon is the head alone (the vertical wordmark cannot be read at that size), drawn with a slightly heavier line so that it holds
at small sizes, on the cream of the original logo. The wordmark stays on the welcome screen, the About card, the splash and the store
graphic. Outputs are overwritten; the originals are never touched.
"""
import os
from PIL import Image, ImageDraw, ImageFilter

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)
CREAM = (225, 222, 207, 255)
INK = (38, 34, 30, 255)

mark = Image.open(os.path.join(HERE, "brand", "socrates-ai-mark.png")).convert("RGBA")
head = mark.crop((100, 0, 282, 500))  # the head; the wordmark starts at x=288
head = head.crop(head.getbbox())


def heavy(img, k):
    """The same drawing in one ink colour with a line k pixels heavier."""
    alpha = img.split()[3].filter(ImageFilter.MaxFilter(k))
    out = Image.new("RGBA", img.size, INK)
    out.putalpha(alpha)
    return out


def head_on(canvas_size, height_frac, background):
    """The head, centred as if the face were whole, on a square canvas, built large and brought down so the line stays even. The smaller the head ends up, the heavier
    the line is drawn, so that it holds."""
    big = 1024
    h = head.copy()
    s = big * height_frac / h.height
    h = h.resize((round(h.width * s), round(h.height * s)), Image.LANCZOS)
    shown = canvas_size * height_frac
    h = heavy(h, 5 if shown >= 400 else 7 if shown >= 250 else 9 if shown >= 120 else 13)
    c = Image.new("RGBA", (big, big), background)
    # The drawing is half a face (the line down its right edge is the middle of the face), so it is placed as if the face were whole: that
    # edge stands at the centre of the canvas, and the half face lies to the left of it.
    c.alpha_composite(h, (big // 2 - h.width, (big - h.height) // 2))
    return c.resize((canvas_size, canvas_size), Image.LANCZOS)


def rounded(img, radius_frac):
    m = Image.new("L", img.size, 0)
    ImageDraw.Draw(m).rounded_rectangle((0, 0, img.width - 1, img.height - 1), radius=round(img.width * radius_frac), fill=255)
    out = Image.new("RGBA", img.size, (0, 0, 0, 0))
    out.paste(img, (0, 0), m)
    return out


def circle(img):
    m = Image.new("L", img.size, 0)
    ImageDraw.Draw(m).ellipse((0, 0, img.width - 1, img.height - 1), fill=255)
    out = Image.new("RGBA", img.size, (0, 0, 0, 0))
    out.paste(img, (0, 0), m)
    return out


def save(img, *parts, rgb=False):
    path = os.path.join(ROOT, *parts)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    (img.convert("RGB") if rgb else img).save(path, optimize=True)


def full_mark_on_cream(size):
    """The logo with its wordmark, on cream, centred on a canvas of the given width and height."""
    w, h = size
    c = Image.new("RGBA", size, CREAM)
    side = min(w, h)
    m = mark.resize((side, side), Image.LANCZOS)
    c.alpha_composite(m, ((w - side) // 2, (h - side) // 2))
    return c


def original_icon(size):
    """The founder's original logo (assets/brand/socrates-original.png, with its wordmark) cropped to a square around the drawing, then scaled."""
    from PIL import ImageChops
    o = Image.open(os.path.join(ROOT, "assets", "brand", "socrates-original.png")).convert("RGB")
    bg = Image.new("RGB", o.size, o.getpixel((2, 2)))
    d = ImageChops.difference(o, bg).point(lambda v: 255 if v > 24 else 0)
    x0, y0, x1, y1 = d.getbbox()
    # the face is half a face: its straight edge (the rightmost point of the head, above the wordmark) goes in the middle, as if it were whole
    mid = d.crop((0, 0, o.width, y0 + 186)).getbbox()[2]
    half = max(mid - x0, x1 - mid, (y1 - y0) // 2)
    side = round(2 * half * 1.14)
    cx, cy = mid, (y0 + y1) // 2
    box = (cx - side // 2, cy - side // 2, cx - side // 2 + side, cy - side // 2 + side)
    return o.crop(box).resize((size, size), Image.LANCZOS).convert("RGBA")


# Android launcher: legacy square and round icons, and the adaptive foreground (108 dp, the visible part is the middle 66 dp)
res = ("mobile", "android", "app", "src", "main", "res")
for name, legacy, adaptive in (("mdpi", 48, 108), ("hdpi", 72, 162), ("xhdpi", 96, 216), ("xxhdpi", 144, 324), ("xxxhdpi", 192, 432)):
    base = original_icon(legacy)
    save(rounded(base, 0.16), *res, "mipmap-" + name, "ic_launcher.png")
    save(circle(base), *res, "mipmap-" + name, "ic_launcher_round.png")
    fg = Image.new("RGBA", (adaptive, adaptive), CREAM)  # the cream is the adaptive background's colour, so the join does not show
    inner = round(adaptive * 0.74)
    fg.alpha_composite(original_icon(inner), ((adaptive - inner) // 2, (adaptive - inner) // 2))
    save(fg, *res, "mipmap-" + name, "ic_launcher_foreground.png")
with open(os.path.join(ROOT, *res, "values", "ic_launcher_background.xml"), "w") as f:
    f.write('<?xml version="1.0" encoding="utf-8"?>\n<resources>\n    <color name="ic_launcher_background">#E1DECF</color>\n</resources>\n')

# Android splash: every existing splash.png keeps its size and gets the logo on cream
for dirpath, _, files in os.walk(os.path.join(ROOT, *res)):
    for fn in files:
        if fn == "splash.png":
            p = os.path.join(dirpath, fn)
            size = Image.open(p).size
            c = Image.new("RGBA", size, CREAM)
            side = round(min(size) * 0.62)
            c.alpha_composite(mark.resize((side, side), Image.LANCZOS), ((size[0] - side) // 2, (size[1] - side) // 2))
            c.convert("RGB").save(p, optimize=True)

# Play listing: the 512 icon (Play applies its own mask) and the 1024x500 feature graphic
# The two store icons are large enough to carry the vertical wordmark, so they show the whole logo; the launcher and window icons stay head-only.
save(original_icon(512), "mobile", "store", "play-icon-512.png", rgb=True)
save(original_icon(512), "fastlane", "metadata", "android", "en-US", "images", "icon.png", rgb=True)
save(full_mark_on_cream((1024, 500)), "mobile", "store", "feature-graphic-1024x500.png", rgb=True)

# Desktop window and the screens (shared by the desktop window and the phone)
save(original_icon(512), "desktop", "icon.png")
save(original_icon(192), "app", "icon.png")
save(mark.resize((500, 500), Image.LANCZOS), "app", "mark.png")  # transparent: the screens draw it on their own background (dark theme inverts it in CSS)
print("icons written")
