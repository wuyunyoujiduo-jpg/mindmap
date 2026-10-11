//go:build windows

package main

import (
	"syscall"
)

// fileID is a file's identity, which stays when it is renamed.
type fileID struct{ dev, ino uint64 }

func idOf(path string) (fileID, bool) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fileID{}, false
	}
	h, err := syscall.CreateFile(p, 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return fileID{}, false
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return fileID{}, false
	}
	return fileID{uint64(info.VolumeSerialNumber), uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}, true
}
