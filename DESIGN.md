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
  teal: "#0E8F9C"         # the lens 来源's hue, and nothing else's (dark #4CC7D3)

typography:
  page-title:  { fontSize: 26px, fontWeight: 700, letterSpacing: -0.7px }
  write:       { fontSize: 16px, fontWeight: 400 }
  row:         { fontSize: 14.5px, fontWeight: 400 }   # 700 when chosen
  body:        { fontSize: 14px, fontWeight: 400 }
  part-name:   { fontSize: 13px, fontWeight: 700 }
  caption:     { fontSize: 12px, fontWeight: 400 }
  side-head:   { fontSize: 12px, fontWeight: 700, letterSpacing: 0.4px }
  count:       { fontSize: 11px, features: tnum }      # the left column's tablets (sideCount)

rounded: { small: 7px, row: 8px, key: 9px, card: 12px }   # on the leaf; what else has a corner: Corners, below

motion:   # transitions.dev's scale; design.go's motion* and easeDetent
  micro: 80ms
  quick: 150ms    # a color under the pointer, a press, what closes or goes
  fast: 250ms     # what opens from a key, tabs' pill, an icon's swap, a row to its place
  medium: 350ms   # a panel closing
  slow: 400ms     # a panel opening, an empty state coming, what was loading shown
  breath: 900ms   # a skeleton's breath, the one length off the scale (Motion, below)
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
| Negative tracking on headlines, never on small text | page titles at -0.7; small names of parts +0.4 |
| No shadow on cards, buttons or text | none on the leaf; only what floats over the page has one (two things drawn with the shadow's tool are not shadows: Lines, below) |
| No gradient as decoration | none in the UI (the icons' own shading is theirs) |
| A press scales the control down | `press`, on every key |
| Don't mix radii grammars | 7, 8, 9, 12 on the leaf and nothing between; what floats, pictures, tags and dots have their own (Corners, below) |

Not taken: full-bleed photo tiles, the black global nav, pill CTAs (the
user found small round corners right), 17px body (a desktop app's rows
are 14 to 16).

## Corners

On the leaf a corner is one of four: 7 a small thing's (`radiusSmall`),
8 a row's (`radiusRow`), 9 a key's, a tab's and a field's (`radiusPill`),
12 a card's and the leaf's own (`radiusCard`). What else has a corner is
of another kind, and each kind has its own, which the app has had for
many versions and the user has seen so:

- **A dot or a mark** is round: half its side (1 to 5 for a dot of 2 to
  10; 999 where the side is not known).
- **A small tag on a row** (a word on a tint, smaller than a key: a tag
  in a note, a link's site, a holiday's mark, a key's name in the cheat
  sheet): 6, or 4 where it is one line of the smallest type: a smaller
  thing than a key, a smaller corner.
- **A picture** shown whole in a page (a clip's, a video's, a slide's):
  10.
- **What floats** has a corner by its size, larger than anything on the
  leaf, which with its shadow tells it from the leaf: a popover and a
  toast 14, a job's card and the AI's window 16, a dialog 20 (`modal`).

A new thing takes the corner of its kind; a corner not listed here is a
fault.

## Lines

A hairline round a card is drawn as a shadow of half a point with no
blur and no offset (`paper`, `wallCard`): MyGo's border takes room and
its shadow does not, and it is a line, not a shadow. The white pill of a
track of tabs (`slidePill`) has that line and, under it, a shadow of one
point at 7%: it lies on its track, the one thing on the leaf that is
raised, and has been drawn so for many versions. Nothing else on the
leaf has either.

## Icons

- **Places and lists** (the top row's keys, the left column's rows, the
  directory's folders, empty states): [Kubai Icons](https://github.com/kubai087/kubai-icons)
  (MIT), `kubai.go`. Soft at rest; under the pointer or chosen, in the
  accent and in the pose the set gives each on hover, one fading into the
  other in `motion.fast`. Bitmaps drawn by `tools/kubai/gen.mjs` from
  `tools/kubai/names.txt`: add a name there and run it for a new one.
  Never draw a look-alike by hand. In the top row only the AI's spark
  has its color at rest; the other keys are the same icons in gray
  (`kubaiGray`) till the pointer is on them: four alike in the accent
  had no first among them.
- **Marks on keys and tools** (＋, ×, ⋯, bold, a chevron): thin drawn
  lines, one family (`icons.go`), in the ink. They are marks, not places,
  as on macOS.

## Hands

- **A way back**: what leaves where it was says so with 撤销 on the toast
  (`tellUndo`): a task ticked off, a task or a clip put on a map
  (`moveTaskUndoably`, `takeClipPlace`), a tag given, a clip deleted,
  tasks put on today.
- **Escape** puts away what opened: a task's details, a clip's, as the
  bar, the search and the canvas's. Pressed in a field of the pane, it
  first leaves the field (`escRest`), and the next puts the pane away.
  All the keys are in the cheat sheet (`shortcuts`), which a new one is
  added to.
- **The arrows** go through a list, its details following (`taskKeys`,
  the wall's in `clipWall`, where they go as they point: up and down a
  column, left and right to the card beside), and the space bar ticks a
  task off; never
  while something is written in (`FocusTakesText`).
- **The places** are ⌥⌘1 to ⌥⌘4 (今天, 项目, 收集, 技能) and the bar ⌘J,
  said on their tips and in the menu 前往 (⌘1–6 are a map's views and
  ⌘K the commands, older than these: 0.84.0 gave the places ⌘1–3 and
  the bar ⌘K, which the menu took first).
- **Words of no one device**: a tip, a placeholder and an empty state
  say what a thing does (记下, 只看这一天), never the key or the press
  that does it (回车记下, 双击, 右键): a phone is to come. The keys are
  the cheat sheet's (`shortcuts`) and the menus', which may name them;
  the places' tips name theirs by `keyLabel`, as the system writes them.
  What only the pointer's other key did has a ⋯ too (a tag's row in a
  map's panel, `tagRow`).
- **The canvas takes a topic anywhere**: a double click on the bare
  canvas makes a floating topic there, as its menu does (`canvasMenu`,
  which says the double click, and puts a clip of 收集 there as a topic
  that links to it); a topic let go on the bare canvas leaves the tree
  for that place (`floatDropped`), and a floating topic let go over a
  topic goes under it, that topic in a dashed line of the accent
  meanwhile (`joinFloating`); each with 撤销.
- **The left column takes what is dropped on it**: a task on a project
  goes to that map (`dropTaskOnMap`), a clip on a tag takes it
  (`tagClipUndoably`), as a map on a folder.

## Motion

Only the five lengths and the one curve above (from
[transitions.dev](https://transitions.dev)), each for what it names. No
other length in a file: `rg "time.Millisecond"` should find only
`design.go`'s scale and what is not an animation. Tabs slide (tabs
sliding), a toast rises and fades (MyGo's own), what opens from a key
grows from 0.96 (modal), a panel slides in (panel reveal), an icon swaps
by a fade (icon swap).

One length is off the scale: a skeleton breathes in 900 ms
(`skeletonShapes`). It is no transition, nothing going from a place to
another, but a pulse that goes on while something loads, slower than
anything that moves so that it reads as waiting; it has been so since
the skeletons were drawn, and no other length is to join it.

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

## A phone

A window narrower than `compactWidth` (640) is a phone's, and is laid
out so (`phone.go`, `narrowLayout`); a desktop's window never is, its
least width being 900. Its draft is
https://claude.ai/artifact/5ySnmjspG48dmg8MpwcwLt.

- **The page is the screen**: white to its edges, no leaf on a gray
  ground. Over a place's page, its row of keys (`phoneTop`): the left
  column's at the left; the search and the AI's spark at the right, and
  ＋ on 项目. The settings are at the drawer's foot.
- **The places at the foot** (`phoneTabs`): 今天, 项目, 收集, on the
  ground's gray under a hairline, each its Kubai icon (26) over its name
  at the caption's size; the one shown in the accent, at 700.
- **The bar at the foot** (`phoneBar`), over the places, where the thumb
  is: a pale block, ＋, what to write, and the tag of what it is kept as
  (任务 on 今天, else 随记). A tap grows it upward where it is into the
  card to write in, outlined in the accent, its tools and its 记为 under
  the words; the places go while it is open, the keyboard under it. Not
  a sheet over a dimmed page: the user found that not good to look at.
- **The left column slides in** over the page (`mobile.Drawer`, 288,
  white, under the system's bars by their insets), with how the sync
  stands and 设置 at its foot (`drawerFoot`), and goes when what it
  narrows to is chosen (`drawerSubject`).
- **What is chosen rises from the foot** (`sheet`, `mobile.Sheet` at
  92%, a topic's panel at 60% and 92%): a task, a skill, a branch being
  grown, a topic's details. A clip, which is read at length, covers the
  page under ‹ 收集 in the accent (`sheetBackTo`).
- **A window is a page** (`phonePage`): the settings, the search, the
  history, the templates, the cards to share, over the place's page
  under ‹ 返回. A few lines and keys stay a window (`smallModals`: the
  tour, an update, a merge), their keys one under another where a row
  ran past the edge (`tourKeysPhone`). A phone's words name no key of a
  keyboard and nothing a computer alone has (`phoneTourCards`, the
  palette's commands, the settings' parts).
- **The settings are a list** (`phoneSettings`, `settingsList`,
  `navRow`): the parts as rows in pale blocks, each its Kubai icon on the
  accent's tint, its name, how it stands in gray (自动, 未设置, a sync's
  time, the AI's model) and a chevron, the line under it from its name;
  a row opens the part as a page of its own under ‹ 设置, its name large
  at its head, the whole page scrolling. The version is at the list's
  foot. A phone's 连接 is the AI alone, so its row is 助手 and opens it.
  Six pills over the page with its name said again under them, and a
  block cut at its side, were the user's screenshot (0.108.6).
- **A sheet's focus is on itself**, not on its first field: MyGo moves
  the focus into a sheet as it rises, and a title taking it brought the
  keyboard up over what was to be read.
- **A clip is a page of its own** (`phoneCovered`): no row of keys, bar
  nor places round it, ‹ 收集 its one way back.
- **A page is 16 in** (`pagePad`); a head with something at its right has
  it under it (`pageHead`, 今天's head); a row of the settings has what
  sets it under its name where they do not fit side by side (`setRow`).
- **今天**: 列表 | 日程 for 列表 日 周 月; 日程 is a day's hours under a
  strip of its week (`phoneWeek`: weekday, date, the Chinese calendar's
  word; the day shown on the accent's tint, today in the accent, a day of
  rest on its ground). A task's row is 60 tall, its words at 16, and a
  finger swipes it (`mobile.SwipeRow`): right, 完成 on the success
  green; left, 明天 in the accent and 删除 in the danger, each with 撤销.
- **项目**: one column of the cards; ＋ in the top row with its menu.
- **收集**: the search the page's width, the kinds as chips under it
  (`phoneKinds`, the chosen one on the accent's tint), the wall in two
  columns 10 apart.
- **A map** has no top row: its bar has ‹ to 项目 at its start and the
  spark at its end. A mind map is drawn growing to the right from its
  center at the left (`drawnStructure`), the map itself unchanged. With
  a topic chosen, its keys float at the canvas's foot in a bar of their
  own (`phoneCanvasFoot`: 子主题, 同级, 改字, 任务, 详情, a finger's size
  each, the last in the accent); one round key over it goes back to the
  middle. The zoom is the fingers'. A finger pans the map, from its
  blank or across a topic, and glides on after a flick (`fingerPan`,
  `panGlide`); two pinch it (`InputZoom`); one held half a second on a
  topic takes it up, its ghost under the finger, the topic it would go
  under outlined, and let go there it goes under it with 撤销
  (`carryTopic`): a long press is that, so the topic's menu is the bar's
  ⋯ and the canvas has none. A map is placed with its center at the
  screen's left, a step smaller (to 0.8) where it is wider than the
  screen; its bar has no panel key (the topic's bar has 详情), and the
  canvases' row shows only for two or more (a new one is 视图's). The
  board's columns are a card's width each, swiped through
  (`boardColumn`). A topic's 更多 is a sheet of choices from the foot
  (`phoneTopicMore`, MyGo's `ActionSheet`): the desktop's menu, laid
  out by Android, ran every row of its submenus past the screen, and the
  user asked for less: six, what the bar has not (移到其他导图, 展开/折叠
  下级, 复制, 粘贴为子主题, AI 拆解成任务, 删除).
  Android's back goes from a map to the place it was opened from and
  from a clip to 收集 (`phoneBack`, by the back key MyGo tries after
  Escape); with nothing taking either it left the app. In the outline, a line written in has
  its keys over the keyboard (`outlineKeys`: out, in, up, down, a task,
  完成).

## 项目's cards

A map's card is a card apart as the wall's (`wallCard`), its face a
faint well at its top (`drawMapFace`), the map drawn as large as the
well takes (to 1.7): a map of three topics is not a thin line.

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

## 长成分支

Built to the second draft of 长成分支 (`grow.go`). 收集 has no new view
and no grouping but its tags; what is new is one key and one sheet.

- **The key** stands at the right of the page's head, before the search,
  only while a tag is chosen and it has two clips or more to grow from: a
  shade key (`key`), the one key of the head. While its sheet shows it is
  on the accent's tint, as what is chosen; the key that acts is the
  sheet's.
- **Once grown**, a quiet chip stands in the key's place, 已在「导图」: 12,
  the accent on its tint, the corner of a small thing (7); a click goes to
  the branch. The key comes back beside it, 再长一次, only when the tag has
  clips the branch does not link to.
- **The sheet** is the right column, where a clip's details are: its name
  at 20/700, from what in the caption's size; a part's name 13/700; where
  it goes a pale block with a key's corner, the map's dot, 「导图 › 主题」
  and 改 at its end, the whole of it the key. The outline is lines with a
  hairline under each: the branch's name heavy, a level 16 in, 「N 条来源」
  at the end in the muted ink, the accent's under the pointer and when
  open; the sources open under their line, a kind's dot before each. What
  the clips do not speak of is one line in a dashed box (the one dashed
  line of the leaf: it marks what is not there). Then the key that acts,
  放到导图, and 先不.
- **The wait** is stage lines in the outline's place (Waiting); what
  failed stays there in the danger's color with 重试.
- A toast has one key (撤销): the way to the branch, 去看看, is the
  sheet's, which stays to say where the branch went, and the chip's.

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

图解's comparison (对比, `diacompare.go`) is a table of type and hairlines,
no card: the options a head at 14/700 over a line of `faint`, what they are
compared by a first column 150 wide in muted type, a cell 13.5 with 11 by
14 round it, a hairline under each row. A cell under the pointer is a pale
block; the one chosen has the accent's line inside its edge (1.5, corner
7), no fill. What the map does not say is a dashed outline of `faint`
round 导图里没说, the one dashed thing on the page, for what is open and
not the AI's to fill. A row in which every option says the same is muted.
Covered, asked back, a cell is a pale block of its line's height. 我的倾向,
the user's own row, is cut off by a line of `faint`, its name heavy: the
option leant to has the accent's dot and 倾向这个 at 700, the others a
faint 点一下标出 until one is marked. More than four options are cards down
the page, white with a hairline (things of a kind), an option's name and
its one key, 标为倾向, at the head.
## A map's lenses

镜头 (`lens.go`) color one map another way, and move nothing: a topic's
box is where it is by its branches, so what a lens adds is never in the
topic (来源's count is a small tablet on its corner, `lensCount`).

- **The row** (`lensBar`) is words on the canvas at its lower left, not a
  card: no ground, no shadow. First its name, which is not a choice and
  does not look as one: a small mark (`iconLens`, a circle half filled)
  and 镜头 at 11/700, +0.6, muted; a hairline 14 high; then the choices
  at 12, muted: 分支 · 新旧 · 来源 · 进度, the one chosen in the ink, 700,
  on the pale block (radius 7), each as wide as its word in bold so that
  none moves when another is chosen. A white floating bar with the
  accent's tint was a second row of tabs under the map's, the user found,
  and the name set as the choices one more of them.
- **The legend** is at the lower right, as faint: swatches 14 by 10,
  radius 3, a hairline round each, words at 11 in the faint ink.
- **Clear of the canvas's bar**, which is at the foot on the right: the
  row stands beside it, in its middle, while both fit, and over it when
  they do not, by what the two measure; the legend over the bar, and over
  the row when the row is there (`TestLensRowKeepsClear`).
- **One hue by steps.** 分支 is the canvas as it is. The others fill
  every topic with one hue mixed into the leaf (in Oklab), words in the
  ink; the step that has none is white with a hairline, its words muted;
  the lines between topics are the hairline's gray, a topic's own marks
  (its links, its key to fold) gray too; the middle stays in the ink.
  新旧: the accent at 34, 20 and 9 of a hundred for this week, this
  month, three months (7, 30 and 90 days, fixed), white beyond. 来源: the
  teal at 30 and 13 for three clips or more, one or two, white for none.
  进度: done the success at 22, doing the accent at 14, something late
  the danger at 16, white for what is no task: the three colors that
  mean so everywhere, a step each. On the dark leaf each is 1.4 times as
  much, a tint reading as less there.
- **The teal** is this lens's alone. It is no second accent: nothing
  acts, is chosen or links in it.
- **The right column** with nothing chosen (the middle, not chosen by
  hand: it shows no ring then, and no topic's bar) says the lens
  (`lensPanel`): its name as a topic's (20/700), two figures (`figures`),
  a part's name, and lines to go to with a hairline under each, a word at
  the end of a line in the caption's size. A topic chosen has its page,
  with the lens's fact a caption under its name. The bare canvas clicked
  is nothing chosen again.
- A picture of the map and its slides are never through a lens.

## AI 技能's page

Built to the draft https://claude.ai/artifact/SUEZAk4SLoJ76BxsM9LXDE,
then made the place 技能 (0.99.2): its left column the parts (要处理, 30
天没用过), the scopes (全局, each project by its map's dot, 插件, 别的电脑)
and the tools, by their two letters, those with no skill folded under one
row; one row of it is chosen at a time, a scope or a tool (the two
chosen together, two chips over the list, read to the user as one kind
of condition said twice, 0.100.8), and said as a chip with a ✕ over the
list, as 收集's. The page is headed as the others, the one copy said in a quiet
line, then, in a pale block, one key in the accent for all that needs no
judgment (一键优化, what it does a line each under its name) and the way
to what does (去看看), then a table: a skill's name in the monospace (it is a
folder's), a tool's two letters full where it has the skill and in a
dashed outline where not, a dot of the accent where a copy took a
link's place, the state a pill with a dot (the danger's for a fault).
With nothing chosen the right column says what a session of a tool in a
project loads.

## 今天's page

Built to the draft https://claude.ai/artifact/5Nw4Mn4q4ozd9rpUNGh8AP.

- **Head**: the page's name at 26/700, -0.7; the day beside it in `ink2`;
  under it the day in three figures, the figure at 700 in the ink
  (`figures`), what it counts in the muted ink.
- **A group's name**: 12/700, +0.3, muted (the danger's color when late):
  the tasks are what is read. A row is 54 high, its line faint; a late
  day is a small pale tag of the danger's color.
- **A table when there is room** (`tableRoom`): the tasks' columns are
  named once over the list (`taskTableHead`: 任务, 项目 or 位置, 日期), a
  task's project in a column of its own with its dot, its day at the
  right. A priority is a small pill in its color, a state that is not
  plain 待做 a pill with a dot (`taskPills`). The list's foot says how
  many it holds, faint.
- **Pills on a row**: a priority has its color; a state is gray with a
  dot of its color: one color on a row, not three.
- **Under today's**: the tasks of the seven days after, as 接下来 7 天,
  so that the page is not half empty and tomorrow is seen coming.
- **A task's details**: how it repeats is three keys for the ways most
  used (不重复, 每天, 每周) and one that opens the others (`dropButton`),
  not six chips; 12 between its rows.
- **A toast** (`toastLook`): a white card at the window's foot, led by a
  small tile, a green tick for what was done, a red cross for what failed;
  its key in the accent.
- **The week and the day** are hours, as a calendar's (`weektime.go`,
  0.103, after TickTick's and Feishu's): the hours down the left, 48 to
  an hour, a faint line at each; a column a day cut by a faint line,
  headed by its weekday, its date at 16/700 and the Chinese calendar's
  day by it, today's in the accent and on a breath of the accent down
  its column (`todayFill`: the gray is the days of rest's). Over the hours a band of the tasks
  of the whole day, each a pale block (`taskChip`). A task with a time is
  a block on its hours, on its map's pastel laid whole on the white with
  a line of the map's color at its left, side by side with those it
  meets (`lanes`); one that comes again is the same, faint, after ↻. Now
  is a line of the danger's color across today, its time in the hours'
  column. What is being chosen or dragged to is a tint of the accent
  with its hours said in it (`hourShade`).
- **The month**: a cell a day cut by hairlines, the days of other months
  told by faint figures alone; a few of a day's tasks as pale blocks and
  how many more.
- **Days of rest** (`restDay`, `restFill`; the user's, after the draft's
  fourth page): two rules, the same in every calendar. The ground says
  whether a day is one of rest: a Saturday or Sunday not worked, or a day
  the holidays' notice gives off, is on `restFill` (the ink at 3%, 4% in
  the dark: the block's fill at two thirds) with its date and weekday in
  the muted ink; a weekend day worked is as any day worked, white and in
  the ink. And a mark is for what is not as the week has it: 休 (with the
  holiday's name under the pointer) on a day off, 班 on a weekend worked,
  small tags of the success's and the danger's colors; a weekend as any
  has none. In the week and the day the whole column is on the ground,
  head, band and hours; in the month the cell, a day of another month
  too, and Saturday's and Sunday's names over it are faint; in a list of
  days (最近 7 天, 今天 and 明天 of 全部) the day's heading is on it, with
  its tag; in the small calendar of a day to choose (`dateField`,
  `monthPick`, the app's own for this) the day's cell, the tag a small 休
  or 班 at its corner. Today keeps its accent over the ground. A year
  with no notice, and the holidays turned off, go by Saturday and Sunday.
  Gray in a calendar says rest and nothing else.
- **The AI's key by a heading** (`sparkKey`): the spark and its words in
  the accent, on no block until pointed at.

项目 in a wide window (`projRailRoom`) has a column at its right of what
goes on in the projects (`projectsRail`): four figures with no ground of their own and a line under them, those
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
| Long work that can be counted: the index, a run of clips, a download | a card at the window's foot on the right (`jobsView`, `jobCard`): what, how far, the time left, a bar; it stays on every page, can be stopped, and turns in place into its result, which stays until put away |
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
  Three things on the hours ask for 600 and are left so, as they have
  been seen since the hours were drawn (0.103): now's time in the hours'
  column and the hours said on a span being chosen, which are figures,
  where 600 shows, in the caption's size; and a block's title, which is
  therefore drawn regular where it is Chinese, light on its pastel
  (`weektime.go`). No fourth is to join them.
- Don't add a second accent, or a fifth ink.
- Don't write a length of time outside the scale (the breath alone is:
  Motion).
- Don't ship a list without its empty state.
