"""What MyMind's icons are drawn with, in the shape and grid of macOS
icons: a rounded square of continuous corners, 824 points in a canvas of
1024 (SHAPE), an SVG rendered to it (render), and the shadow of a shape
(shadow). The icons are the other scripts': the app's,
resources/icon.png, is desk.py's. The first icon, a small mind map, was
drawn here, and over resources/icon.png when run with no name: it is in
the repository's history.
"""
import io
import math

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
