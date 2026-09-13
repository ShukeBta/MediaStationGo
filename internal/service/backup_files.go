package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func createBackupTempFile(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	pattern := "mediastation_" + time.Now().Format("20060102_150405") + "_*.partial"
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path, pathErr := filepath.Abs(f.Name())
	if err := errors.Join(pathErr, f.Close()); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return path, nil
}

func backupFileInfo(path string) (*BackupInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("backup must be a regular file")
	}
	return &BackupInfo{Filename: filepath.Base(path), Path: path, Size: info.Size(), CreatedAt: info.ModTime(), DatabaseType: backupFilenameDatabaseType(path)}, nil
}

func validateBackupHeader(path, dialect string) error {
	if _, err := backupFileInfo(path); err != nil {
		return err
	}
	f, err := os.Open(path) // #nosec G304 -- path is constrained to the configured backup directory.
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 16)
	n, err := io.ReadFull(f, header)
	magic := []byte("SQLite format 3\x00")
	if dialect == "postgres" {
		magic = []byte("PGDMP")
	}
	if (err != nil && err != io.ErrUnexpectedEOF) || !bytes.HasPrefix(header[:n], magic) {
		return fmt.Errorf("invalid %s backup header", dialect)
	}
	return nil
}
