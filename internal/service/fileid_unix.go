//go:build unix

package service

import (
	"fmt"
	"os"
	"syscall"
)

// fileIdentity returns a stable "device:inode" identifier for the file at
// path. Hardlinks to the same data share an identity, which lets the scanner
// avoid importing the same physical file twice (e.g. a seeding source kept by
// keep_seeding and its organized hardlink). ok is false when the identity
// cannot be determined.
func fileIdentity(path string) (string, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return "", false
	}
	// 单链接文件没有去重意义，避免给独立文件也打上可碰撞的标识。
	if st.Nlink < 2 {
		return "", false
	}
	return fmt.Sprintf("%d:%d", uint64(st.Dev), uint64(st.Ino)), true
}
