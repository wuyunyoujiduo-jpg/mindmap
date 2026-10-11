//go:build !darwin && !windows

package main

import (
	"errors"
	"image"

	"github.com/egoist/mygo"
)

// 目前只支持 macOS 和 Windows；其他平台只为能编译和跑测试。

func hasCapturePermission() bool { return true }
func requestCapturePermission()  {}

const capturePermissionHelp = ""

func openCapturePermissionSettings() {}

func captureDisplay(mygo.Display) (image.Image, error) {
	return nil, errors.New("当前平台不支持截图")
}

func raiseOverlay(*mygo.Window) {}
func returnFocus()              {}
