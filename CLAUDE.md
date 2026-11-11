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
4. Commit, push to the working branch, bump `version` in `mygo.json` (the
   third number for fixes and small changes, 0.25.0 → 0.25.1; the second
   only for a new feature of some size, 0.25.x → 0.26.0), add
   its section to `CHANGELOG.md` (in Chinese: the update window shows it),
   and release by running the `build` workflow with `release: true`.

## Notes

- The UI's look is in `design.go` (sizes, helpers, the theme of
  `quietTheme`) and `icons.go` (icons drawn, not glyphs, so that they sit
  centered; the rail's and the lists' are one family of thin lines,
  `drawRailIcon`). The look is plain and restrained, as Apple's pages are
  (the user's reference is https://library.aimran.es/, after flomo): two
  levels only, a cool pale ground (`ground`, one of `grounds` chosen in
  the settings, `config.Ground`) and white cards on it with large round
  corners (`radiusCard`), told by a shadow so soft it is hardly seen
  (`soft`, in `raise` and `listCard`; `lifted` for what is over the page:
  a card under the pointer, `hoverLift`, the canvas's bars, popups,
  `modal`), with no ring; keys, tabs and the fields of search are pills
  (`radiusPill`), icon buttons round; text large (14 to 16) with room
  around it; few controls in sight, what can be done with a row showing
  under the pointer; one color, the violet-blue accent: the keys that act
  (`inkButton`, `accentButton`), the outline and halo of the line written
  in (`focusHalo`), tags on a pale wash of it (`softTag`). What is chosen
  among a few (the sidebar's part, a `pillTab`, a `segmented`, the views'
  tabs) is on a white pill that slides to it from the one chosen before
  (`slidePill`, a `Match` transition, behind the others' words). The desk and 收集 write in
  the same card (`write.go`, `writeCard`: a tag and a reference offered
  as they are written, a picture, bold, lists; a reference is kept in
  the note's text as `<clip:id>` and shown under its words, `noteText`); other keys are a
  shade of the ground (`key`, `textKey`). A part of a page is a small
  heading (`trayHead`) over a card. There is no sidebar: the app has three
  places, 今天, 项目 and 收集 (`places.go`), a switch of them at the top of
  the pages of lists (`placesBar`, with the search of everything and the
  settings); 项目 is the maps as cards, by folder (`projectsView`), and a
  map opened takes the page, its bar going back to 项目. The words of the
  UI are of no one device (记下, never 回车记下): a phone is to come.
  A map is on white, the pages of lists on the ground. The
  accent is kept for
  what is chosen, for links and for the AI's spark. Color is kept for
  small things that mean something: each map has its own (`mapcolor.go`),
  shown as a dot by its tasks and what was put on it, or as a pastel with
  a deep ink (`tinted`); a kind of clip has its own. The
  desk (`today.go`) is the first page: write it down, today's tasks, what
  was written lately (only the inbox's tasks wait for a map: a note's
  place is 收集). The desk, the tasks and 收集 are laid out the same: a
  `pageColumn` in the middle, its `pageHeader`, a `captureBox` to write
  in, `pillTab`s to narrow by, then trays; what is chosen shows its
  details in a pane on the right, as the topic's panel on the maps' page,
  which is put away until asked for. The whole is being rebuilt to the draft
  https://claude.ai/artifact/J4Ngp4MFp9B7KnLVJhiavb, step by step: the places,
  项目, and 今天 with the tasks in it (`tasksView`: list, week, month; tasks
  that repeat, `model.Repeat`, `repeatProp`, `parseRepeat`) are done; to
  come, one bar at the window's foot that writes, searches and asks the
  AI, and what is chosen in a sheet from the right. A task's state, priority and day are
  set the same way in both (`segmented`, `dateProp`). On the canvas the
  main topics are on a pastel of their branch's color, those below on a
  faint tint (the scheme 明亮 of `style.go`). Notes and clips are one
  page, 收集 (`libview.go`: a wall of cards, narrowed by kind and by tag;
  its 回顾 shows a few old notes in place, `memos.go`). 项目 has the maps
  and their folders, those starred first, the folder they are all in at
  its foot: what is
  collected is 收集's (the word of the UI, never 资料库), and is on a map
  by a topic's link. The AI has two entries, 问 AI (whose panel lists
  what else it does, before a conversation) and AI 归位.
  A page is drawn first as an interactive draft (an HTML artifact, the
  desk's is https://claude.ai/artifact/RU6pxJSxMAsGbVYQrvs21b), then built
  to it, its snapshot held beside the draft: the draft uses only what
  MyGo has (stacked and inset shadows, transitions, poses, the system's
  fonts). Things move as in the drafts (`design.go`: `easeLook` for colors
  and shadows, `rowMotion` for rows that come and go, `press` for what
  gives under the pointer, `hoverLift` for cards that open, `easeDetent`
  the curve, `cubic-bezier(.22,1,.36,1)`); nothing may move what
  is under the pointer away from it on a click, as a part opening on
  focus would. The tests run with no motion (`app.noMotion`).
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
