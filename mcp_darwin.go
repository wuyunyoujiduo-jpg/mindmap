//go:build darwin

package main

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// hideFromDock keeps "MyMind mcp" out of the Dock: it runs the app's own
// program, which macOS would otherwise show a second time while Claude
// keeps it running.
func hideFromDock() {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		return
	}
	app := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	if app != 0 {
		const prohibited = 2 // NSApplicationActivationPolicyProhibited
		app.Send(objc.RegisterName("setActivationPolicy:"), prohibited)
	}
}
