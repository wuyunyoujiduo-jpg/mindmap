//go:build windows

package screenshot

import (
	"errors"
	"image"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procGetMonitorInfoW    = user32.NewProc("GetMonitorInfoW")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procBitBlt             = gdi32.NewProc("BitBlt")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
)

const (
	srcCopy    = 0x00CC0020
	captureBlt = 0x40000000
)

type rect struct{ Left, Top, Right, Bottom int32 }

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// Windows 不需要单独申请截屏权限。
func hasCapturePermission() bool { return true }
func requestCapturePermission()  {}

const capturePermissionHelp = ""

func openCapturePermissionSettings() {}

// captureDisplay 用 GDI 截下一块显示器（MyGo 的 Display.ID 就是 HMONITOR），得到物理像素的图片。
func captureDisplay(d mygo.Display) (image.Image, error) {
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(uintptr(d.ID), uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return nil, errors.New("GetMonitorInfo 失败")
	}
	r := mi.Monitor
	w, h := int(r.Right-r.Left), int(r.Bottom-r.Top)

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, errors.New("GetDC 失败")
	}
	defer procReleaseDC.Call(0, screenDC)
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, errors.New("CreateCompatibleDC 失败")
	}
	defer procDeleteDC.Call(memDC)

	// 自顶向下的 32 位 BGRA 位图。
	bi := struct {
		Header bitmapInfoHeader
		Colors [1]uint32
	}{Header: bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}}
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	var bits unsafe.Pointer
	bmp, _, _ := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return nil, errors.New("CreateDIBSection 失败")
	}
	defer procDeleteObject.Call(bmp)
	old, _, _ := procSelectObject.Call(memDC, bmp)
	defer procSelectObject.Call(memDC, old)

	if ok, _, _ := procBitBlt.Call(memDC, 0, 0, uintptr(w), uintptr(h), screenDC, uintptr(r.Left), uintptr(r.Top), srcCopy|captureBlt); ok == 0 {
		return nil, errors.New("BitBlt 失败")
	}
	src := unsafe.Slice((*byte)(bits), w*h*4)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(src); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = src[i+2], src[i+1], src[i], 0xFF
	}
	return img, nil
}

// MyGo 的 AlwaysOnTop 在 Windows 上就是 HWND_TOPMOST，后显示的窗口会盖住任务栏。
func raiseOverlay(*mygo.Window) {}
func ReturnFocus()              {}
