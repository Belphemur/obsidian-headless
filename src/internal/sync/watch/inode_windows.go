//go:build windows

package watch

import "os"

func getInode(info os.FileInfo) uint64 {
	return 0
}
