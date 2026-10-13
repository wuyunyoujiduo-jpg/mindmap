"""MindMap's icon, bold: one card filling the icon, a large map on it and
the AI's spark at its corner, in few large shapes that read in the Dock.

    python3 tools/icon/bold.py out.png [size]
"""
import sys

from PIL import Image

from concepts import GRAD, base, layers, sparkle
from make import S, SHAPE, shadow


def bold(canvas):
    card = '<rect x="180" y="250" width="610" height="570" rx="110" fill="{c}"/>'
    cx, cy = 340, 516
    leaves = [(640, 372, "#2ED39A"), (640, 516, "#FFB938"), (640, 660, "#FF5C7A")]
    branches = "".join(
        f'<path d="M{cx},{cy} C{cx+150},{cy} {cx+130},{ly} {lx-60},{ly}" fill="none" stroke="#C3CCF4" '
        f'stroke-width="40" stroke-linecap="round"/>' for lx, ly, _ in leaves)
    pills = "".join(f'<rect x="{lx-82}" y="{ly-46}" width="164" height="92" rx="46" fill="{c}"/>'
                    for lx, ly, c in leaves)
    center = f'<circle cx="{cx}" cy="{cy}" r="74" fill="#4655F0"/>'
    layers(canvas,
           ("", base([(0, "#6F7DFF"), (0.55, "#5146E8"), (1, "#3B22B8")], glow_opacity=0.28)),
           ("shadow", card.format(c="#000")),
           ("", card.format(c="#FFFFFF") + branches + pills + center),
           ("soft", sparkle(752, 290, 120, "#2A1A8F")),
           ("", sparkle(752, 290, 142, "#FFFFFF")),
           ("", f"<defs>{GRAD}</defs>" + sparkle(752, 290, 112, "url(#ai)")))


if __name__ == "__main__":
    canvas = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    canvas.alpha_composite(shadow(f'<path d="{SHAPE}" fill="#000"/>', 14, 10, 0.32))
    bold(canvas)
    size = int(sys.argv[2]) if len(sys.argv) > 2 else S
    if size != S:
        canvas = canvas.resize((size, size), Image.LANCZOS)
    canvas.save(sys.argv[1])
