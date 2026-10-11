//go:build !windows

package main

import (
	"os"
	"syscall"
)

// fileID is a file's identity, which stays when it is renamed.
type fileID struct{ dev, ino uint64 }

func idOf(path string) (fileID, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return fileID{}, false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, false
	}
	return fileID{uint64(st.Dev), uint64(st.Ino)}, true
}
