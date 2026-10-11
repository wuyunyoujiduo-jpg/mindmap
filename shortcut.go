package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Global shortcuts as the settings show and record them: kept as MyGo
// accelerators ("Control+Alt+A"), shown as the system writes them, ⌃⌥A on
// macOS and Ctrl+Alt+A elsewhere, and set by pressing them, as OneShot
// does.

// modOrder is the order of modifiers, as macOS menus write them.
var modOrder = []struct {
	mod      ui.Modifiers
	acc      string
	mac, win string
}{
	{ui.Ctrl, "Control", "⌃", "Ctrl"},
	{ui.Alt, "Alt", "⌥", "Alt"},
	{ui.Shift, "Shift", "⇧", "Shift"},
	{ui.Super, "Command", "⌘", "Win"},
}

// keyNames are the keys a shortcut may end with, by their accelerator
// names.
var keyNames = func() map[ui.Key]string {
	m := map[ui.Key]string{
		ui.KeySpace: "Space", ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",",
		ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeySemicolon: ";", ui.KeyQuote: "'",
		ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: "\\", ui.KeyBackquote: "`",
		ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyUp: "Up", ui.KeyDown: "Down",
	}
	for i := range 26 {
		m[ui.KeyA+ui.Key(i)] = string(rune('A' + i))
	}
	for i := range 10 {
		m[ui.Key0+ui.Key(i)] = string(rune('0' + i))
	}
	for i := range 12 {
		m[ui.KeyF1+ui.Key(i)] = fmt.Sprintf("F%d", i+1)
	}
	return m
}()

var macKeys = map[string]string{"Left": "←", "Right": "→", "Up": "↑", "Down": "↓"}

// parseAccel reads an accelerator into its modifiers and key name.
func parseAccel(acc, goos string) (ui.Modifiers, string, bool) {
	parts := strings.Split(strings.TrimSpace(acc), "+")
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return 0, "", false
	}
	var mods ui.Modifiers
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "control", "ctrl":
			mods |= ui.Ctrl
		case "alt", "option":
			mods |= ui.Alt
		case "shift":
			mods |= ui.Shift
		case "command", "cmd", "super", "meta":
			mods |= ui.Super
		case "cmdorctrl", "commandorcontrol":
			if goos == "darwin" {
				mods |= ui.Super
			} else {
				mods |= ui.Ctrl
			}
		default:
			return 0, "", false
		}
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	if len(key) == 1 {
		key = strings.ToUpper(key)
	}
	return mods, key, true
}

// accelLabel shows an accelerator as the system writes shortcuts.
func accelLabel(acc, goos string) string {
	mods, key, ok := parseAccel(acc, goos)
	if !ok {
		return acc
	}
	var parts []string
	for _, m := range modOrder {
		if mods&m.mod != 0 {
			if goos == "darwin" {
				parts = append(parts, m.mac)
			} else {
				parts = append(parts, m.win)
			}
		}
	}
	if goos == "darwin" {
		if k, ok := macKeys[key]; ok {
			key = k
		}
		return strings.Join(parts, "") + key
	}
	return strings.Join(append(parts, key), "+")
}

// keyLabel shows an accelerator as this system writes shortcuts.
func keyLabel(acc string) string { return accelLabel(acc, runtime.GOOS) }

// recordedAccel makes the accelerator of keys pressed while recording, or
// reports that they make none: a key needs a modifier, but F1–F12, and
// Shift alone only types a capital.
func recordedAccel(mods ui.Modifiers, key ui.Key) (string, bool) {
	name, ok := keyNames[key]
	if !ok {
		return "", false
	}
	fn := key >= ui.KeyF1 && key <= ui.KeyF12
	if (mods == 0 || mods == ui.Shift) && !fn {
		return "", false
	}
	var parts []string
	for _, m := range modOrder {
		if mods&m.mod != 0 {
			parts = append(parts, m.acc)
		}
	}
	return strings.Join(append(parts, name), "+"), true
}

// shortcutField is a global shortcut of the settings: it shows the
// shortcut, and a click records the next keys pressed; Esc cancels.
// def is what an empty setting means, set saves a new accelerator ("" for
// the default) and registers it, and pause lets go of the shortcut while
// recording, so that pressing it is recorded rather than run.
func (a *app) shortcutField(c *ui.Context, name, value, def string, set func(acc string) error, pause func()) {
	t := c.Theme()
	recording := a.recordingKey == name
	current := value
	if current == "" {
		current = def
	}
	label, col := keyLabel(current), t.Text
	if recording {
		label, col = "请按下新的快捷键…", t.TextMuted
	}
	border := t.Border
	if recording {
		border = t.Accent
	}
	ui.Row(c.Key("sc:" + name)).Gap(10).AlignItems(ui.Center).Children(func() {
		box := ui.Row(c.Key("rec")).Width(180).Height(30).Padding(0, 10).AlignItems(ui.Center).
			Radius(7).Border(1, border).Background(t.Background).Focusable().Cursor(ui.CursorPointer).
			Label(name).Tooltip("点一下，再按下新的组合键")
		box.HandleInput(func(ev ui.InputEvent) bool {
			if a.recordingKey != name || ev.Kind != ui.InputKeyDown {
				return false
			}
			if ev.Key == ui.KeyEscape && ev.Mods == 0 {
				a.stopRecording(set, value)
				return true
			}
			acc, ok := recordedAccel(ev.Mods, ev.Key)
			if !ok {
				return true // a modifier alone, or a key without one: wait
			}
			if acc == def {
				acc = ""
			}
			a.recordingKey = ""
			if err := set(acc); err != nil {
				a.keyErr[name] = "这个组合键用不了（可能被其他应用占用）：" + err.Error()
				_ = set(value)
			} else {
				delete(a.keyErr, name)
			}
			return true
		})
		if box.Clicked() && !recording {
			a.recordingKey = name
			delete(a.keyErr, name)
			pause()
		}
		if recording && !box.Focused() && box.PressedOutside() {
			a.stopRecording(set, value) // clicked elsewhere
		}
		box.Children(func() {
			ui.Text(c, label).FontSize(textBody).TextColor(col).NoWrap().PassThrough()
		})
		if value != "" && !recording && ui.Link(c, "恢复默认", "").FontSize(12).Clicked() {
			if err := set(""); err != nil {
				a.keyErr[name] = err.Error()
			}
		}
	})
	if msg := a.keyErr[name]; msg != "" {
		ui.Text(c, msg).FontSize(12).TextColor(t.Danger).Wrap()
	}
}

// stopRecording ends recording, with the shortcut as it was.
func (a *app) stopRecording(set func(string) error, value string) {
	a.recordingKey = ""
	_ = set(value)
}
