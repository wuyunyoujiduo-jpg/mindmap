"""Draws the menu bar's icons, black on clear for macOS to tint (template
images), 44 pixels for 22 points: the knowledge cards and the
constellation, as the app's icons.

    python3 tools/icon/tray.py
"""
import io

import cairosvg
from PIL import Image

from concepts import sparkle

S = 44


def save(body, out):
    svg = f'<svg xmlns="http://www.w3.org/2000/svg" width="{S}" height="{S}" viewBox="0 0 {S} {S}">{body}</svg>'
    png = cairosvg.svg2png(bytestring=svg.encode(), output_width=S, output_height=S)
    Image.open(io.BytesIO(png)).convert("RGBA").save(out, optimize=True)


# A card, a map on it, and the spark at its corner.
cards = (
    '<g transform="rotate(-8 18 25)"><rect x="6" y="11" width="22" height="27" rx="4.5" fill="none" stroke="#000" stroke-width="3"/></g>'
    '<circle cx="12.5" cy="25" r="2.6" fill="#000"/>'
    '<path d="M15 25 C18 25 18 20 21.5 20 M15 25 L21.5 25 M15 25 C18 25 18 30 21.5 30" fill="none" stroke="#000" stroke-width="2.2" stroke-linecap="round"/>'
    + sparkle(33, 11, 9.5, "#000", k=0.16)
)
# The spark, and knowledge around it, joined to it.
pts = [(9, 10), (36, 9), (39, 30), (21, 39), (5, 29)]
orbit = (
    "".join(f'<line x1="22" y1="22" x2="{x}" y2="{y}" stroke="#000" stroke-width="1.8" stroke-opacity="0.75"/>' for x, y in pts)
    + "".join(f'<rect x="{x-3.2}" y="{y-3.2}" width="6.4" height="6.4" rx="1.8" fill="#000"/>' for x, y in pts)
    + sparkle(22, 22, 13, "#000", k=0.14)
)

if __name__ == "__main__":
    save(cards, "appicons/tray-cards.png")
    save(orbit, "appicons/tray-orbit.png")
