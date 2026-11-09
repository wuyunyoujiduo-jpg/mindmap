"""MindMap's icon, vivid: a large white note on a bright color, written and
a task on it ticked, the AI's spark at its corner: notes, tasks and the
assistant, in a few large shapes that stand out in the Dock.

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
    """A white note: two lines written, a task ticked, and the AI's spark
    at its corner, in the color's deep shade."""
    ink = colors[1][1]
    note = '<rect x="250" y="240" width="470" height="560" rx="96" fill="{c}"/>'
    lines = (f'<path d="M345,420 L560,420" stroke="{ink}" stroke-width="44" stroke-linecap="round"/>'
             f'<path d="M345,540 L500,540" stroke="{ink}" stroke-width="44" stroke-linecap="round" stroke-opacity="0.45"/>'
             f'<path d="M350,665 L410,722 L530,610" fill="none" stroke="{ink}" stroke-width="48" '
             f'stroke-linecap="round" stroke-linejoin="round"/>')
    layers(canvas,
           ("", base(colors, glow_opacity=0.3)),
           ("shadow", note.format(c="#000")),
           ("", note.format(c="#FFFFFF") + lines),
           ("soft", sparkle(745, 300, 95, "#000")),
           ("", sparkle(745, 300, 115, "#FFFFFF", k=0.14)))


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
