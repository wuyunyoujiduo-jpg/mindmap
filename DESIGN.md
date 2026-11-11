---
version: alpha
name: MyMind
description: A quiet desktop workspace — a warm pale ground, flat white cards told by a hairline and a breath of shadow, four inks, one violet-blue accent kept for the key that acts, the focus and links.

colors:
  ground: "#F6F5F4"        # the window, the sidebar, pages of lists
  ground-tray: "#EFEDEA"   # a column of a board, a track of tabs
  canvas: "#FFFFFF"        # cards, fields, keys, the map's page
  hairline: "#E5E3DF"      # lines between parts
  card-edge: "rgba(30,26,16,0.10)"  # a card's ring, inside its edge
  ink: "#24231F"           # what is read first
  slate: "#5D5B54"         # read second: a part's heading, a row not chosen (ink2)
  steel: "#787671"         # captions, a word on something (TextMuted)
  stone: "#A4A097"         # counts, hints, placeholders (faint)
  primary: "#5B5BD6"
  primary-hover: "#4D4DC4"
  primary-pressed: "#4040A8"
  on-primary: "#FFFFFF"
  danger: "#D2452D"
  success: "#2E9E6B"
  map-colors: ["#E8684A", "#E8A13A", "#3DAA7D", "#3B8FE0", "#8B6FE0", "#D95C8C"]
  dark-ground: "#1A1A19"
  dark-canvas: "#252523"
  dark-ink: "#E9E8E4"

typography:
  page-title:   { fontSize: 20px, fontWeight: 600, letterSpacing: -0.3px }
  number:       { fontSize: 22px, fontWeight: 600, letterSpacing: -0.5px, features: tnum }
  write:        { fontSize: 16px, fontWeight: 400, lineHeight: 1.5 }
  body:         { fontSize: 14px, fontWeight: 400 }
  body-strong:  { fontSize: 14px, fontWeight: 600 }   # what is chosen
  small:        { fontSize: 13px, fontWeight: 400 }
  part-heading: { fontSize: 13px, fontWeight: 600 }   # slate
  key-label:    { fontSize: 13px, fontWeight: 500 }
  caption:      { fontSize: 12px, fontWeight: 400 }
  eyebrow:      { fontSize: 12px, fontWeight: 600, letterSpacing: 0.3px }  # stone

rounded: { chip: 5px, key: 8px, card: 12px, pill: 9999px }

spacing: { unit: 4px, row: 32px, key: 30px, page-gutter: 28px, page-max: 1080px }
---

# MyMind's design

The tokens above are `design.go`'s (`quietTheme`, `ink2`, `faint`,
`radiusChip`/`radiusKey`/`radiusCard`, `ring`, `lift`, `focusRing`). The
form is that of the DESIGN.md files of
[awesome-design-md](https://github.com/VoltAgent/awesome-design-md); the
rules were taken from what Notion's, Linear's and Cal.com's have in
common, on flomo's restraint, the user's reference.

## Overview

A window of a warm pale gray, the sidebar on it with no line between;
on it, white cards. Nothing is told by a fill of color: a card by a
hairline ring and a breath of shadow, what is chosen by a shade of gray
and a heavier weight, the one key that acts by the accent. Text is of one
family, the system's, in four inks; headings are heavy and a little
tight, everything else regular.

## Colors

- **Ground** is the window. Four are offered in the settings (浅灰, 淡紫,
  淡蓝, 米色); cards are white on all of them.
- **Four inks**, no more: ink (read first), slate (read second), steel
  (captions), stone (counts and hints). A text that is none of these is a
  link, a danger or on the accent.
- **The accent** is for: the key that acts (`inkButton`, `accentButton`,
  the key that sends), the ring of the field written in (`focusRing`),
  links, the AI's spark, the ring of the card chosen, today in a calendar.
  Never a fill behind text that is read, never a row of the sidebar.
- **Color that means something** is small: a map's dot, a kind's dot, a
  priority's circle, a tag on a pale wash of the accent (`softTag`).

## Elevation

| Level | Treatment | Use |
|---|---|---|
| 0 | none | the ground, the sidebar, rows |
| 1 | inset ring `card-edge` + `0 1px 2px rgba(15,15,15,.045)` | cards, keys, fields (`raise`, `key`) |
| 2 | level 1 + `0 4px 12px rgba(15,15,15,.08)` | a card under the pointer that opens (`liftHover`) |
| focus | inset 1px accent + 3px halo of the accent at 16% | the field written in |
| float | hairline border + `0 4px 16px rgba(0,0,0,.10)` | bars and menus over the canvas |

Dark: no shadows; the ring is white at 8%.

## Shapes

Chips 5, keys and fields 8, cards 12; full round only for dots, a task's
circle and small counts. Keys are rectangles, not pills.

## Components

- **Sidebar row**: 32 high, radius 8, icon 18 and 14px text in slate;
  chosen, a shade (`chosen`) with ink at 600. Counts in stone at its end.
- **Card** (`raise`, `listCard`): white, radius 12, level 1; rows inside
  (`listCardRow`) are 46 high, radius 8, a shade under the pointer.
- **Part heading** (`trayHead`): 13/600 slate, a word after it in stone,
  what it does at its end.
- **Key** (`key`, `softButton`, `dropButton`): white, level 1, 30 high;
  under the pointer its face darkens 4.5%, pressed 9% and it gives 3%.
- **Primary key** (`inkButton`): the accent whole, label 13/500 white;
  disabled, the accent at 32%.
- **Tabs** (`pillTab`): the one chosen is a small card; the others slate.
  A choice among a few (`segmented`, `tabGroup`) is a gray track with a
  card for the one chosen.
- **Write card** (`writeCard`): a card with 16px text, tools as plain
  glyphs under it, the key that sends at its end; focused, the focus ring.

## Do

- Reach for weight and ink before color or size to tell things apart.
- Keep one primary key in sight per part of a page.
- Put what can be done with a row under the pointer, not always in sight.
- Use `ring`/`lift` for depth; they take no room, so nothing moves.

## Don't

- Don't fill a row, a tab or a card with the accent.
- Don't add a fifth ink, or a gray that is not on the ladder.
- Don't outline with `Border` what changes state: it takes room, and what
  is under the pointer would move.
- Don't use shadows heavier than level 2 on a page; `float` is for what
  is over the canvas.
- Don't round a card more than 12.
