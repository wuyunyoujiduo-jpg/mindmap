# MyMind

A macOS-first desktop app, in Go on [MyGo](https://github.com/egoist/mygo)
(native UI): mind maps for managing projects, a library of clips organized
by AI, and an AI assistant.

## Every change

1. **Check MyGo first.** Run `tools/check-mygo.sh`; when a newer version is
   out, upgrade with `tools/check-mygo.sh -u`, read what it adds (its
   `docs/` in the module cache, `go doc github.com/egoist/mygo`) and use it
   where it helps. Note the upgrade in the commit message.
2. Make the change, matching the code around it.
3. Check: `gofmt -l .`, `go vet ./...`, `go test ./...`,
   `GOOS=darwin GOARCH=arm64 go build -o /dev/null .`,
   `GOOS=windows go build -o /dev/null .`, `go tool mygo vet`.
   UI snapshots: `MINDMAP_SNAPSHOTS=<dir> go test .`, then look at them.
4. Work on `main` (the user's choice: a branch left behind once shipped
   a version from an old base over a newer one). Before anything, fetch
   and start from `origin/main`, and check that `mygo.json`'s version is
   the one the update manifest serves. Commit, push to `main`, bump `version` in `mygo.json` (the
   third number for fixes and small changes, 0.25.0 → 0.25.1; the second
   only for a new feature of some size, 0.25.x → 0.26.0), add
   its section to `CHANGELOG.md` (in Chinese: the update window shows it),
   and release by running the `build` workflow with `release: true`.

## Notes

- The UI's look is in `design.go` (sizes, helpers, the theme of
  `quietTheme`) and `icons.go` (icons drawn, not glyphs, so that they sit
  centered); `DESIGN.md` says it token by token and names its sources,
  to be read before the look is changed and changed with it: the style
  is Apple's (`DESIGN.apple.md`, from awesome-design-md), the icons of
  places and lists are Kubai Icons (`kubai.go`, drawn by
  `tools/kubai/gen.mjs`), what moves takes transitions.dev's lengths
  (`motion*` in `design.go`, and no other), and every list has its empty
  state (`emptyState`), what loads its `skeleton`, what was done its
  toast (`tell`; `tellUndo` where it can be taken back), and every wait
  its sign by `DESIGN.md`'s table (`wait.go`: `busyKey` on the key
  pressed, `jobsView` cards at the top right for work that can be
  counted, `stageLines` for work that cannot, all through `waitShown`). It is flat and exact. The user's references, in turn, were
  flomo, https://library.aimran.es/ and TickTick: TickTick's is the one
  for the style (and the user found shadows, gradients and glows wanting,
  two versions of them); the structure is the app's own, thought out from
  its functions, borrowing where a borrowing serves them and never copied.
  - The shell (`view.go`): the window is a quiet gray (`ground`, one of
    `grounds` chosen in the settings, `config.Ground`). One row at its top,
    on every page, a map's too (`topBar` in `places.go`): the left column's
    key, the switch of the three places, 今天, 项目 and 收集, the one bar,
    the AI's spark, the search of everything, the settings. Under it one
    white leaf with round corners (`leaf`), everything else on it, cut
    into columns by thin lines: what of the place shows, the page, what is
    chosen.
  - The left column (`side.go`, `sidePane`, `sideRow`; put away by
    `config.HideTree`) is the one way to narrow a page: 今天's lists, what
    waits for a place, and the maps to see the tasks of alone
    (`todaySide`); 项目's directory (`tree.go`, `mapTree`: folders that
    open and close and their maps, so that going from a map to another is
    one click); 收集's kinds, ways to look back and every tag (`libSide`).
  - The one bar (`bar.go`, `oneBar`) is in the top row: one line until
    written in, then the card to write in with its tools (`write.go`,
    `writeCard`: a tag and a reference offered as they are written, a
    picture, bold, lists; a reference is kept as `<clip:id>`, `noteText`),
    floating forward of the page on a deep shadow (`floated`), so that
    nothing moves. A tag on it says what the words are kept as, a thought
    or a task, and a click changes it (`barTask`: on 今天 a plain line is
    a task). It keeps a thought, a task
    (and how it repeats), a link; shows what there is already as it is
    written; asks the AI after a ?.
  - What is chosen (a task, a clip) is in a column at the leaf's right
    (`sheet`); a map's topic has its panel there.
  - The AI floats (`aiFloat` in `view.go`): a window at the foot of the
    window on the right, called from the spark of the top row, over
    whatever page shows and taking no room from it; the user asked whether
    a floating window suited it, and it does, the leaf having three
    columns already.
  - On the leaf a lone card is a pale block (`raise`, `blockFill`),
    things of a kind are cards apart, white with a hairline (`wallCard`), a
    card on a block is white with a hairline (`paper`), a list is rows with a thin
    line under each (`listCardRow`, the tasks' rows), a field is a pale
    block that turns white in the accent's line when written in
    (`captureBox`, `area`, `focusHalo`). Nothing has a shadow but what
    floats over the page (`lifted`: the canvas's bars, popups, `modal`,
    the bar open). Corners are small and alike (`radiusPill` is 9, not a
    pill). The key that acts is the accent, flat (`jewel`, `inkButton`,
    `accentButton`); other keys a shade (`key`, `textKey`); a track of tabs
    a pale block (`well`) with a white pill that slides to what is chosen
    (`slidePill`, a `Match` transition, behind the others' words). Type
    does the ranking: a page's name large and bold (`pageHeader`), a
    part's name small and bold (`trayHead`), words on things small and
    gray. The settings are rows in pale blocks, a name and a word on it at
    the left, what sets it at the right (`rowsField`, `setRow`, `setLine`,
    `switchRow`; `field` for what is not rows); the system's buttons take
    the app's corners (`button`).
  - 今天 (`taskspage.go`, `tasksView`): as a list it is one column read
    down (`todayWidth`), headed by the list's name and the line to add a
    task (`tasksAdd`); under today's list, cut from it by a band the
    page's width, what waits for a place as a few lines (`waitingBand`,
    `waitingLine`: the user found the cards there crowding the tasks, and
    chose lines; in a wide window it is a column beside the tasks,
    `waitingRail`, for half the window stood empty); its cards are the left column's list 等着归位
    (`homeWaiting`), each card saying a place by its words alone, with
    no AI asked (`nearPlace`, which must share a word with it,
    `sharesWord`); AI 归位 for better; what waits is cards that stand apart, white
    with a hairline (`wallCard`: pale blocks on white were ugly, the user
    found, and then one gray ground under white cards a slab); a card is clean, a click opens it, and
    what can be done with it is one menu, under the key at its corner (a
    phone has no other) and under the pointer's other key (`waitingMenu`;
    a row of keys at each card's foot was too much, the user found).
    A thought and a task stay each what it is, a clip and a topic, and
    one is made of the other by a link (`convert.go`: `clipToTask`,
    `taskToMemo`; a task's details read the thoughts it links to,
    `taskThoughts`). The week and the month show tasks
    on their days, and those that repeat (`model.Repeat`, `repeatProp`,
    `parseRepeat`) faint on the days they come again (`taskGhost`). A
    task's state, priority and day are set with `segmented` and `dateProp`.
    A project chosen in the left column shows all of its tasks not done,
    by when (`tasksOfMap`), with the way to its map, and nothing that
    waits: that is of no one project.
  - 项目 (`projectsView`): the maps as cards, each with the map drawn
    small as its face (`drawMapFace`) and a ring for its tasks, white
    with a hairline as every card of a kind (`wallCard`); with the
    left column put away, the folders are a row of pills (`projFolder`).
    A map opened has its own bar under the top row; on the canvas the
    center is in the ink, the main topics on a pastel of their branch's
    color, those below on a faint tint (the scheme 明亮 of `style.go`).
  - 收集 (`libview.go`): notes and clips, a wall of cards that
    stand apart on the white page, each white with a hairline
    (`wallCard`; the user found one gray ground under them a slab); its 回顾 shows a
    few old notes in place (`memos.go`). What is collected is 收集's (the
    word of the UI, never 资料库), and is on a map by a topic's link.
  - Color is kept for small things that mean something: each map has its
    own (`mapcolor.go`), shown as a dot or as a pastel with a deep ink
    (`tinted`); a kind of clip has its own. The accent is for what acts,
    what is chosen, links and the AI's spark.
  - The words of the UI are of no one device (记下, never 回车记下): a
    phone is to come.
  A page is drawn first as an interactive draft (an HTML artifact; the
  structure's is https://claude.ai/artifact/J4Ngp4MFp9B7KnLVJhiavb, the
  style's https://claude.ai/artifact/PrnAB6DckyndB9iRduA9bN), then built
  to it, its snapshot held beside the draft: the draft uses only what
  MyGo has (stacked and inset shadows, transitions, poses, the system's
  fonts). Things move as in the drafts (`design.go`: `easeLook` for colors
  and shadows, `rowMotion` for rows that come and go, `press` for what
  gives under the pointer, `hoverLift` for cards that open, `easeDetent`
  the curve, `cubic-bezier(.22,1,.36,1)`); nothing may move what
  is under the pointer away from it on a click, as a part opening on
  focus would. The tests run with no motion (`app.noMotion`).
- What the user's corrections taught, to design by (each cost a version
  to learn; the user asked that they be kept here):
  - Cards of one kind stand apart on the white leaf, each white with a
    hairline (`wallCard`: 收集's wall, what waits for a place). Not each
    a pale block of its own, which read as dull islands; and not white
    cards on one pale ground they share, which the user had asked for
    and then found one gray slab. A lone thing on the leaf (a field, a map's card) may be a
    pale block.
  - A card shows what it is, not what can be done with it. A click opens
    it; all that can be done is one menu, under a faint ⋯ at its corner
    and under the pointer's other key, the same menu (a phone has no
    other key, so the ⋯ stays). At most one key stands on a card, and
    only when it is the next thing to do (放过去 by a place offered).
    A row of keys on every card was too much.
  - What comes forward of the page is told by a shadow, not by a line
    round it: an outline over the page read as something in the way.
  - When the user says a thing is good but has one fault, mend the fault
    and leave the thing: the bar open was liked, and was rebuilt as one
    line for covering the page, and had to be put back.
  - What a field will do with the words is said on it, and can be
    changed there (记为 灵感 / 任务): a rule that decides silently is a
    way in the user cannot find.
  - Each main thing to do has its way in where it is looked for (the
    line to add a task on 今天's lists), though another way does it too.
  - What puts a thing away says so and gives the way back at once
    (撤回), and a way back later. The same for what is ticked off,
    moved or tagged (`tellUndo`; `DESIGN.md`'s Hands, with the keys:
    Escape, the arrows, ⌘1–3, ⌘K, and what the left column takes when
    dropped on).
  - Two kinds of thing that the user wants as one stay each in its own
    store and are joined by a link, made of each other without loss.
  - Before a page is shipped, hold its snapshot beside the reference and
    go part by part: the ground, the card, the type's weight, the color
    of the small keys. The user does, and sees it at once.
- The app's icon is drawn by `tools/icon/`; `resources/icon.png` is the
  desk (`tools/icon/desk.py`: a white note on the app's violet-blue, three lines
  each led by a dot of a map's color, the last ticked, the AI's spark), and
  `appicons/` the icons to choose in the settings (the desk, the vivid note
  of `vivid.py` in three colors, the knowledge cards of `bold.py`, the
  orbit), with their menu bar icons (`tray.py`).
- The semantic search model is a plugin downloaded from the public
  `mindmap-models` repository (`plugin.go`).
- Auto-updates: `mygo.json`'s `updates` and MyGo's updater plugin. Releases
  sign the update files and publish them (`.github/publish-updates.sh`) to
  `mindmap-models`' release `desktop-latest`, which apps read from
  github.com with no API call (`updates.url`), and to `desktop-v<version>`
  for apps before 0.30.1. The secrets `MYGO_UPDATER_PRIVATE_KEY` and
  `RELEASES_TOKEN` are the GitHub environment `mymind`'s.
- macOS builds are per chip, `darwin/arm64` and `darwin/amd64`, a disk
  image each; each updates from its own manifest.
- Sync with GitHub: `internal/ghsync` (a three-way merge over the REST API,
  state in `<maps folder>/.sync/`), wired in `sync.go`.
- Screenshots are the app's own (`internal/screenshot`), on OneShot's way:
  every display frozen under a window of its own (`session.go`), a panel
  so that it shows over full-screen apps without activating the app, a region
  framed there with handles and marked up in colors, the toolbar by it.
  macOS may keep those windows below the menu bar: the shot is drawn offset
  by where the window really is (`screenShot.off`). No system tool is used.
  Global shortcuts reach the app with a menu open (`menus_darwin.go`; and,
  as macOS hands them over only once the menu closes, the keyboard's state
  is watched while a menu of the app is open, `watchKeysUnderMenu`, which
  needs no permission; the listener of `keytap_darwin.go` needs 输入监控),
  and the shot closes that menu once taken.
  Each step goes to `screenshot.log`, shown in the settings.
- The user writes in Chinese; the app's texts are Chinese.
