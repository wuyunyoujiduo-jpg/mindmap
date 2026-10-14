//go:build !darwin

package main

// watchPinch watches for pinches on macOS only; elsewhere the touchpad's
// pinch comes as Ctrl and the wheel.
func watchPinch(fn func(magnification float64)) {}
