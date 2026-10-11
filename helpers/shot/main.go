// Command shot is OneShot's screenshot, run by MyMind as a program of its
// own: a menu bar app (no Dock icon), on the MyGo version OneShot is made
// with, it frames a region at once, marks it up, and on the toolbar's
// choice copies it, or writes it to -out and says what to do with it.
//
//	shot -out /tmp/x.png
//
// The last line it prints is the outcome: "copy", "save", "library",
// "cancel", "permission" (no screen recording permission) or "error: …".
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/egoist/mygo"
)

// Action is what is done with the shot.
type Action int

const (
	Copy Action = iota
	Save
	Library
)

func (a Action) String() string {
	return [...]string{"copy", "save", "library"}[a]
}

// Log tells each step, on stderr, which MyMind keeps in its log.
func Log(format string, args ...any) { log.Printf(format, args...) }

func main() {
	out := flag.String("out", "", "where the shot is written, for save and library")
	flag.Parse()
	log.SetFlags(0)
	log.SetPrefix("shot: ")
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(func() { go run(*out) })
	if err := mygo.App.Run(); err != nil {
		fmt.Println("error:", err)
		os.Exit(2)
	}
}

func finish(outcome string, code int) {
	fmt.Println(outcome)
	os.Exit(code)
}

func run(out string) {
	if !hasCapturePermission() {
		Log("没有屏幕录制权限")
		requestCapturePermission()
		finish("permission", 3)
	}
	Log("正在截取屏幕…")
	shots, err := captureScreens()
	if err == nil && len(shots) == 0 {
		err = fmt.Errorf("没有找到屏幕")
	}
	if err != nil {
		finish("error: "+err.Error(), 2)
	}
	for _, sh := range shots {
		w, h := sh.bmp.Size()
		Log("屏幕 %d：%v，%dx%d 像素", sh.index, sh.display.Bounds, w, h)
	}
	var outcome = "cancel"
	openSession(shots, func(png []byte, act Action) {
		// The session has put a copy on the clipboard itself.
		if act != Copy {
			if err := os.WriteFile(out, png, 0o644); err != nil {
				outcome = "error: " + err.Error()
				return
			}
		}
		outcome = act.String()
	}, func() {
		code := 0
		if outcome == "cancel" {
			code = 1
		}
		finish(outcome, code)
	})
	Log("截图窗口已打开")
}
