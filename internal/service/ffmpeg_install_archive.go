package service

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

const (
	maxFFmpegZipEntryBytes = int64(2 << 30)
	maxFFmpegZipTotalBytes = int64(4 << 30)
)

// unzip 解压 ZIP 文件。
func unzip(log *zap.Logger, zipPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return err
	}
	destRoot, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	var totalWritten int64
	for _, file := range reader.File {
		if file.UncompressedSize64 > uint64(maxFFmpegZipEntryBytes) {
			return fmt.Errorf("zip entry too large: %s", file.Name)
		}
		target, err := safeZipTarget(destRoot, file.Name)
		if err != nil {
			return err
		}
		info := file.FileInfo()
		if info.Mode()&os.ModeSymlink != 0 {
			log.Warn("跳过 ZIP 符号链接", zap.String("name", file.Name))
			continue
		}
		if info.IsDir() {
			if err := os.MkdirAll(target, 0o750); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) // #nosec G304 -- target is constrained by safeZipTarget to the extraction directory.
		if err != nil {
			_ = src.Close()
			return err
		}
		written, err := io.Copy(dst, io.LimitReader(src, maxFFmpegZipEntryBytes+1))
		totalWritten += written
		if err != nil {
			_ = dst.Close()
			_ = src.Close()
			return err
		}
		if written > maxFFmpegZipEntryBytes || totalWritten > maxFFmpegZipTotalBytes {
			_ = dst.Close()
			_ = src.Close()
			return fmt.Errorf("zip content too large: %s", file.Name)
		}
		if err := dst.Close(); err != nil {
			_ = src.Close()
			return err
		}
		if err := src.Close(); err != nil {
			return err
		}
	}
	return nil
}

func safeZipTarget(destRoot, name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "\\") || filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("unsafe zip path: %s", name)
	}
	cleanName := filepath.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
	if cleanName == "." || strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) {
		return "", fmt.Errorf("unsafe zip path: %s", name)
	}
	target := filepath.Join(destRoot, cleanName)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(destRoot, targetAbs)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("unsafe zip path: %s", name)
	}
	return targetAbs, nil
}

func findFFmpegPackageRoot(root string) (string, error) {
	var ffmpegPath string
	var ffprobePath string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		switch strings.ToLower(d.Name()) {
		case "ffmpeg.exe":
			ffmpegPath = path
		case "ffprobe.exe":
			ffprobePath = path
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("扫描解压目录失败: %w", err)
	}
	if ffmpegPath == "" || ffprobePath == "" {
		return "", fmt.Errorf("解压后未找到 ffmpeg/ffprobe 可执行文件")
	}

	return filepath.Dir(filepath.Dir(ffmpegPath)), nil
}

func copyDirContents(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(srcDir, entry.Name())
		dstPath := filepath.Join(dstDir, entry.Name())
		if err := copyTree(srcPath, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func copyTree(srcPath, dstPath string) error {
	info, err := os.Stat(srcPath)
	if err != nil {
		return err
	}

	if info.IsDir() {
		if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
			return err
		}
		entries, err := os.ReadDir(srcPath)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyTree(filepath.Join(srcPath, entry.Name()), filepath.Join(dstPath, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}

	in, err := os.Open(srcPath) // #nosec G304 -- srcPath is produced by walking the validated extracted ffmpeg package tree.
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o750); err != nil {
		return err
	}

	out, err := os.Create(dstPath) // #nosec G304 -- dstPath is generated under the configured ffmpeg install directory.
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}
