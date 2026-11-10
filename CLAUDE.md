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
  `drawRailIcon`). The palette is warm and restrained: paper for the page,
  a shade of beige for the panels, the rail and the board's columns, white
  cards (`cardColor`), a near-black ink, thin warm lines, a violet-blue
  for what acts. Color is kept for small things that mean something: each
  map has its own (`mapcolor.go`), shown as a dot by its tasks and what was
  put on it, or as a pastel with a deep ink (`tinted`); a kind of clip has
  its own. A narrow rail has the app's parts (`rail.go`), the maps' folders
  beside it when asked; the desk (`today.go`) is the first page: write it
  down, today's tasks, what waits for a place. On the canvas the main
  topics are on a pastel of their branch's color, those below on a faint
  tint (the scheme 明亮 of `style.go`). Notes and clips are one page, 收集
  (`libview.go`: a wall of cards, narrowed by kind and by tag; its 回顾
  shows a few old notes in place, `memos.go`). The maps' list is beside the
  rail on the maps' page only, with maps and their folders, those starred
  first: what is collected is 收集's (the word of the UI, never 资料库), and
  is on a map by a topic's link. The AI has two entries, 问 AI (whose
  panel lists what else it does, before a conversation) and AI 归位.
  Design drafts are in the Design artifact the user reviews before a page
  is built.
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
  Global shortcuts reach the app with a menu open (`menus_darwin.go`), and
  the shot closes that menu once taken.
  Each step goes to `screenshot.log`, shown in the settings.
- The user writes in Chinese; the app's texts are Chinese.
