package screenshot

import (
	"fmt"
	"testing"
)

// Every key the settings record a shortcut with is one the listener
// reads: the letters, the figures, the signs, F1 to F12, the space and
// the arrows, by the names the settings keep them under.
func TestParseKey(t *testing.T) {
	names := []string{"Space", "-", "=", ",", ".", "/", ";", "'", "[", "]", "\\", "`", "Left", "Right", "Up", "Down"}
	for i := range 26 {
		names = append(names, string(rune('A'+i)))
	}
	for i := range 10 {
		names = append(names, string(rune('0'+i)))
	}
	for i := range 12 {
		names = append(names, fmt.Sprintf("F%d", i+1))
	}
	seen := map[int64]string{}
	for _, name := range names {
		code, flags, ok := parseKey("Control+Alt+" + name)
		if !ok || flags != flagControl|flagOption {
			t.Errorf("%s: not read (%d, %x)", name, code, flags)
			continue
		}
		if other, dup := seen[code]; dup {
			t.Errorf("%s and %s have one code, %d", name, other, code)
		}
		seen[code] = name
	}
	// The arrows' codes, as macOS has them.
	for name, want := range map[string]int64{"Left": 123, "Right": 124, "Down": 125, "Up": 126} {
		if code, flags, ok := parseKey("Command+Shift+" + name); !ok || code != want || flags != flagCommand|flagShift {
			t.Errorf("%s: %d %x %v", name, code, flags, ok)
		}
	}
	for _, bad := range []string{"", "Control+", "Control+Alt", "Control+Home"} {
		if _, _, ok := parseKey(bad); ok {
			t.Errorf("%q read as a shortcut", bad)
		}
	}
}
