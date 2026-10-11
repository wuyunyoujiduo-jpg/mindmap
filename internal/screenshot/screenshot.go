// Package screenshot frames a part of the screen and marks it up before it
// is kept: a window over each display shows what was on it, frozen; a
// drag frames a region, which can be moved and resized; boxes, arrows,
// lines and text mark it; and the toolbar copies it, saves it or keeps it
// in the library. It came from OneShot, a menu bar screenshot tool.
package screenshot

import "sync/atomic"

// Action is what is done with the shot.
type Action int

const (
	Copy    Action = iota // to the clipboard
	Save                  // to a file
	Library               // into MindMap's library
)

var busy atomic.Bool

// Start captures every display and opens the windows to frame a region
// in. deliver gets the picture and what to do with it, off the main
// thread, after the windows close; onEnd runs once the shot is over,
// however it ended. A shot already under way is left as it is, and Start
// reports false. Call it from any goroutine.
func Start(deliver func(png []byte, act Action), onEnd func()) (bool, error) {
	if !busy.CompareAndSwap(false, true) {
		return false, nil
	}
	done := func() {
		busy.Store(false)
		if onEnd != nil {
			onEnd()
		}
	}
	shots, err := captureScreens()
	if err != nil || len(shots) == 0 {
		done()
		return false, err
	}
	openSession(shots, deliver, done)
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
