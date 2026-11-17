package main

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/wuyunyoujiduo-jpg/mindmap/internal/library"
)

// What is typed in a clip's details is kept after a pause in the typing,
// as a map is, not at every letter: each letter wrote the clip's file and
// had everything worked out from the clips worked out again (the search's
// index of all of them, how many hold a secret, the tags, the links of
// every map), 1.17 s a letter among 800 notes. The clip in memory is as
// it was typed at once, so its card and its details show it; its file,
// and what is counted from the clips, follow when the typing stops.
//
// The last letters are never lost: what waits is kept when the field is
// left, when the details close or show another clip, when 收集 is left,
// before the library's folder is read again, before a sync reads it, and
// as the app is left (keepTyped's callers).
//
// With a second password set, a clip that holds what is to be sealed is
// kept at once, as before: a secret is sealed under the hand that writes
// it (vault.go), and is not left in the open in memory for a pause.

// typed is what was typed in clips' details and is not kept yet.
type typed struct {
	ids   map[string]bool // the clips changed
	at    time.Time       // when the last letter came
	field string          // and the field it was typed in
}

// clipTyped takes a clip as changed by a letter typed in a field of its
// details.
func (a *app) clipTyped(clip *library.Clip, field string) {
	if a.vault != nil && a.vault.On() && clip.Unsealed() {
		a.saveClip(clip)
		return
	}
	t := &a.collect.typed
	if t.ids == nil {
		t.ids = map[string]bool{}
	}
	t.ids[clip.ID], t.at, t.field = true, a.now(), field
}

// typedLeft keeps what was typed in a field of a clip's details once the
// field is left, for another or for none.
func (a *app) typedLeft(clip *library.Clip, field string, in ui.Element) {
	if t := &a.collect.typed; t.ids[clip.ID] && t.field == field && !in.Focused() {
		a.later(a.keepTyped)
	}
}

// waitsTyped tells whether a clip has letters typed and not kept yet.
func (a *app) waitsTyped(id string) bool { return a.collect.typed.ids[id] }

// keepTyped keeps now what was typed and is not kept yet.
func (a *app) keepTyped() {
	t := &a.collect.typed
	if len(t.ids) == 0 || a.lib == nil {
		return
	}
	ids := t.ids
	t.ids = nil
	for id := range ids {
		if c := a.lib.Find(id); c != nil {
			a.saveClip(c)
		}
	}
}

// typedDue keeps what was typed once the typing has paused, or the clip
// it was typed in is no more the one shown; it runs at every frame.
func (a *app) typedDue(c *ui.Context, now time.Time) {
	t := &a.collect.typed
	if len(t.ids) == 0 {
		return
	}
	wait := autosaveDelay - now.Sub(t.at)
	if wait <= 0 || len(t.ids) > 1 || !t.ids[a.libSel] || !a.collectShown() {
		a.keepTyped()
		return
	}
	c.After(wait)
}
