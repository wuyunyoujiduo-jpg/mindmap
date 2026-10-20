//go:build darwin

package screenshot

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// 屏幕录制权限的检查与申请（CoreGraphics，macOS 10.15+）。
var cgPreflight, cgRequest func() bool

func init() {
	lib, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return
	}
	purego.RegisterLibFunc(&cgPreflight, lib, "CGPreflightScreenCaptureAccess")
	purego.RegisterLibFunc(&cgRequest, lib, "CGRequestScreenCaptureAccess")
}

func hasCapturePermission() bool { return cgPreflight == nil || cgPreflight() }

func requestCapturePermission() {
	if cgRequest != nil {
		cgRequest()
	}
}

const capturePermissionHelp = "在「系统设置 › 隐私与安全性 › 录屏与系统录音」里打开 MyMind，然后退出并重新打开 MyMind。\n\n" +
	"每装一个新版本都要重新允许一次：如果列表里的 MyMind 已经是打开的，先选中它点「−」删掉，再点「＋」把「应用程序」里的 MyMind 加进去。"

func openCapturePermissionSettings() {
	_ = mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture")
}

// captureDisplay 用系统自带的 screencapture 截下一块屏幕，得到原始像素的图片。
func captureDisplay(d mygo.Display) (image.Image, error) {
	f, err := os.CreateTemp("", "mindmap-shot-*.png")
	if err != nil {
		return nil, err
	}
	f.Close()
	defer os.Remove(f.Name())
	b := d.Bounds
	// 区域紧跟 -R 写在一起：副屏在主屏左边时 x 为负数，分开写像一个选项。
	region := fmt.Sprintf("-R%d,%d,%d,%d", b.X, b.Y, b.Width, b.Height)
	if out, err := exec.Command("/usr/sbin/screencapture", "-x", "-t", "png", region, f.Name()).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("screencapture: %v %s", err, out)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(data))
}

// NSWindow 层级与集合行为：盖住菜单栏和程序坞，出现在当前桌面（含全屏应用）。
const (
	nsScreenSaverWindowLevel = 1000
	canJoinAllSpaces         = 1 << 0
	fullScreenAuxiliary      = 1 << 8
	fullScreenNone           = 1 << 9
	// 样式：去掉 titled（1 << 0）。带标题栏的窗口（无边框也是隐藏了标题栏而已）
	// 被 macOS 限制在菜单栏下面，菜单栏就框不到；无标题栏的才能盖满整块屏幕。
	styleTitled             = 1 << 0
	styleNonactivatingPanel = 1 << 7
)

// raiseOverlay 把选区窗口提到菜单栏之上。MyGo 的 AlwaysOnTop 只到浮动层，低于菜单栏。主线程调用。
func raiseOverlay(w *mygo.Window) {
	win := objc.ID(w.NativeHandle())
	if win == 0 {
		return
	}
	style := objc.Send[uint](win, objc.RegisterName("styleMask"))
	win.Send(objc.RegisterName("setStyleMask:"), style&styleNonactivatingPanel&^styleTitled)
	win.Send(objc.RegisterName("setLevel:"), nsScreenSaverWindowLevel)
	behavior := objc.Send[uint](win, objc.RegisterName("collectionBehavior"))
	win.Send(objc.RegisterName("setCollectionBehavior:"), behavior&^fullScreenNone|canJoinAllSpaces|fullScreenAuxiliary)
}
