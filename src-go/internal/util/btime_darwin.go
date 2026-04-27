//go:build darwin

package util

import (
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func setBirthTime(path string, t time.Time) error {
	var attrList unix.Attrlist
	attrList.Bitmapcount = unix.ATTR_BIT_MAP_COUNT
	attrList.Commonattr = unix.ATTR_CMN_CRTIME

	ts := unix.Timespec{Sec: t.Unix(), Nsec: int64(t.Nanosecond())}
	buf := (*[unsafe.Sizeof(ts)]byte)(unsafe.Pointer(&ts))[:]

	return unix.Setattrlist(path, &attrList, buf, 0)
}
