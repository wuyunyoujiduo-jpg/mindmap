---
version: alpha
name: MyMind
description: A flat, exact desktop workspace after TickTick's manner — a quiet gray window, one white leaf cut into columns by thin lines, pale blocks and white cards, four inks, one violet-blue accent; no shadow but under what floats.

colors:
  ground: "#F4F4F6"       # the window (one of four, chosen in the settings)
  leaf: "#FFFFFF"         # the page, white cards
  block: "rgba(29,29,31,0.045)"  # a lone card, a field, a track of tabs (blockFill)
  hairline: "#E5E5EA"
  ink: "#1D1D1F"          # read first (Text)
  ink2: "#48484D"         # read second: a row not chosen (ink2)
  muted: "#86868B"        # words on things (TextMuted)
  faint: "#A9A9AF"        # counts, small names of parts (faint)
  primary: "#5B5BD6"
  on-primary: "#FFFFFF"
  danger: "#E5484D"
  success: "#30A46C"

typography:
  page-title:  { fontSize: 22px, fontWeight: 700, letterSpacing: -0.5px }
  write:       { fontSize: 16px, fontWeight: 400 }
  row:         { fontSize: 14.5px, fontWeight: 400 }   # 600 when chosen
  body:        { fontSize: 14px }
  part-name:   { fontSize: 13px, fontWeight: 600 }
  small:       { fontSize: 13px }
  caption:     { fontSize: 12px }
  side-head:   { fontSize: 12px, fontWeight: 600, letterSpacing: 0.4px }  # faint
  count:       { fontSize: 12px, features: tnum }                         # faint

rounded: { small: 7px, row: 8px, key: 9px, card: 12px }
---

# MyMind's design

The tokens are `design.go`'s. The form is that of the DESIGN.md files of
[awesome-design-md](https://github.com/VoltAgent/awesome-design-md);
what is in it is the app's own, and the user's corrections (kept at
length in `CLAUDE.md`) come before anything here.

## Colors

- **Four inks, no more.** Ink for what is read first; `ink2` for a row of
  the left column not chosen; muted for words on things; `faint` for
  counts and the small names of parts. Anything else is a link, a danger
  or on the accent.
- **The accent** is for what acts, what is chosen, links and the AI's
  spark. Small colors mean something: a map's dot, a kind's dot, a
  priority's circle.

## Elevation

Nothing has a shadow but what floats over the page (the bar open, bars
of the canvas, popups, modals), and what floats has a shadow and no line
round it. On the leaf, depth is by fill alone: white, the pale block,
the gray of what is chosen.

## Shapes

Corners are small and alike: 7, 8, 9, 12. Nothing is a pill.

## Components

- **Left column's row** (`sideRow`): 36 high, radius 8; `ink2` until
  chosen or pointed at, then ink; chosen, a gray block at 600. Its count
  in `faint`, in figures of one width.
- **Task's row** (`taskRow`): a thin line under it; chosen, a round gray
  block whose line goes.
- **Field** (`captureBox`, `area`, the day's field): a pale block with no
  line, white in the accent's line when written in.
- **Tabs** (`well`, `slidePill`): a pale track, a white pill that slides.
- **Scroll bars**: 4 wide and faint, darker under the pointer.

## Do

- Rank by weight and ink before size or color.
- Keep fields, keys and tracks to the one pale block.

## Don't

- Don't add a shadow, a gradient or a glow to anything on the leaf.
- Don't outline what floats.
- Don't give a field a darker fill or a line of its own.
- Don't add a fifth ink.
