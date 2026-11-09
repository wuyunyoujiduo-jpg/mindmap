"""MindMap's icon, vivid: a large white map filling a bright icon, the AI's
spark by it, in a few large shapes that stand out in the Dock.

    python3 tools/icon/vivid.py out.png [size] [blue|coral|teal]
"""
import sys

from PIL import Image

from concepts import base, layers, sparkle
from make import S, SHAPE, shadow

COLORS = {
    "blue": [(0, "#5C7CFF"), (0.55, "#4A4BF0"), (1, "#3A23C8")],
    "coral": [(0, "#FF9A5A"), (0.55, "#FF6B5E"), (1, "#E2406E")],
    "teal": [(0, "#3FD9B5"), (0.55, "#14B39C"), (1, "#0B7F86")],
}


def vivid(canvas, colors):
    cx, cy = 318, 524
    leaves = [(706, 318), (742, 524), (706, 730)]
    lw, lh = 196, 104
    branches = "".join(
        f'<path d="M{cx+60},{cy} C{cx+210},{cy} {lx-lw/2-150},{ly} {lx-lw/2+20},{ly}" fill="none" '
        f'stroke="{{c}}" stroke-width="54" stroke-linecap="round"/>' for lx, ly in leaves)
    pills = "".join(f'<rect x="{lx-lw/2}" y="{ly-lh/2}" width="{lw}" height="{lh}" rx="{lh/2}" fill="{{c}}"/>'
                    for lx, ly in leaves)
    center = f'<circle cx="{cx}" cy="{cy}" r="136" fill="{{c}}"/>'
    shapes = branches + pills + center
    layers(canvas,
           ("", base(colors, glow_opacity=0.3)),
           ("shadow", shapes.format(c="#000")),
           ("", shapes.format(c="#FFFFFF")),
           ("", sparkle(cx, cy, 70, colors[1][1])),  # the spark at the map's heart
           ("soft", sparkle(760, 230, 70, "#000")),
           ("", sparkle(760, 230, 86, "#FFFFFF")))


if __name__ == "__main__":
    out = sys.argv[1]
    size = int(sys.argv[2]) if len(sys.argv) > 2 else S
    colors = COLORS[sys.argv[3] if len(sys.argv) > 3 else "blue"]
    canvas = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    canvas.alpha_composite(shadow(f'<path d="{SHAPE}" fill="#000"/>', 14, 10, 0.32))
    vivid(canvas, colors)
    if size != S:
        canvas = canvas.resize((size, size), Image.LANCZOS)
    canvas.save(out)
