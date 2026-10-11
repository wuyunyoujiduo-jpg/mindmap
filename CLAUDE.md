# MindMap

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
4. Commit, push to the working branch, bump `version` in `mygo.json`, add
   its section to `CHANGELOG.md` (in Chinese: the update window shows it),
   and release by running the `build` workflow with `release: true`.

## Notes

- The UI's look is in `design.go` (sizes, helpers) and `icons.go` (icons
  drawn as lines, not glyphs, so that they sit centered).
- The app's icon is drawn by `tools/icon/`; `resources/icon.png` is the
  knowledge cards, and `appicons/` the icons to choose in the settings.
- The semantic search model is a plugin downloaded from the public
  `mindmap-models` repository (`plugin.go`).
- Auto-updates: `mygo.json`'s `updates` and MyGo's updater plugin. Releases
  sign the update files and publish them to `mindmap-models`, tagged
  `desktop-v<version>` (secrets `MYGO_UPDATER_PRIVATE_KEY`, `RELEASES_TOKEN`).
- Screenshots come from OneShot, in `internal/screenshot`.
- The user writes in Chinese; the app's texts are Chinese.
