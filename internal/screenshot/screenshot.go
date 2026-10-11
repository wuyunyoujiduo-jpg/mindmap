// Package screenshot is the app's screenshot: a region of the screen,
// marked up with boxes, arrows, lines and text, then kept in the library,
// copied or saved. On macOS the system's tool frames the region and an
// Annotator marks it up in an ordinary window; elsewhere a window laid over
// each display does both (session.go).
package screenshot

import (
	"errors"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// Action is what is done with the shot.
type Action int

const (
	Copy    Action = iota // to the clipboard
	Save                  // to a file
	Library               // into MyMind's library
)

// ErrBusy is Start's error while a shot is under way.
var ErrBusy = errors.New("上一次截图还没有结束")

var (
	mu        sync.Mutex
	busySince time.Time // zero when no shot is under way
)

// stuck is how long a shot may last before another may start anyway, in
// case its windows never showed.
const stuck = 2 * time.Minute

// Log, when set, is told each step of a shot, to find what went wrong.
var Log = func(string, ...any) {}

// shown is how long the windows have to show and draw before the shot is
// handed to fallback.
const shown = 1500 * time.Millisecond

// Start captures every display and opens the windows to frame a region
// in. deliver gets the picture and what to do with it, off the main
// thread, after the windows close; onEnd runs once the shot is over,
// however it ended. When the windows have not shown and drawn soon after,
// they close and fallback, when not nil, takes the shot on another way,
// before onEnd. While a shot is under way, Start returns ErrBusy. Call it
// from any goroutine.
func Start(deliver func(png []byte, act Action), onEnd func(), fallback func()) (bool, error) {
	mu.Lock()
	if !busySince.IsZero() && time.Since(busySince) < stuck {
		mu.Unlock()
		return false, ErrBusy
	}
	busySince = time.Now()
	mu.Unlock()
	done := func() {
		mu.Lock()
		busySince = time.Time{}
		mu.Unlock()
		Log("截图结束")
		if onEnd != nil {
			onEnd()
		}
	}
	Log("正在截取屏幕…")
	shots, err := captureScreens()
	if err == nil && len(shots) == 0 {
		err = errors.New("没有找到屏幕")
	}
	if err != nil {
		Log("截取屏幕失败：%v", err)
		done()
		return false, err
	}
	for _, sh := range shots {
		w, h := sh.bmp.Size()
		Log("屏幕 %d：%v，%dx%d 像素", sh.index, sh.display.Bounds, w, h)
	}
	s := openSession(shots, deliver, done)
	Log("截图窗口已打开")
	go func() {
		time.Sleep(shown)
		if s.isEnded() {
			return
		}
		visible, drawn := false, false
		for _, sh := range s.shots {
			visible = visible || sh.win.IsVisible()
			drawn = drawn || sh.drawn.Load()
		}
		if visible && drawn {
			Log("截图窗口已显示")
			return
		}
		Log("截图窗口没有出现（显示 %v，绘制 %v），改用另一种截图方式", visible, drawn)
		mygo.RunOnMain(func() { s.endThen(fallback) })
	}()
	return true, nil
}

// HasPermission tells whether the app may capture the screen (macOS asks
// the user once).
func HasPermission() bool { return hasCapturePermission() }

// RequestPermission asks for it.
func RequestPermission() { requestCapturePermission() }

// PermissionHelp says where the permission is given.
const PermissionHelp = capturePermissionHelp

// OpenPermissionSettings opens the system's settings where it is.
func OpenPermissionSettings() { openCapturePermissionSettings() }
