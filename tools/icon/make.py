"""Draws MindMap's app icon, resources/icon.png, in the shape and grid of
macOS icons: a rounded square of continuous corners, 824 points in a
canvas of 1024, with its shadow; on it, a small mind map.

    python3 tools/icon/make.py [out.png]
"""
import io
import math
import sys

import cairosvg
from PIL import Image, ImageFilter

S = 1024          # the canvas
B, O = 824, 100   # the body and its offset


def squircle(x, y, w, h, n=5.0, steps=720):
    """A superellipse, the continuous corners of macOS icons."""
    cx, cy, a, b = x + w / 2, y + h / 2, w / 2, h / 2
    pts = []
    for i in range(steps):
        t = 2 * math.pi * i / steps
        c, s = math.cos(t), math.sin(t)
        px = cx + a * math.copysign(abs(c) ** (2 / n), c)
        py = cy + b * math.copysign(abs(s) ** (2 / n), s)
        pts.append(f"{px:.2f},{py:.2f}")
    return "M" + " L".join(pts) + " Z"


def svg(body):
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="{S}" height="{S}" viewBox="0 0 {S} {S}">{body}</svg>'


def render(body):
    png = cairosvg.svg2png(bytestring=svg(body).encode(), output_width=S, output_height=S)
    return Image.open(io.BytesIO(png)).convert("RGBA")


def shadow(body, blur, dy, alpha):
    """The shapes' shadow: blurred, moved down, faint."""
    img = render(body)
    a = img.getchannel("A").point(lambda v: v * alpha)
    sh = Image.new("RGBA", img.size, (10, 20, 60, 0))
    sh.putalpha(a)
    sh = sh.filter(ImageFilter.GaussianBlur(blur))
    out = Image.new("RGBA", img.size, (0, 0, 0, 0))
    out.paste(sh, (0, dy), sh)
    return out


SHAPE = squircle(O, O, B, B)

# The map: a center node, and three branches flowing right to its ideas.
CX, CY = 335, 512                      # the center node
LEAVES = [(700, 312, "#5EE2B0"), (732, 512, "#FFC94D"), (700, 712, "#FF7B95")]
LW, LH = 164, 94                        # a leaf


def branches(color, width):
    out = []
    x0 = CX + 95
    for lx, ly, _ in LEAVES:
        x1 = lx - LW / 2 + 6
        mid = (x0 + x1) / 2
        out.append(f'<path d="M{x0},{CY} C{mid},{CY} {mid},{ly} {x1},{ly}" fill="none" '
                   f'stroke="{color}" stroke-width="{width}" stroke-linecap="round"/>')
    return "".join(out)


def nodes(fill_center="#FFFFFF", leaf=None):
    out = [f'<rect x="{CX-125}" y="{CY-82}" width="250" height="164" rx="82" fill="{fill_center}"/>']
    for lx, ly, col in LEAVES:
        out.append(f'<rect x="{lx-LW/2}" y="{ly-LH/2}" width="{LW}" height="{LH}" rx="{LH/2}" fill="{leaf or col}"/>')
    return "".join(out)


background = f'''
<defs>
  <linearGradient id="bg" x1="0" y1="0" x2="0.35" y2="1">
    <stop offset="0" stop-color="#5C8DFF"/>
    <stop offset="0.55" stop-color="#4361EE"/>
    <stop offset="1" stop-color="#3A2FBF"/>
  </linearGradient>
  <radialGradient id="glow" cx="0.3" cy="0.12" r="0.75">
    <stop offset="0" stop-color="#FFFFFF" stop-opacity="0.2"/>
    <stop offset="1" stop-color="#FFFFFF" stop-opacity="0"/>
  </radialGradient>
  <linearGradient id="edge" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#FFFFFF" stop-opacity="0.45"/>
    <stop offset="0.5" stop-color="#FFFFFF" stop-opacity="0.05"/>
    <stop offset="1" stop-color="#000000" stop-opacity="0.12"/>
  </linearGradient>
</defs>
<path d="{SHAPE}" fill="url(#bg)"/>
<path d="{SHAPE}" fill="url(#glow)"/>
<path d="{SHAPE}" fill="none" stroke="url(#edge)" stroke-width="3"/>
'''

center_mark = f'''
<defs><linearGradient id="dot" x1="0" y1="0" x2="1" y2="1">
  <stop offset="0" stop-color="#5C8DFF"/><stop offset="1" stop-color="#3A2FBF"/>
</linearGradient></defs>
<circle cx="{CX-48}" cy="{CY}" r="26" fill="url(#dot)"/>
<rect x="{CX-6}" y="{CY-13}" width="80" height="26" rx="13" fill="#C9D3F5"/>
'''

leaf_shine = "".join(
    f'<rect x="{lx-LW/2+22}" y="{ly-12}" width="{LW-60}" height="24" rx="12" fill="#FFFFFF" fill-opacity="0.55"/>'
    for lx, ly, _ in LEAVES)


def main(out):
    canvas = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    # The icon's own shadow, as in Apple's template.
    canvas.alpha_composite(shadow(f'<path d="{SHAPE}" fill="#000"/>', 14, 10, 0.32))
    canvas.alpha_composite(render(background))
    canvas.alpha_composite(shadow(branches("#000", 38) + nodes("#000", "#000"), 16, 12, 0.35))
    canvas.alpha_composite(render(branches("#FFFFFF", 38)))
    canvas.alpha_composite(render(nodes()))
    canvas.alpha_composite(render(center_mark + leaf_shine))
    canvas.save(out)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "resources/icon.png")
