package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/wuyunyoujiduo-jpg/mindmap/internal/model"
)

// A map's name follows its central topic, that of its first canvas, while
// the name is the one the app gave it: a new map's ("新导图 2") or the
// topic's as it was. A name the user gave stays.

// rootTitle is the map's central topic, as a file name may have it: its
// first line, without what file names cannot hold.
func rootTitle(f *model.File) string {
	if f == nil || len(f.Sheets) == 0 || f.Sheets[0].Root == nil {
		return ""
	}
	return nameFor(f.Sheets[0].Root.Title)
}

var nameUnsafe = regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]+`)

func nameFor(title string) string {
	title, _, _ = strings.Cut(title, "\n")
	title = strings.Join(strings.Fields(nameUnsafe.ReplaceAllString(title, " ")), " ")
	title = strings.Trim(title, ". ")
	if r := []rune(title); len(r) > 60 {
		title = strings.TrimSpace(string(r[:60]))
	}
	return title
}

var numbered = regexp.MustCompile(` \d+$`)

// follows tells whether a map's name is the one the app gave it after the
// topic followed, numbered when another map had it, or a new map's.
func follows(name, followed string) bool {
	base := numbered.ReplaceAllString(name, "")
	return base == followed || base == "新导图"
}

// followTitle renames the open map after its central topic, when its name
// still follows it; save calls it. nameTitle is the topic its name was
// given after.
func (a *app) followTitle() {
	if a.file == nil || a.path == "" {
		return
	}
	title, name := rootTitle(a.file), mapName(a.path)
	followed := a.nameTitle
	a.nameTitle = title
	if title == "" || title == name || !follows(name, followed) {
		return // the same, or a name the user gave
	}
	to := freeMapPath(filepath.Dir(a.path), title)
	if err := os.Rename(a.path, to); err != nil {
		return
	}
	model.MoveAssets(a.path, to)
	a.moved(a.path, to)
}
