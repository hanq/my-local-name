"""Generate the MY LOCAL NAME app and tray icons.

The mark is a baroque seal: gold hairlines on a plum ground, a cream medallion
and a pink mDNS signal glyph. Run from the repo root:

    python scripts/generate_icons.py
"""

from __future__ import annotations

import math
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parent.parent
ASSETS = ROOT / "assets"
BUILD = ROOT / "build"

INK = (20, 10, 17, 255)
PLUM = (43, 17, 34, 255)
CREAM = (247, 233, 228, 255)
PINK = (255, 93, 143, 255)
PINK_DEEP = (214, 31, 92, 255)
PINK_SOFT = (255, 179, 199, 255)
GOLD = (201, 162, 39, 235)
ROSE_GOLD = (192, 132, 151, 255)


def radial_gradient(size: int, inner, outer) -> Image.Image:
    img = Image.new("RGBA", (size, size))
    px = img.load()
    cx, cy = size * 0.52, size * 0.34
    max_dist = math.hypot(size, size)
    for y in range(size):
        for x in range(size):
            d = math.hypot(x - cx, y - cy) / max_dist
            t = min(1.0, d * 1.55)
            px[x, y] = tuple(round(inner[i] * (1 - t) + outer[i] * t) for i in range(4))
    return img


def draw_signal(draw: ImageDraw.ImageDraw, size: int, cx: float, cy: float,
                unit: float, colors: tuple, dot_scale: float = 0.18) -> None:
    """Draw a WiFi-like upward signal: a dot and three arcs."""
    dot_r = unit * dot_scale
    draw.ellipse((cx - dot_r, cy - dot_r, cx + dot_r, cy + dot_r), fill=colors[0])
    for i, radius in enumerate((unit * 0.55, unit * 0.95, unit * 1.35)):
        width = max(2, int(unit * (0.14 - i * 0.015)))
        box = (cx - radius, cy - radius, cx + radius, cy + radius)
        draw.arc(box, start=222, end=318, fill=colors[min(i + 1, len(colors) - 1)], width=width)


def make_app_icon() -> Image.Image:
    size = 2048
    base = radial_gradient(size, (72, 24, 52, 255), INK).convert("RGBA")
    draw = ImageDraw.Draw(base, "RGBA")

    cx = cy = size / 2
    # Baroque double seal ring.
    for radius, width, color in ((760, 10, GOLD), (700, 3, ROSE_GOLD)):
        draw.ellipse((cx - radius, cy - radius, cx + radius, cy + radius),
                     outline=color, width=width)

    # Corner hairlines, kept away from the seal.
    tick = 74
    offset = 118
    for sx in (-1, 1):
        for sy in (-1, 1):
            x = cx + sx * offset
            y = cy + sy * offset
            draw.line((x, y, x, y + sy * tick), fill=GOLD, width=5)
            draw.line((x, y, x + sx * tick, y), fill=GOLD, width=5)

    # Cream medallion behind the signal glyph.
    draw.ellipse((cx - 560, cy - 560, cx + 560, cy + 560), fill=CREAM)
    draw.ellipse((cx - 560, cy - 560, cx + 560, cy + 560), outline=INK, width=22)

    draw_signal(draw, size, cx, cy + 250, 330,
                (INK, PINK_DEEP, PINK, PINK_SOFT))

    # Small wax-seal accent below the glyph.
    draw.ellipse((cx - 46, cy + 430, cx + 46, cy + 522), fill=PLUM)
    draw.ellipse((cx - 46, cy + 430, cx + 46, cy + 522), outline=GOLD, width=5)

    # Subtle vignette for depth.
    vignette = Image.new("L", (size, size), 0)
    vdraw = ImageDraw.Draw(vignette)
    vdraw.ellipse((size * 0.06, size * 0.06, size * 0.94, size * 0.94), fill=255)
    vignette = vignette.filter(ImageFilter.GaussianBlur(size // 12))
    dark = Image.new("RGBA", (size, size), (10, 4, 8, 255))
    base = Image.composite(base, dark, vignette)

    return base.resize((1024, 1024), Image.LANCZOS)


def make_tray_icon() -> Image.Image:
    size = 256
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img, "RGBA")
    margin = 12
    draw.ellipse((margin, margin, size - margin, size - margin), fill=CREAM)
    draw.ellipse((margin, margin, size - margin, size - margin), outline=INK, width=7)
    draw.ellipse((margin + 22, margin + 22, size - margin - 22, size - margin - 22),
                 outline=ROSE_GOLD, width=2)
    draw_signal(draw, size, size / 2, size * 0.69, 78,
                (INK, PINK_DEEP, PINK, PINK), dot_scale=0.16)
    return img.resize((64, 64), Image.LANCZOS)


def make_template_icon() -> Image.Image:
    """macOS menu bar template: black with alpha only."""
    size = 256
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    draw = ImageDraw.Draw(img, "RGBA")
    black = (0, 0, 0, 255)
    draw_signal(draw, size, size / 2, size * 0.68, 84,
                (black, black, black, black), dot_scale=0.16)
    return img.resize((32, 32), Image.LANCZOS)


def main() -> None:
    ASSETS.mkdir(exist_ok=True)
    BUILD.mkdir(exist_ok=True)

    app_icon = make_app_icon()
    app_icon.save(ASSETS / "appicon.png")
    app_icon.save(BUILD / "appicon.png")

    tray = make_tray_icon()
    tray.save(ASSETS / "tray.png")
    make_template_icon().save(ASSETS / "tray_template.png")

    print("wrote assets/appicon.png, assets/tray.png, assets/tray_template.png, build/appicon.png")


if __name__ == "__main__":
    main()
