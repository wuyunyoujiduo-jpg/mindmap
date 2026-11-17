package screenshot

import "strings"

// The shortcut as macOS's keyboard events have it (keytap_darwin.go): the
// key's virtual code and the modifiers' flags. Here, and not in the
// Mac's file, so that the tests read it on every system: the settings
// record a shortcut by MyGo's names of the keys (the app's shortcut.go),
// and a key recorded that this table lacked was a shortcut the listener
// could not read, a line of "不认识快捷键" in the log each time a menu
// opened. The arrows were such.

const (
	flagShift   = 1 << 17
	flagControl = 1 << 18
	flagOption  = 1 << 19
	flagCommand = 1 << 20
	// flagMods are the flags a shortcut is told by. An event of an arrow
	// or a function key has others set besides (the numeric pad's, Fn's),
	// which are left out of the comparison.
	flagMods = flagShift | flagControl | flagOption | flagCommand
)

// keyCodes are the virtual key codes of the keys a shortcut may end in:
// every key the settings record (the app's keyNames).
var keyCodes = map[string]int64{
	"A": 0, "S": 1, "D": 2, "F": 3, "H": 4, "G": 5, "Z": 6, "X": 7, "C": 8, "V": 9, "B": 11, "Q": 12, "W": 13,
	"E": 14, "R": 15, "Y": 16, "T": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "=": 24, "9": 25,
	"7": 26, "-": 27, "8": 28, "0": 29, "]": 30, "O": 31, "U": 32, "[": 33, "I": 34, "P": 35, "L": 37, "J": 38,
	"'": 39, "K": 40, ";": 41, "\\": 42, ",": 43, "/": 44, "N": 45, "M": 46, ".": 47, "`": 50,
	"SPACE": 49, "TAB": 48, "RETURN": 36, "ENTER": 36,
	"F1": 122, "F2": 120, "F3": 99, "F4": 118, "F5": 96, "F6": 97, "F7": 98, "F8": 100, "F9": 101, "F10": 109,
	"F11": 103, "F12": 111,
	"LEFT": 123, "RIGHT": 124, "DOWN": 125, "UP": 126,
}

// parseKey reads an accelerator as "Control+Command+A".
func parseKey(key string) (int64, uint64, bool) {
	var flags uint64
	code := int64(-1)
	for _, part := range strings.Split(key, "+") {
		switch p := strings.ToUpper(strings.TrimSpace(part)); p {
		case "CMD", "COMMAND", "CMDORCTRL", "COMMANDORCONTROL", "SUPER", "META":
			flags |= flagCommand
		case "CTRL", "CONTROL":
			flags |= flagControl
		case "ALT", "OPTION":
			flags |= flagOption
		case "SHIFT":
			flags |= flagShift
		default:
			c, ok := keyCodes[p]
			if !ok {
				return 0, 0, false
			}
			code = c
		}
	}
	return code, flags, code >= 0
}
