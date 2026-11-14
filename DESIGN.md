---
version: alpha
name: MyMind
description: A desktop workspace in Apple's manner (DESIGN.apple.md) — near-invisible chrome, one accent, type that ranks by weight and tracking, no shadow on the page — with Kubai's icons for the places gone to and transitions.dev's scale for what moves.

colors:
  ground: "#F4F4F6"       # the window (Apple's parchment #f5f5f7, a shade cooler)
  leaf: "#FFFFFF"
  block: "rgba(29,29,31,0.045)"
  hairline: "#E5E5EA"
  ink: "#1D1D1F"          # Apple's ink
  ink2: "#48484D"
  muted: "#86868B"        # Apple's secondary gray
  faint: "#A9A9AF"
  primary: "#5B5BD6"      # the one accent: Apple's is blue, MyMind's its own violet-blue
  danger: "#E5484D"
  success: "#30A46C"

typography:
  page-title:  { fontSize: 22px, fontWeight: 700, letterSpacing: -0.5px }
  write:       { fontSize: 16px, fontWeight: 400 }
  row:         { fontSize: 14.5px, fontWeight: 400 }   # 700 when chosen
  body:        { fontSize: 14px, fontWeight: 400 }
  part-name:   { fontSize: 13px, fontWeight: 700 }
  caption:     { fontSize: 12px, fontWeight: 400 }
  side-head:   { fontSize: 12px, fontWeight: 700, letterSpacing: 0.4px }
  count:       { fontSize: 12px, features: tnum }

rounded: { small: 7px, row: 8px, key: 9px, card: 12px }

motion:   # transitions.dev's scale; design.go's motion* and easeDetent
  micro: 80ms
  quick: 150ms    # a color under the pointer, a press, what closes or goes
  fast: 250ms     # what opens from a key, tabs' pill, an icon's swap, a row to its place
  medium: 350ms   # a panel closing
  slow: 400ms     # a panel opening, an empty state coming, what was loading shown
  ease: "cubic-bezier(0.22, 1, 0.36, 1)"
---

# MyMind's design

**Follow `DESIGN.apple.md`.** It is Apple's DESIGN.md from
[awesome-design-md](https://github.com/VoltAgent/awesome-design-md), kept
whole beside this file. This file says what of it a desktop app takes,
what it cannot (Apple's is a site of product photographs), and the three
other sources: the icons, the motion, the states. The user's corrections
(at length in `CLAUDE.md`) come before anything here. `design.go` is
these tokens.

## What is Apple's

| Apple's rule | Here |
|---|---|
| UI recedes; no decorative chrome | one white leaf on a quiet ground, columns cut by hairlines |
| A single accent for everything that acts | the violet-blue: keys that act, what is chosen, links, the focus |
| Ink `#1d1d1f`, gray `#86868b`, parchment `#f5f5f7` | the same ink and gray; the ground a shade cooler |
| Weights 400 and 600; 500 absent; headlines 600, not 700 | two weights, 400 and 700: asked for 600, Chinese is drawn regular; at 700 PingFang draws its Semibold, which is Apple's 600 |
| Negative tracking on headlines, never on small text | page titles at -0.5; small names of parts +0.4 |
| No shadow on cards, buttons or text | none on the leaf; only what floats over the page has one |
| No gradient as decoration | none in the UI (the icons' own shading is theirs) |
| A press scales the control down | `press`, on every key |
| Don't mix radii grammars | 7, 8, 9, 12 and nothing between |

Not taken: full-bleed photo tiles, the black global nav, pill CTAs (the
user found small round corners right), 17px body (a desktop app's rows
are 14 to 16).

## Icons

- **Places and lists** (the top row's keys, the left column's rows, the
  directory's folders, empty states): [Kubai Icons](https://github.com/kubai087/kubai-icons)
  (MIT), `kubai.go`. Soft at rest; under the pointer or chosen, in the
  accent and in the pose the set gives each on hover, one fading into the
  other in `motion.fast`. Bitmaps drawn by `tools/kubai/gen.mjs` from
  `tools/kubai/names.txt`: add a name there and run it for a new one.
  Never draw a look-alike by hand.
- **Marks on keys and tools** (＋, ×, ⋯, bold, a chevron): thin drawn
  lines, one family (`icons.go`), in the ink. They are marks, not places,
  as on macOS.

## Motion

Only the five lengths and the one curve above (from
[transitions.dev](https://transitions.dev)), each for what it names. No
other length in a file: `rg "time.Millisecond"` should find only
`design.go`'s scale and what is not an animation. Tabs slide (tabs
sliding), a toast rises and fades (MyGo's own), what opens from a key
grows from 0.96 (modal), a panel slides in (panel reveal), an icon swaps
by a fade (icon swap).

## States

Every list and page has all of these, or does not need one:

- **Empty** (`emptyState`): an icon, a line, what to do, and the key that
  does it when there is one. 今天's list, 收集's wall (nothing yet,
  nothing found, nothing to look back at), 项目.
- **Loading** (`skeleton`): pale lines that breathe where the words will
  be, while a page is read or the AI writes. Never a bare spinner.
- **Said at the field** (`setRowSays`): what is wrong with what is
  written is said under the field's name as it is written (the sync's
  repository and token), not after it fails.
- **Told** (`tell`, `toasts`): what was done is said in a toast, on
  whatever page.
- **Taken back** (`tellUndo`): what is deleted goes at once and the toast
  offers 撤销; only what a topic links to is asked about first.
- Everything is on this computer: what is clicked is done before it is
  drawn again, so nothing waits on a server to show.

## A page

- **It starts at the left column's line** (`pageColumn`), 28 in, on every
  page, and is no wider than it reads well: the room left is on the
  right, where what is chosen opens. Never centered: a page's name must
  not move from a place to another, nor when a task is opened.
- **Its head** (`pageHead`): the name at 26/700, -0.7; a word beside it in
  `ink2`; under them the page in figures (`figures`), the figure at 700
  in the ink, what it counts muted. 今天, 项目 and 收集 are headed so.

## 收集's wall

Cards of their own height in three columns (`clipWall`), each put in the
column that is shortest (`clipTall` guesses a card's height), the newest
at the top: never rows of one height, which left a hole under every
short note. Each card stands apart on the white page, white with a
hairline that darkens under the pointer (`wallCard`), 12 between them:
the user found one gray ground under them all a slab. 120 at first, more
asked for at its foot.

What is done from any page (a screenshot, the AI, the search) has its
key in the top row, not on one page's head.

## What opens a menu

Ends in `chevron`: a small angle drawn as a thin line, the same wherever
it is. Never a letter (⌄, ▾): it sits low and is of another weight on
each font. A key that opens a choice is a key as the others beside it
(`key`, `dropButton`), never bare gray words.

A map's face on its card (`drawMapFace`) is the map itself, small: its
first two levels laid out by the canvas's own layout for its structure.

## The settings

Rows in pale blocks (`rowsField`, `setRow`): a group's name small, heavy
and muted, as a list's groups; a row's name regular, its word under it
muted; only the page's and the part's names are heavy. Its list of parts
is rows with icons, as the left column's. A shortcut's box is a pale
block, white in the accent's line while it records. What copies a block
of text is a small key at the end of the block's name, never a bar
across the page.

## A map's page

The same rules where they fit a tool's page: the map's name and a topic's
name in the panel are heavy and a little together, the outline's first
line is a page's name (26/700, -0.7); what is below a topic is told in
figures (`nodeFigures`); the field for a new tag is a pale block as every
field; a column of the board says its count faint. The board's columns
stay pale grounds under white cards (things of a kind, the user's rule).

## 今天's page

Built to the draft https://claude.ai/artifact/5Nw4Mn4q4ozd9rpUNGh8AP.

- **Head**: the page's name at 26/700, -0.7; the day beside it in `ink2`;
  under it the day in three figures, the figure at 600 in the ink, what
  it counts in the muted ink.
- **A group's name**: 12/700, +0.3, muted (the danger's color when late):
  the tasks are what is read. A row is 54 high, its line faint; a late
  day is a small pale tag of the danger's color.
- **A table when there is room** (`tableRoom`): the tasks' columns are
  named once over the list (`taskTableHead`: 任务, 项目 or 位置, 日期), a
  task's project in a column of its own with its dot, its day at the
  right. A priority is a small pill in its color, a state that is not
  plain 待做 a pill with a dot (`taskPills`). The list's foot says how
  many it holds, faint.
- **A toast** (`toastLook`): a white card at the window's foot, led by a
  small tile, a green tick for what was done, a red cross for what failed;
  its key in the accent.
- **The week**: a column a day cut by a faint line, not a block each;
  today's on a breath of the accent, its day in a dot of it, named 今天;
  the days of rest faint. A task there is a pale block.
- **The AI's key by a heading** (`sparkKey`): the spark and its words in
  the accent, on no block until pointed at.

项目 in a wide window (`projRailRoom`) has a column at its right of what
goes on in the projects (`projectsRail`): four figures as tiles, those
worked on lately with how far each is, the tasks that come next. 收集's
wall has as many columns as the room has (`clipWall`), a card never
stretched. Both after the dashboards the user showed (stat tiles, recent
projects with their progress, upcoming tasks).

A page uses the room a wide window has by standing a second thing
beside the first, not by stretching one column across it nor by leaving
half the window empty: with `railRoom` or more, what waits is a column at
the tasks' right (`waitingRail`), the page's height; with less, or with a
task's details open, it is the band under them.

Built to the draft https://claude.ai/artifact/V87PRBUjrJQQk1Kb3sj4Pw (A):
what waits for a place is a band under today's tasks, the page's width,
a line above it and a breath of shade; in it a line each, four at most,
its words, where it would go, when. All that can be done with one is on
the list 等着归位, a click away. Two kinds of thing on a page are cut
apart so, not stacked as cards of one weight.

## Waiting

`wait.go`. Which sign a wait takes is by how long it is and whether it
can be counted, never by habit:

| The wait | Its sign |
|---|---|
| What is of this computer: a task ticked, a day changed, a clip deleted | none. It is done as it is clicked; the result is the feedback. Never a "saving" that plays a wait there is not |
| One action, a few seconds, length unknown: a sync, a check, a backup | on its key (`busyKey`): its words and width stay, a spinner over them, a second press does nothing; then a toast says what came of it |
| Long work that can be counted: the index, a run of clips, a download | a card at the window's top right (`jobsView`, `jobCard`): what, how far, the time left, a bar; it stays on every page, can be stopped, and turns in place into its result, which stays until put away |
| Work that cannot be counted: the AI finding places, reading a map | its steps (`stageLines`): those done with a check, the one going on with a spinner; after ten seconds, that it takes longer than usual |
| What the AI writes of a clip | shapes where it will be (`skeletonShapes`), in its own part, at the size of what comes; what was there before stays while the new is written |
| The AI's answer | what it read, then its words as they come with a caret after the last; at its end 复制 and 重试 |
| What need not be seen: saving, syncing by itself | nothing, or a quiet word in a fixed place |

- Nothing that shows comes before 300ms, nor goes before 500ms more
  (`waitShown`: every sign goes through it), so that nothing flickers.
- One sign for a wait: not a spinner on the key and another on the page.
- Nothing moves when what was waited for comes.
- What failed stays said where it happened, in the danger's color, with
  重试: a toast that goes is not enough for what must be mended.
- A toast is at the window's foot (the top has the bar, which opens).

## Don't

- Don't add a shadow, a gradient or a glow to anything on the leaf.
- Don't outline what floats.
- Don't use weight 500 or 600: Chinese shows neither. Heavy is 700.
- Don't add a second accent, or a fifth ink.
- Don't write a length of time outside the scale.
- Don't ship a list without its empty state.
