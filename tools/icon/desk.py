"""MyMind's icon, the desk: a white note on the app's indigo, three lines
on it, each led by a dot of a map's color (coral, amber, green), the
last ticked; the AI's spark at its corner. The colors are the app's own:
what is written belongs to a map, and each map has its color.

    python3 tools/icon/desk.py out.png [size]
"""
import sys

from PIL import Image

from concepts import base, layers, sparkle
from make import S, SHAPE, shadow

INDIGO = [(0, "#7878E6"), (0.55, "#5B5BD6"), (1, "#4040A8")]
INK = "#1C1B1A"


def desk(canvas):
    note = '<rect x="232" y="226" width="520" height="580" rx="104" fill="{c}"/>'
    rows = ""
    for y, color, width, opacity in ((390, "#E8684A", 250, 1), (516, "#E8A13A", 190, 0.55)):
        rows += (f'<circle cx="345" cy="{y}" r="34" fill="{color}"/>'
                 f'<path d="M430,{y} L{430 + width},{y}" stroke="{INK}" stroke-width="42" '
                 f'stroke-linecap="round" stroke-opacity="{opacity}"/>')
    # the last line, done: a green disc with a tick, its line lighter
    rows += ('<circle cx="345" cy="652" r="46" fill="#3DAA7D"/>'
             '<path d="M322,654 L339,671 L370,636" fill="none" stroke="#FFFFFF" stroke-width="20" '
             'stroke-linecap="round" stroke-linejoin="round"/>'
             f'<path d="M430,652 L640,652" stroke="{INK}" stroke-width="42" stroke-linecap="round" stroke-opacity="0.28"/>')
    layers(canvas,
           ("", base(INDIGO, glow_opacity=0.25)),
           ("shadow", note.format(c="#000")),
           ("", note.format(c="#FFFFFF") + rows),
           ("soft", sparkle(770, 286, 95, "#000")),
           ("", sparkle(770, 286, 112, "#FFFFFF", k=0.14)))


if __name__ == "__main__":
    out = sys.argv[1]
    size = int(sys.argv[2]) if len(sys.argv) > 2 else S
    canvas = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    canvas.alpha_composite(shadow(f'<path d="{SHAPE}" fill="#000"/>', 14, 10, 0.32))
    desk(canvas)
    if size != S:
        canvas = canvas.resize((size, size), Image.LANCZOS)
    canvas.save(out)
