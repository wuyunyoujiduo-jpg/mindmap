"""Other concepts of MindMap's icon, on project knowledge and AI:

    spark   the AI's spark at the center of a map of knowledge
    cards   the library's cards, a map on the front one, and the spark
    orbit   knowledge in orbit around the spark, a constellation

    python3 tools/icon/concepts.py <concept> out.png

The app's icon, resources/icon.png, is now bold.py's, the bold take on
cards (also appicons/cards.png); appicons/ holds it and orbit at 512
pixels, the icons to choose from in the settings.
"""
import math
import sys

from PIL import Image

import make
from make import S, SHAPE, render, shadow


def sparkle(cx, cy, r, fill, k=0.13):
    """The four pointed star of AI, its sides curved in."""
    q = r * k
    return (f'<path d="M{cx},{cy-r} Q{cx+q},{cy-q} {cx+r},{cy} Q{cx+q},{cy+q} {cx},{cy+r} '
            f'Q{cx-q},{cy+q} {cx-r},{cy} Q{cx-q},{cy-q} {cx},{cy-r} Z" fill="{fill}"/>')


GRAD = '''
<linearGradient id="ai" x1="0" y1="0" x2="1" y2="1">
  <stop offset="0" stop-color="#4FC3FF"/>
  <stop offset="0.5" stop-color="#7B61FF"/>
  <stop offset="1" stop-color="#FF6AD5"/>
</linearGradient>'''


def base(stops, glow="#FFFFFF", glow_opacity=0.2, cx=0.3, cy=0.12):
    s = "".join(f'<stop offset="{o}" stop-color="{c}"/>' for o, c in stops)
    return f'''
<defs>
  <linearGradient id="bg" x1="0" y1="0" x2="0.35" y2="1">{s}</linearGradient>
  <radialGradient id="glow" cx="{cx}" cy="{cy}" r="0.75">
    <stop offset="0" stop-color="{glow}" stop-opacity="{glow_opacity}"/>
    <stop offset="1" stop-color="{glow}" stop-opacity="0"/>
  </radialGradient>
  <linearGradient id="edge" x1="0" y1="0" x2="0" y2="1">
    <stop offset="0" stop-color="#FFFFFF" stop-opacity="0.45"/>
    <stop offset="0.5" stop-color="#FFFFFF" stop-opacity="0.05"/>
    <stop offset="1" stop-color="#000000" stop-opacity="0.12"/>
  </linearGradient>
  {GRAD}
</defs>
<path d="{SHAPE}" fill="url(#bg)"/>
<path d="{SHAPE}" fill="url(#glow)"/>
<path d="{SHAPE}" fill="none" stroke="url(#edge)" stroke-width="3"/>'''


def layers(canvas, *parts):
    for kind, body in parts:
        if kind == "shadow":
            canvas.alpha_composite(shadow(body, 16, 12, 0.35))
        elif kind == "soft":
            canvas.alpha_composite(shadow(body, 40, 0, 0.9))
        else:
            canvas.alpha_composite(render(body))


def spark(canvas):
    """The map of the first icon, its center the AI's spark."""
    cx, cy = 330, 512
    leaves = [(700, 312, "#5EE2B0"), (732, 512, "#FFC94D"), (700, 712, "#FF7B95")]
    lw, lh = 164, 94
    br = "".join(f'<path d="M{cx+70},{cy} C{(cx+70+lx-lw/2)/2},{cy} {(cx+70+lx-lw/2)/2},{ly} {lx-lw/2+6},{ly}" '
                 f'fill="none" stroke="{{c}}" stroke-width="38" stroke-linecap="round"/>' for lx, ly, _ in leaves)
    nodes = "".join(f'<rect x="{lx-lw/2}" y="{ly-lh/2}" width="{lw}" height="{lh}" rx="{lh/2}" fill="{{c{i}}}"/>'
                    for i, (lx, ly, _) in enumerate(leaves))
    disc = f'<circle cx="{cx}" cy="{cy}" r="128" fill="{{c}}"/>'
    shine = "".join(f'<rect x="{lx-lw/2+22}" y="{ly-12}" width="{lw-60}" height="24" rx="12" fill="#FFFFFF" fill-opacity="0.55"/>'
                    for lx, ly, _ in leaves)
    black = {f"c{i}": "#000" for i in range(3)}
    layers(canvas,
           ("", base([(0, "#3E5BFF"), (0.6, "#3241D9"), (1, "#2A1F9E")])),
           ("shadow", br.format(c="#000") + nodes.format(**black) + disc.format(c="#000")),
           ("", br.format(c="#FFFFFF") + disc.format(c="#FFFFFF")),
           ("", f"<defs>{GRAD}</defs>" + nodes.format(**{f"c{i}": col for i, (_, _, col) in enumerate(leaves)}) + shine),
           ("", f"<defs>{GRAD}</defs>" + sparkle(cx, cy, 92, "url(#ai)")))


def cards(canvas):
    """The library: three cards fanned, a map on the front one, the spark
    at its corner."""
    def card(x, y, w, h, rot, fill):
        return (f'<g transform="rotate({rot} {x+w/2} {y+h/2})">'
                f'<rect x="{x}" y="{y}" width="{w}" height="{h}" rx="56" fill="{fill}"/></g>')
    back = card(250, 250, 470, 560, -14, "{a}") + card(270, 240, 470, 560, -6, "{b}")
    front = card(280, 250, 470, 560, 3, "{c}")
    # a map on the front card, turned with it
    lines = ('<g transform="rotate(3 515 530)">'
             '<circle cx="400" cy="530" r="34" fill="#4361EE"/>'
             '<path d="M434,530 C500,530 500,420 560,420 M434,530 L560,530 M434,530 C500,530 500,640 560,640" '
             'fill="none" stroke="#B9C3E6" stroke-width="22" stroke-linecap="round"/>'
             '<rect x="560" y="398" width="120" height="44" rx="22" fill="#5EE2B0"/>'
             '<rect x="560" y="508" width="120" height="44" rx="22" fill="#FFC94D"/>'
             '<rect x="560" y="618" width="120" height="44" rx="22" fill="#FF7B95"/>'
             '<rect x="340" y="330" width="200" height="26" rx="13" fill="#DCE2F5"/>'
             '<rect x="340" y="720" width="260" height="22" rx="11" fill="#E7EBF7"/></g>')
    star = sparkle(735, 290, 150, "url(#ai)") + sparkle(820, 420, 48, "url(#ai)")
    layers(canvas,
           ("", base([(0, "#F4F6FF"), (1, "#DCE3FA")], glow="#FFFFFF", glow_opacity=0.8)),
           ("shadow", back.format(a="#000", b="#000") + front.format(c="#000")),
           ("", back.format(a="#8EA6FF", b="#B6C5FF")),
           ("shadow", front.format(c="#000")),
           ("", front.format(c="#FFFFFF") + lines),
           ("soft", f"<defs>{GRAD}</defs>" + sparkle(735, 290, 120, "#7B61FF")),
           ("", f"<defs>{GRAD}</defs>" + star))


def orbit(canvas):
    """Knowledge around the spark: notes, links, pictures and tasks on
    two orbits, joined to it and to each other."""
    cx, cy = 512, 512
    pts = []
    for i, (r, deg, col, size) in enumerate([
            (300, -60, "#5EE2B0", 74), (300, 20, "#FFC94D", 66), (300, 105, "#FF7B95", 74),
            (300, 190, "#4FC3FF", 66), (300, 255, "#B38CFF", 58), (190, 150, "#FFFFFF", 40),
            (190, -15, "#FFFFFF", 36)]):
        a = math.radians(deg)
        pts.append((cx + r * math.cos(a), cy + r * math.sin(a), col, size))
    rings = (f'<circle cx="{cx}" cy="{cy}" r="300" fill="none" stroke="#FFFFFF" stroke-opacity="0.14" stroke-width="4"/>'
             f'<circle cx="{cx}" cy="{cy}" r="190" fill="none" stroke="#FFFFFF" stroke-opacity="0.1" stroke-width="4"/>')
    links = "".join(f'<line x1="{cx}" y1="{cy}" x2="{x:.1f}" y2="{y:.1f}" stroke="#FFFFFF" stroke-opacity="0.35" stroke-width="10" stroke-linecap="round"/>'
                    for x, y, _, _ in pts[:5])
    links += "".join(f'<line x1="{pts[i][0]:.1f}" y1="{pts[i][1]:.1f}" x2="{pts[j][0]:.1f}" y2="{pts[j][1]:.1f}" '
                     f'stroke="#FFFFFF" stroke-opacity="0.22" stroke-width="7" stroke-linecap="round"/>'
                     for i, j in [(0, 6), (6, 1), (2, 5), (5, 3)])
    dots = "".join(f'<rect x="{x-s/2:.1f}" y="{y-s/2:.1f}" width="{s}" height="{s}" rx="{s*0.32:.1f}" fill="{c}"/>'
                   for x, y, c, s in pts)
    layers(canvas,
           ("", base([(0, "#1C2470"), (0.55, "#151A55"), (1, "#0D0F33")], glow="#7B61FF", glow_opacity=0.55, cx=0.5, cy=0.5)),
           ("", rings + links),
           ("shadow", dots.replace('fill="#', 'data-x="#').replace("data-x", "fill")),
           ("", dots),
           ("soft", sparkle(cx, cy, 150, "#8F7BFF")),
           ("", f"<defs>{GRAD}</defs>" + sparkle(cx, cy, 165, "url(#ai)")),
           ("", sparkle(cx, cy, 70, "#FFFFFF").replace('fill="#FFFFFF"', 'fill="#FFFFFF" fill-opacity="0.85"')))


def draw(name, out):
    canvas = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    canvas.alpha_composite(shadow(f'<path d="{SHAPE}" fill="#000"/>', 14, 10, 0.32))
    {"spark": spark, "cards": cards, "orbit": orbit}[name](canvas)
    canvas.save(out)


if __name__ == "__main__":
    draw(sys.argv[1], sys.argv[2])
