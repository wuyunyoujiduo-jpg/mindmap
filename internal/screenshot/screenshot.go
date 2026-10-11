// Package screenshot frames a part of the screen and marks it up before it
// is kept: a window over each display shows what was on it, frozen; a
// drag frames a region, which can be moved and resized; boxes, arrows,
// lines and text mark it; and the toolbar copies it, saves it or keeps it
// in the library. It came from OneShot, a menu bar screenshot tool.
package screenshot

import (
	"errors"
	"sync"
	"time"
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

// Start captures every display and opens the windows to frame a region
// in. deliver gets the picture and what to do with it, off the main
// thread, after the windows close; onEnd runs once the shot is over,
// however it ended. While a shot is under way, Start returns ErrBusy.
// Call it from any goroutine.
func Start(deliver func(png []byte, act Action), onEnd func()) (bool, error) {
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
	openSession(shots, deliver, done)
	Log("截图窗口已打开")
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
