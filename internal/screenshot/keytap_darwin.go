//go:build darwin

package screenshot

import (
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// 菜单展开时的截图快捷键。
//
// 菜单展开时（本应用的「+」菜单、右键菜单，或别的应用的菜单），系统不把全局
// 快捷键（Carbon 的 RegisterEventHotKey）交给应用。这里另装一个只监听的键盘
// 事件监听（CGEventTap），菜单开着也能看到按键：看到截图快捷键，而全局快捷键
// 没有响应，就由它开始截图。它要「输入监控」权限，没有时什么也不做。
//
// 没有这个权限时，本应用自己的菜单展开期间改为查看键盘的状态
// （CGEventSourceKeyState，不需要权限）：快捷键按下了，就开始截图
// （watchKeysUnderMenu）。否则快捷键要等菜单收起才送到，收起菜单时才截图。

const (
	kCGSessionEventTap             = 1
	kCGHeadInsertEventTap          = 0
	kCGEventTapOptionListenOnly    = 1
	kCGEventKeyDown                = 10
	kCGKeyboardEventKeycode        = 9
	kCGEventTapDisabledByTimeout   = 0xFFFFFFFE
	kCGEventTapDisabledByUserInput = 0xFFFFFFFF

	flagShift   = 1 << 17
	flagControl = 1 << 18
	flagOption  = 1 << 19
	flagCommand = 1 << 20
	flagMods    = flagShift | flagControl | flagOption | flagCommand
)

var (
	cgEventTapCreate         func(tap, place, options uint32, mask uint64, callback, refcon uintptr) uintptr
	cgEventTapEnable         func(tap uintptr, enable bool)
	cgEventGetIntegerValue   func(event uintptr, field uint32) int64
	cgEventGetFlags          func(event uintptr) uint64
	cgPreflightListen        func() bool
	cgRequestListen          func() bool
	cgEventSourceKeyState    func(state int32, key uint16) bool
	cgEventSourceFlagsState  func(state int32) uint64
	cfMachPortCreateRLSource func(alloc, port uintptr, order int) uintptr
	cfRunLoopGetMain         func() uintptr
	cfRunLoopAddSource       func(rl, source, mode uintptr)
	cfRunLoopCommonModes     uintptr

	tapMu     sync.Mutex
	tapPort   uintptr
	tapCode   int64 = -1
	tapFlags  uint64
	tapFire   func()
	hotKeyAt  time.Time // when the global shortcut last fired
	seenAt    time.Time // when the shortcut was last seen under a menu
	menuShot  time.Time // when a shot was last begun under a menu
	polling   bool      // the keyboard's state is being watched
	tapLoaded bool
)

func loadTap() bool {
	if tapLoaded {
		return cgEventTapCreate != nil
	}
	tapLoaded = true
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	purego.RegisterLibFunc(&cgEventTapCreate, cg, "CGEventTapCreate")
	purego.RegisterLibFunc(&cgEventTapEnable, cg, "CGEventTapEnable")
	purego.RegisterLibFunc(&cgEventGetIntegerValue, cg, "CGEventGetIntegerValueField")
	purego.RegisterLibFunc(&cgEventGetFlags, cg, "CGEventGetFlags")
	purego.RegisterLibFunc(&cgPreflightListen, cg, "CGPreflightListenEventAccess")
	purego.RegisterLibFunc(&cgRequestListen, cg, "CGRequestListenEventAccess")
	purego.RegisterLibFunc(&cgEventSourceKeyState, cg, "CGEventSourceKeyState")
	purego.RegisterLibFunc(&cgEventSourceFlagsState, cg, "CGEventSourceFlagsState")
	purego.RegisterLibFunc(&cfMachPortCreateRLSource, cf, "CFMachPortCreateRunLoopSource")
	purego.RegisterLibFunc(&cfRunLoopGetMain, cf, "CFRunLoopGetMain")
	purego.RegisterLibFunc(&cfRunLoopAddSource, cf, "CFRunLoopAddSource")
	if p, err := purego.Dlsym(cf, "kCFRunLoopCommonModes"); err == nil {
		cfRunLoopCommonModes = **(**uintptr)(unsafe.Pointer(&p))
	}
	return true
}

// KeysAllowed tells whether the app may listen to the keyboard (输入监控),
// which the shortcut needs while a menu is open.
func KeysAllowed() bool { return loadTap() && cgPreflightListen() }

// RequestKeys asks for it: the system's prompt, or its settings.
func RequestKeys() {
	if loadTap() {
		cgRequestListen()
	}
}

// HotKeyFired is told when the global shortcut fired, so that the
// listener does not take the shot again.
func HotKeyFired() {
	tapMu.Lock()
	hotKeyAt = time.Now()
	tapMu.Unlock()
}

// ListenShotKey watches for the shortcut (as "Control+Command+A") while a
// menu keeps it from the app, and calls fire then. Main thread. Without
// 输入监控, it does nothing.
func ListenShotKey(key string, fire func()) {
	code, flags, ok := parseKey(key)
	tapMu.Lock()
	tapCode, tapFlags, tapFire = -1, 0, nil
	if ok {
		tapCode, tapFlags, tapFire = code, flags, fire
	}
	tapMu.Unlock()
	if !ok || tapPort != 0 || !KeysAllowed() {
		if !ok {
			Log("菜单内截图：不认识快捷键 %s", key)
		}
		return
	}
	tapPort = cgEventTapCreate(kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionListenOnly, 1<<kCGEventKeyDown, keyTap, 0)
	if tapPort == 0 {
		Log("菜单内截图：无法监听键盘")
		return
	}
	src := cfMachPortCreateRLSource(0, tapPort, 0)
	cfRunLoopAddSource(cfRunLoopGetMain(), src, cfRunLoopCommonModes)
	cgEventTapEnable(tapPort, true)
	Log("菜单内截图：已监听 %s", key)
}

var keyTap = purego.NewCallback(func(proxy uintptr, typ uint32, event, refcon uintptr) uintptr {
	if typ == kCGEventTapDisabledByTimeout || typ == kCGEventTapDisabledByUserInput {
		if tapPort != 0 {
			cgEventTapEnable(tapPort, true)
		}
		return event
	}
	if typ != kCGEventKeyDown {
		return event
	}
	code := cgEventGetIntegerValue(event, kCGKeyboardEventKeycode)
	flags := cgEventGetFlags(event) & flagMods
	tapMu.Lock()
	match := tapFire != nil && code == tapCode && flags == tapFlags
	fire := tapFire
	tapMu.Unlock()
	if match {
		shortcutSeen(fire, "键盘监听")
	}
	return event
})

// shortcutSeen is told that the shortcut was pressed while a menu may
// keep it from the app. The global shortcut, when it works, fires too:
// the shot is taken here only when it did not, and once for a press seen
// in two ways.
func shortcutSeen(fire func(), by string) {
	pressed := time.Now()
	tapMu.Lock()
	again := pressed.Sub(seenAt) < 600*time.Millisecond
	seenAt = pressed
	tapMu.Unlock()
	if again {
		return
	}
	go func() {
		time.Sleep(250 * time.Millisecond)
		tapMu.Lock()
		fired := hotKeyAt.After(pressed.Add(-100 * time.Millisecond))
		if !fired {
			menuShot = time.Now()
		}
		tapMu.Unlock()
		if !fired {
			Log("全局快捷键没有响应（可能有菜单展开），由%s开始截图", by)
			fire()
		}
	}()
}

// LateHotKey tells whether the global shortcut firing now is the press a
// shot was already begun for under a menu: the system hands it over only
// once the menu closes, which the shot does.
func LateHotKey() bool {
	tapMu.Lock()
	defer tapMu.Unlock()
	return time.Since(menuShot) < 3*time.Second
}

// watchKeysUnderMenu looks at the keyboard's state while a menu of the app
// is open, which needs no permission, and begins the shot when the
// shortcut goes down. Called as a menu opens; it ends with the last menu.
func watchKeysUnderMenu() {
	tapMu.Lock()
	if polling || !loadTap() {
		tapMu.Unlock()
		return
	}
	polling = true
	tapMu.Unlock()
	go func() {
		defer func() {
			tapMu.Lock()
			polling = false
			tapMu.Unlock()
		}()
		const combinedSession = 0
		was := true // a key held as the menu opens is not a press
		for {
			menusMu.Lock()
			open := len(menus) > 0
			menusMu.Unlock()
			if !open {
				return
			}
			tapMu.Lock()
			code, flags, fire := tapCode, tapFlags, tapFire
			tapMu.Unlock()
			down := fire != nil && code >= 0 && cgEventSourceKeyState(combinedSession, uint16(code)) &&
				cgEventSourceFlagsState(combinedSession)&flagMods == flags
			if down && !was {
				shortcutSeen(fire, "键盘状态")
			}
			was = down
			time.Sleep(25 * time.Millisecond)
		}
	}()
}

// keyCodes are the virtual key codes of the keys a shortcut may end in.
var keyCodes = map[string]int64{
	"A": 0, "S": 1, "D": 2, "F": 3, "H": 4, "G": 5, "Z": 6, "X": 7, "C": 8, "V": 9, "B": 11, "Q": 12, "W": 13,
	"E": 14, "R": 15, "Y": 16, "T": 17, "1": 18, "2": 19, "3": 20, "4": 21, "6": 22, "5": 23, "=": 24, "9": 25,
	"7": 26, "-": 27, "8": 28, "0": 29, "]": 30, "O": 31, "U": 32, "[": 33, "I": 34, "P": 35, "L": 37, "J": 38,
	"'": 39, "K": 40, ";": 41, "\\": 42, ",": 43, "/": 44, "N": 45, "M": 46, ".": 47, "`": 50,
	"SPACE": 49, "TAB": 48, "RETURN": 36, "ENTER": 36,
	"F1": 122, "F2": 120, "F3": 99, "F4": 118, "F5": 96, "F6": 97, "F7": 98, "F8": 100, "F9": 101, "F10": 109,
	"F11": 103, "F12": 111,
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
