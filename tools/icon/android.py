"""The foreground of MyMind's icon on Android (mygo.json's
android.adaptiveIcon): desk.py's note and spark with no ground, made small
enough for launchers, which crop to the middle of it, a circle or a
squircle, and may move it. The ground is the indigo of desk.py, as a color.

    python3 tools/icon/android.py resources/android-foreground.png [size]
"""
import sys

from PIL import Image

from concepts import layers, sparkle
from desk import INK
from make import S

SCALE = 0.62  # the note and its spark inside the circle launchers keep


def foreground():
    canvas = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    note = '<rect x="232" y="226" width="520" height="580" rx="104" fill="{c}"/>'
    rows = ""
    for y, color, width, opacity in ((390, "#E8684A", 250, 1), (516, "#E8A13A", 190, 0.55)):
        rows += (f'<circle cx="345" cy="{y}" r="34" fill="{color}"/>'
                 f'<path d="M430,{y} L{430 + width},{y}" stroke="{INK}" stroke-width="42" '
                 f'stroke-linecap="round" stroke-opacity="{opacity}"/>')
    rows += ('<circle cx="345" cy="652" r="46" fill="#3DAA7D"/>'
             '<path d="M322,654 L339,671 L370,636" fill="none" stroke="#FFFFFF" stroke-width="20" '
             'stroke-linecap="round" stroke-linejoin="round"/>'
             f'<path d="M430,652 L640,652" stroke="{INK}" stroke-width="42" stroke-linecap="round" stroke-opacity="0.28"/>')
    layers(canvas,
           ("shadow", note.format(c="#000")),
           ("", note.format(c="#FFFFFF") + rows),
           ("soft", sparkle(770, 286, 95, "#000")),
           ("", sparkle(770, 286, 112, "#FFFFFF", k=0.14)))
    # The note and the spark, centered, at SCALE.
    box = (232, 174, 882, 806)
    part = canvas.crop(box)
    w, h = int(part.width * SCALE), int(part.height * SCALE)
    part = part.resize((w, h), Image.LANCZOS)
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    out.alpha_composite(part, ((S - w) // 2, (S - h) // 2))
    return out


if __name__ == "__main__":
    size = int(sys.argv[2]) if len(sys.argv) > 2 else 512
    img = foreground()
    if size != S:
        img = img.resize((size, size), Image.LANCZOS)
    img.save(sys.argv[1])
