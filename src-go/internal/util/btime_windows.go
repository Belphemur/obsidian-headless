//go:build windows

package util

import (
	"syscall"
	"time"
)

func setBirthTime(path string, t time.Time) error {
	ft := syscall.NsecToFiletime(t.UnixNano())
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	h, err := syscall.CreateFile(
		pathPtr,
		syscall.FILE_WRITE_ATTRIBUTES,
		syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_READ,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	return syscall.SetFileTime(h, &ft, nil, nil)
}
