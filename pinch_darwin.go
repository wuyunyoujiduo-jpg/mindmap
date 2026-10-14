//go:build darwin

package main

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// watchPinch calls fn with how much a pinch on the touchpad magnifies, as
// it does: MyGo does not pass the gesture on, so the app watches for it
// itself, leaving it to the window too.
func watchPinch(fn func(magnification float64)) {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
		return
	}
	const maskMagnify = 1 << 30 // NSEventMaskMagnify
	magnification := objc.RegisterName("magnification")
	handler := objc.NewBlock(func(_ objc.Block, ev objc.ID) objc.ID {
		if ev != 0 {
			fn(objc.Send[float64](ev, magnification))
		}
		return ev
	})
	objc.ID(objc.GetClass("NSEvent")).Send(objc.RegisterName("addLocalMonitorForEventsMatchingMask:handler:"),
		uint64(maskMagnify), handler)
}
