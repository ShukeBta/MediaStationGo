// Package service — file transfer strategies for the organizer.
//
// 整理媒体时支持四种转移方式：
//
//	move     移动（同盘 rename，跨盘 copy+删除源）——会移除源文件
//	copy     复制（保留源文件）
//	hardlink 硬链接（同盘零额外占用，保留源文件；做种不受影响）
//	symlink  软链接（保留源文件，指向源）
//
// 除 move 外，其余方式都保留源文件，因此 qBittorrent 等下载器仍能在原
// 路径找到数据继续做种上传。
package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// TransferMode 表示整理时文件的转移方式。
type TransferMode string

const (
	// TransferMove 移动：同盘 rename，跨盘 copy+删除源。
	TransferMove TransferMode = "move"
	// TransferCopy 复制：保留源文件。
	TransferCopy TransferMode = "copy"
	// TransferHardlink 硬链接：同盘零额外占用并保留源文件，做种不受影响。
	TransferHardlink TransferMode = "hardlink"
	// TransferSymlink 软链接：保留源文件，目标指向源。
	TransferSymlink TransferMode = "symlink"
)

// parseTransferMode 解析转移方式字符串，无法识别时回退为默认的移动。
func parseTransferMode(s string) TransferMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "copy", "复制":
		return TransferCopy
	case "hardlink", "hard", "link", "硬链接", "硬连接":
		return TransferHardlink
	case "symlink", "soft", "softlink", "软链接", "软连接", "符号链接":
		return TransferSymlink
	default:
		return TransferMove
	}
}

// keepsSource 报告该转移方式是否会保留源文件（用于做种）。
func (m TransferMode) keepsSource() bool {
	return m == TransferCopy || m == TransferHardlink || m == TransferSymlink
}

// transferFile 按指定方式把 src 转移到 dst。
// dst 已存在时一律报错，绝不覆盖（防止不同 release 改名后互相覆盖）。
func transferFile(src, dst string, mode TransferMode) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination already exists: %s", dst)
	}
	switch mode {
	case TransferCopy:
		return copyFile(src, dst)
	case TransferHardlink:
		if err := os.Link(src, dst); err != nil {
			// 跨文件系统无法硬链接，退化为复制，仍保留源文件以便继续做种。
			return copyFile(src, dst)
		}
		return nil
	case TransferSymlink:
		target := src
		if abs, err := filepath.Abs(src); err == nil {
			target = abs
		}
		return os.Symlink(target, dst)
	default: // TransferMove
		return moveFile(src, dst)
	}
}

// copyFile 流式复制 src 到 dst（保留源文件）。O_EXCL 保证不覆盖已存在目标。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, werr := io.Copy(f, in); werr != nil {
		f.Close()
		os.Remove(dst)
		return werr
	}
	return f.Close()
}
