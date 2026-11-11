//go:build darwin

package screenshot

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// 菜单展开时也能截图。
//
// MyGo 的全局快捷键挂在 Carbon 的应用事件目标上，而菜单展开时，菜单自己的
// 事件循环只把事件交给事件分发器，快捷键就被吞掉了。这里在分发器上转交一下，
// 和 MASShortcut 一样，菜单开着时快捷键照样送到。
//
// 截图先截下屏幕（菜单还在，冻结的画面里有它），再收起菜单，让截图窗口接管
// 鼠标和键盘。

const (
	kEventClassKeyboard = 0x6B657962 // 'keyb'
	kEventHotKeyPressed = 5
)

var (
	getEventDispatcherTarget  func() uintptr
	getApplicationEventTarget func() uintptr
	installEventHandler       func(target, handler uintptr, numTypes uint32, list *[2]uint32, userData uintptr, outRef *uintptr) int32
	sendEventToEventTarget    func(event, target uintptr) int32

	menusMu sync.Mutex
	menus   []objc.ID // the menus tracked now, the last opened last
)

// hotKeysThroughMenus passes global shortcuts on to the app while a menu
// is open, as they are when none is.
var hotKeysThroughMenus = purego.NewCallback(func(next, event, userData uintptr) uintptr {
	// To the application's handlers, MyGo's among them; as handled, so
	// that the dispatcher does not send it there again.
	return uintptr(uint32(sendEventToEventTarget(event, getApplicationEventTarget())))
})

// WatchMenus makes global shortcuts work while a menu of the app is open,
// and keeps the menus open, which a screenshot closes once taken. Call it
// once, on the main thread, as the app starts.
func WatchMenus() {
	carbon, err := purego.Dlopen("/System/Library/Frameworks/Carbon.framework/Carbon", purego.RTLD_GLOBAL|purego.RTLD_NOW)
	if err != nil {
		Log("菜单内快捷键：%v", err)
		return
	}
	purego.RegisterLibFunc(&getEventDispatcherTarget, carbon, "GetEventDispatcherTarget")
	purego.RegisterLibFunc(&getApplicationEventTarget, carbon, "GetApplicationEventTarget")
	purego.RegisterLibFunc(&installEventHandler, carbon, "InstallEventHandler")
	purego.RegisterLibFunc(&sendEventToEventTarget, carbon, "SendEventToEventTarget")
	spec := [2]uint32{kEventClassKeyboard, kEventHotKeyPressed}
	var ref uintptr
	if st := installEventHandler(getEventDispatcherTarget(), hotKeysThroughMenus, 1, &spec, 0, &ref); st != 0 {
		Log("菜单内快捷键：InstallEventHandler %d", st)
	}

	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		return
	}
	center := objc.ID(objc.GetClass("NSNotificationCenter")).Send(objc.RegisterName("defaultCenter"))
	observe := func(name string, fn func(menu objc.ID)) {
		block := objc.NewBlock(func(_ objc.Block, note objc.ID) {
			fn(note.Send(objc.RegisterName("object")))
		})
		center.Send(objc.RegisterName("addObserverForName:object:queue:usingBlock:"), nsString(name), objc.ID(0), objc.ID(0), block)
	}
	observe("NSMenuDidBeginTrackingNotification", func(menu objc.ID) {
		menusMu.Lock()
		menus = append(menus, menu)
		menusMu.Unlock()
		watchKeysUnderMenu()
	})
	observe("NSMenuDidEndTrackingNotification", func(menu objc.ID) {
		menusMu.Lock()
		for i := len(menus) - 1; i >= 0; i-- {
			if menus[i] == menu {
				menus = append(menus[:i], menus[i+1:]...)
				break
			}
		}
		menusMu.Unlock()
	})
}

func nsString(s string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), s)
}

// closeMenus closes the menus open, once the screens are captured with
// them. On the main thread.
func closeMenus() {
	menusMu.Lock()
	open := append([]objc.ID(nil), menus...)
	menusMu.Unlock()
	for i := len(open) - 1; i >= 0; i-- {
		open[i].Send(objc.RegisterName("cancelTrackingWithoutAnimation"))
	}
	if len(open) > 0 {
		Log("收起了 %d 个展开的菜单", len(open))
	}
}
