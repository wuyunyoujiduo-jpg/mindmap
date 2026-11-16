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
  pressed, `jobsView` cards at the window's foot on the right (under the top row
  they lay over a page's keys) for work that can be
  counted, `stageLines` for work that cannot, all through `waitShown`). It is flat and exact. The user's references, in turn, were
  flomo, https://library.aimran.es/ and TickTick: TickTick's is the one
  for the style (and the user found shadows, gradients and glows wanting,
  two versions of them); the structure is the app's own, thought out from
  its functions, borrowing where a borrowing serves them and never copied.
  - The shell (`view.go`): the window is a quiet gray (`ground`, one of
    `grounds` chosen in the settings, `config.Ground`). One row at its top,
    on every page, a map's too (`topBar` in `places.go`): the left column's
    key, the switch of the places, 今天, 项目, 收集 and 技能, the one bar,
    the AI's spark, the search of everything, the settings. Under it one
    white leaf with round corners (`leaf`), everything else on it, cut
    into columns by thin lines: what of the place shows, the page, what is
    chosen.
  - The left column (`side.go`, `sidePane`, `sideRow`; put away by
    `config.HideTree`; a row's count a small tablet of one width,
    `sideCount`, white on the row chosen, on the danger's tint for what
    is late: bare faint figures read as left over, the user found) is
    the one way to narrow a page: 今天's lists, what
    waits for a place, and the maps to see the tasks of alone
    (`todaySide`); 项目's directory (`tree.go`, `mapTree`: folders that
    open and close and their maps, so that going from a map to another is
    one click; a map opened opens the folders it is in there and brings
    its row into view, `app.open`, `treeReveal`, and its bar names its
    folder under its name, a click on which shows that folder's maps:
    opened from elsewhere, where it is kept was not to be seen); in both, the maps starred stand first in a part of their
    own, 快捷访问, whether or not they have a task (the user asked for
    it at the head, apart, on 今天 too, where a click on one opens its
    map as in the directory, not its tasks; they stay in their folders
    with a star as well); 收集's kinds, ways to look back and every tag (`libSide`): a kind,
    a tag and 星标, 未整理 or 含密钥 narrow together, each row chosen
    showing so, and the page says each as a chip with a ✕ of its own
    under its head, with how many are left (`libFilters`, `filterChip`;
    a page that said 灵感和资料 over nothing found told the user nothing
    of why, and one ✕ for all the conditions left no way to take off
    one).
  - The one bar (`bar.go`, `oneBar`) is in the top row: one line until
    written in, then the card to write in with its tools (`write.go`,
    `writeCard`: a tag and a reference offered as they are written, a
    picture, bold, lists; a link is begun with [[, which offers thoughts,
    clips, maps and the topics of every map, and is kept as `<clip:id>`
    or `<mindmap:map#id>`, `refToken`, `noteText`; a node's note has
    the same [[, kept as a link of its Markdown, `noteLink`,
    `noteLinkOffers`; what is linked to says so in turn, `refIndex`:
    `wikilinks.go`),
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
    the app's corners (`button`). The app is light, dark or as the
    system is by the settings' first rows (`config.Appearance`,
    `SetAppearance`, MyGo's `Theme.SetSource`).
  - 今天 (`taskspage.go`, `tasksView`): as a list it is one column read
    down (`todayWidth`), headed by the list's name and the line to add a
    task (`tasksAdd`); under today's list, cut from it by a band the
    page's width, what waits for a place as a few lines (`waitingBand`,
    `waitingLine`: the user found the cards there crowding the tasks, and
    chose lines; in a wide window it is a column beside the tasks,
    `waitingRail`, for half the window stood empty; with a task of the
    list chosen it is under the tasks again and the list takes the room
    up to the details: the user found it wrong as a column between a
    task and its details, and then a blank there). What waits is the
    inbox's tasks and the clips the user asked to place later
    (`Clip.ToPlace`, 稍后归位 in a clip's details, 标为待归位 in its menu):
    a clip is at home in 收集 and waits for nothing by itself. Before
    0.88 every clip of the week waited, and the page read as a debt;
    the user chose this: projects are kept in maps, what is collected
    in 收集, and a link joins the two where one serves the other, not
    everything collected flowing into a map at last. Its cards are the left column's list 等着归位
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
    A task may have a time of its day to remind at, chosen or written,
    in its line too (明天 9点 写周报, `parseRemind`; `Task.Remind`,
    `remindProp`, `reminders.go`: a notification of its own, and one a
    day of what is due and late in every map). 本周回顾 (`review.go`)
    is the week looked back on, and those before it: done, late, kept,
    with no AI asked, and copied as a report (`weekReport`).
    The search of everything finds tasks, ticked off and put on tomorrow
    in its list (`paletteItem.task`).
    A project chosen in the left column shows all of its tasks not done,
    by when (`tasksOfMap`), with the way to its map, and nothing that
    waits: that is of no one project. The projects there are the maps
    with a task to do, those starred first, then by when they were
    opened last, six and the rest under 全部项目 (`sideMaps`); never the
    inbox, a list above; each its dot with, round it, how far it is, and at
    its end its count, which under the pointer gives way to the key to
    its map (`sideRowTrail`). A ring, a count and an arrow in a row at
    the end were three marks, and not good to look at, the user found;
    and a desktop need not be as a phone is, the user said: what a
    pointer shows may wait for the pointer here. Their counts and
    the lists' are one count (`countsAsTask`); with a project chosen no
    list shows as chosen; what is late has a list of its own while
    there is any (`tasksLate`).
  - 项目 (`projectsView`): the maps as cards, each with the map drawn
    small as its face (`drawMapFace`) and a ring for its tasks, white
    with a hairline as every card of a kind (`wallCard`); with the
    left column put away, the folders are a row of pills (`projFolder`).
    A map opened has its own bar under the top row; the canvas's bar at
    its foot has the key that shows some tasks alone, the rest dimmed
    (`taskFilterKey`, in the menu 视图 too: the top bar had no room for
    it in a narrow window, which CI's wider fonts told); on the canvas the
    center is in the ink, the main topics on a pastel of their branch's
    color, those below on a faint tint (the scheme 明亮 of `style.go`).
    There is no whiteboard, and is not to be (the user asked, 0.93, and
    chose this): a fourth kind of thing would ask at every thought where
    it goes. What a tree cannot hold is a floating topic (`annotate.go`),
    made handy instead: dragged out of the tree and back into it, made
    by the canvas's menu, of a clip too (`DESIGN.md`'s Hands).
  - 图解 (`diagram.go`, `diagramview.go`, `internal/diagram`): a topic's
    branch drawn by the AI to be understood by, in the canvas's place
    (`viewDiagram`): a flow in lanes, the states of a thing, a table of
    what is decided when, and the nouns of the matter (名词), read three
    ways under the diagram (`diaWays`): 讲一遍, told a step a sentence;
    拿例子走一遍, a case the AI chose lighting the same way on each kind;
    考我, the table's results covered, and what was missed asked back
    from 今天 three days on (`diaLine`). The user asked for it (0.96): a
    tree says what is under what, and not what comes after what, turns
    into what, or is decided when. A set is a view of the map, not a
    fourth kind of thing (as the whiteboard would have been): drawn from
    a topic, each box saying in the right column the topics it came from,
    a click going back to them; telling when they changed since
    (`branchSeen`, `diaChanges`); drawn again, never edited by hand (the
    line 让 AI 改图 tells the AI how). What the map leaves unsaid the AI
    marks and does not decide (`Gap`). Kept beside the library, in
    `.library/diagrams/`, a file a topic by the topic's ID, on whichever
    canvas of the map it is (the user: a map has several canvases, and
    they are to work); a noun's places are looked up as it is shown, on
    every canvas (`mentions`). The layout is the app's own
    (`internal/diagram/layout.go`: a grid, routes round the boxes, and a
    row a step when one would cross a box, `Crossed`; a flow with no
    lanes has its longest way as its first column, `sideLanes`). Its
    steps are one calm blue on every map (`diaBlue`): in the topic's own
    color a step read as an end refused or reached. A noun is looked at
    on the diagrams too, what speaks of it lit (`nounLight`); with nothing
    chosen the right column says what to look at next, what is unsaid and
    the cases, not the tabs again; a jump to a topic leaves the way back
    on its toast; a branch cut and pasted keeps its set (`carryDias`);
    the arrows go from box to box (`diaMove`). It was tried on a real
    project's rules (ERGORA's), which is where a document in Markdown
    came to be imported as one, and a folder of them as one map, a canvas
    a document (`internal/formats/document.go`, `importFolder`). Its way
    in is a tab of the map, with 导图, 大纲 and 看板 (`mainViews`,
    `showView`): a key among the topic's was not seen, the user found;
    with no set to show it is its own first page (`diaHome`), which draws
    the topic chosen and lists the map's sets. Under the map's tabs its
    head is one quiet row (`diaHead`): the topic, the kinds as words, the
    one shown on the accent's tint. A flow shows as a drawing or as steps
    (`diaSteps`, `Graph.Steps`): its main way a step a line, numbered, and
    the other ways out of a step said by it in words, a way back a number
    to go to and not a line to follow. The user asked whether a drawing
    whose arrows crossed could be mended or was the wrong form: it was
    the form, for a drawing says every "then" with a line and it is the
    ways back that cross them; as steps the order says "then" for
    nothing. A flow with two or more ways back or leaps shows so by
    itself (`stepsSuit`), and either by a key at its corner. A kind is
    saved as a picture, and a set put among its canvas's slides, a slide
    a kind after its topic's (`diaFigure`, which draws a kind bare, with
    nothing to press, scaled to what holds it; `diaImage`, `slidesFor`,
    `Set.Present`). Its draft
    is https://claude.ai/artifact/QMRCCAAu3tW9ni6CcVkRaL. Checks across
    maps are not built: the user put them off.
  - 收集 (`libview.go`): notes and clips, a wall of cards that
    stand apart on the white page, each white with a hairline
    (`wallCard`; the user found one gray ground under them a slab); its 回顾 shows a
    few old notes in place (`memos.go`). A card's picture is read away
    from the window's work and small (`thumbs.go`, `filePicture`): read
    whole as the cards were first drawn, a wall of screenshots held the
    window still the first time 收集 was opened, and a map of pictures
    when it was opened (the canvas's take the same way, `thumbnail`).
    Nothing that reads a file or decodes a picture belongs in a frame.
    What looks like a key or a password in what is kept (`library.Mask`,
    `Clip.Secret`) is in dots on cards, lines, titles and cards to share,
    and in what goes to an AI; a clip's details show it when asked
    (`secretShown`), and the left column lists the clips that hold one
    (含密钥, `secretCat`). A video is a file clip of a video's ending
    (`video.go`, `isVideo`), shown as one: its first picture on its card,
    playing without sound under the pointer, a player in its details and
    larger over the window, a kind of its own in the left column while
    there is one; the players are the system's (MyGo's `ui/video`),
    opened as shown and closed a frame after (`sweepVideos`). The user
    dropped one in and saw a random name and nothing of it. The AI sees
    it by a few of its pictures (`queueGrab`, `stepGrab`: a player in a
    Tester, which draws on the CPU, stepped a little at each of the
    window's frames so that the window goes on; in a queue of their own,
    the clip queue going on meanwhile, the time counted while the window
    shows, and a failure not kept). A card shows its poster, taken so
    and kept in the cache (`posterPath`), and a player only under the
    pointer: the wall builds all its cards.
    What the AI writes of a clip is as long as what it was made of
    (`applyOrganized`, `summaryLen`, `analysisLen`), and never what it
    does not know (`dropFiller`: a sentence of the AI's answer at a time,
    narrow, never of what the user wrote); the details show the user's content and
    note first, the summary above a long content, and fold the analysis
    and the words it is found by (`foldKey`): the user found a line kept
    and a page of the AI's words under it.
    The thoughts that name a clip and those it names
    are in its details (`clipMentions`). What is collected is 收集's (the
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
    Escape, the arrows, ⌥⌘1–3, ⌘J, and what the left column takes when
    dropped on).
  - Two kinds of thing that the user wants as one stay each in its own
    store and are joined by a link, made of each other without loss.
  - Before a page is shipped, hold its snapshot beside the reference and
    go part by part: the ground, the card, the type's weight, the color
    of the small keys. The user does, and sees it at once.
  - A second level of tabs is not a second track with a pill under the
    first: the two read as one thing said twice, and were ugly, the user
    found of 图解's page. The level under is quiet words at the left, the
    one chosen on the accent's tint.
  - A page of a map has its way in among the map's tabs, where its
    views are looked for; a small key among a topic's is not found.
  - A row of tabs stays where it is whichever of them is chosen: what
    one view alone has beside them keeps its room in the others
    (`structureRoom`), the tabs are in the bar's middle with an end at
    each side (`toolbar`), and a column that comes and goes at the bar's
    end leaves them be (`away`). They jumped on a click of 图解 and of
    大纲, which the user saw at once (`TestViewTabsStay`). A width to
    keep a thing in place is measured, not a figure: one font's figure
    pushed the bar's keys out of it on CI's (`TestToolbarFitsNarrowWindow`).
  - What the app finds by its name and place (the inbox, `inboxPath`)
    says so in its menu and is neither renamed, moved nor deleted there:
    the user asked whether it could be, seeing that it could, and chose
    it to be always there, as where what has no place goes.
  - A menu is short: what is done every day at its head, the rest in a
    submenu of its kind, and what a key or a double click does that no
    one looks for here left out (`nodeMenu`; the user found some thirty
    rows too many). An item that does nothing seen where it is chosen is
    a fault: 管理标签… chose the page of a column that was put away.
  - A tag is made where words are written, by #: in a topic's title too,
    offered under it as it is written and made one only when chosen (a
    click or Tab, `titleTagOffer`), else words of the title; on the
    canvas, in the right column and on the outline's lines alike (the
    user asked for the two after the first). A Tab taken for a tag is
    taken only while one is offered: the outline's Tab indents. A rule that
    made every #word a tag was one the user could not see nor refuse,
    and the tags went from sight while a title was edited: they show
    there, each taken off by a click.
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
  image each; each updates from its own manifest. A release builds
  Apple silicon alone unless the workflow is run with `all: true`, which
  builds Intel's and Windows' too: without it now and then, those stay
  at an old version (Intel's stands at 0.33.1; the user asked for the Mac's alone, 2026-11).
- Agents (`internal/mcp`, `mcp.go`): a project has one map, found by the
  repository an agent works in (`projectMap`: the map whose `Project` is
  that repository, a worktree's too, or whose `Repo` is its origin, as
  on the user's other computer; made when there is none). What agents
  write without being told where goes under the map's 待整理 (`capture`,
  `log_session`'s todos) and into its log (`model.File.Log`: sessions,
  decisions, tasks, edits); a decision is whole as a clip tagged 决策
  (`log_decision`). The map's structure stays the user's. Claude Code's
  hooks, and Codex's and Grok Build's, which are kept as its are
  (`MyMind hook start|stop`, `hook.go`, installed by the window's 装上,
  `agentTools`; Codex's and Grok Build's were written from what is
  published of them, with neither tool at hand to try) give the brief as a session begins and ask for `log_session` when
  the code changed. Writers hold the folder's lock (`lock.go`).
  The agents' skills are 技能, the fourth place, after 收集
  (`skillsview.go`, `internal/skills`, the user's, 0.99; a list of 今天
  at first, which the user moved: it is not of the day): which
  agents there are (a table, `tools.json`, published on its own by the
  workflow agent-tools and fetched once a day, `skillsTableURL`, and an
  agent the table lacks found by `~/.name/skills/*/SKILL.md`; one found
  since last seen said on the page, `skillsFresh`), each skill every
  session's, a project's (the maps' `Project`s and Claude Code's
  projects) or a plugin's, and what is wrong with it. A skill taken in
  is kept once, in the maps folder's `.skills` (`~/.mymind-skills` with
  none), synced by ghsync, and the agents' folders hold links to it (a
  junction on Windows): the user asked for one copy and not one an agent
  and a folder. A link in a repository is the user's to commit or not;
  committed, it is said, and `MakeReal` puts the files there. What each
  computer linked is in `.skills/_machines`, which the other offers.
  Everything done to the folders has its way back (`skills.Undo`). What
  is to do for many at once is offered at the page's head, a click each
  (`skillsSuggestions`: take all in, copies alike back to links, links
  to nothing out, what another computer linked, a new tool's links), and
  what is for the user to judge sent to 要处理: one by one was too slow,
  the user found, with seventy skills in sixteen tools. Its left column
  narrows by a part and a tool together (`skillsSide`), the tools with no
  skill folded: sixteen cards on the page were too many to take in. Codex's
  and the others' folders are as their documentation says (`Checked`
  is Claude Code's alone).
  The app shows the logs as AI 动态, a list of 今天's left column
  (`agents.go`, `agentsView`): by day and project, each entry opened by a
  click and taken back by its menu (`takeBack`, with its way back); with
  a project chosen, that project's whole record. What came since it was
  last looked at (`config.AgentsSeen`) is counted on its row and said in
  a line on 今天 (`agentsLine`).
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
