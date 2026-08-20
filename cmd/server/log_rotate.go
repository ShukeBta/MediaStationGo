package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

const defaultLogMaxSizeMB = 20

type rotatingFileWriter struct {
	mu         sync.Mutex
	path       string
	maxSize    int64
	maxBackups int
	maxAge     time.Duration
	file       *os.File
	size       int64
}

func newRotatingFileWriter(path string, cfg config.LoggingConfig) (*rotatingFileWriter, error) {
	if path == "" {
		return nil, fmt.Errorf("log path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	maxSizeMB := cfg.MaxSizeMB
	if maxSizeMB <= 0 {
		maxSizeMB = defaultLogMaxSizeMB
	}
	w := &rotatingFileWriter{
		path:       path,
		maxBackups: cfg.MaxBackups,
	}
	if cfg.EnableRotation {
		w.maxSize = int64(maxSizeMB) * 1024 * 1024
	}
	if w.maxBackups < 0 {
		w.maxBackups = 0
	}
	if cfg.MaxAgeDays > 0 {
		w.maxAge = time.Duration(cfg.MaxAgeDays) * 24 * time.Hour
	}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		if err := w.open(); err != nil {
			return 0, err
		}
	}
	if w.maxSize > 0 && w.size > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingFileWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

func (w *rotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	w.size = 0
	return err
}

func (w *rotatingFileWriter) open() error {
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", w.path, err)
	}
	w.file = file
	if stat, err := file.Stat(); err == nil {
		w.size = stat.Size()
	}
	return nil
}

func (w *rotatingFileWriter) rotate() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.maxBackups == 0 {
		_ = os.Remove(w.path)
		return w.open()
	}
	for i := w.maxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", w.path, i)
		newPath := fmt.Sprintf("%s.%d", w.path, i+1)
		if _, err := os.Stat(oldPath); err == nil {
			_ = os.Rename(oldPath, newPath)
		}
	}
	if _, err := os.Stat(w.path); err == nil {
		_ = os.Rename(w.path, fmt.Sprintf("%s.1", w.path))
	}
	w.pruneByAge()
	return w.open()
}

func (w *rotatingFileWriter) pruneByAge() {
	if w.maxAge <= 0 {
		return
	}
	cutoff := time.Now().Add(-w.maxAge)
	for i := 1; i <= w.maxBackups; i++ {
		path := fmt.Sprintf("%s.%d", w.path, i)
		if stat, err := os.Stat(path); err == nil && stat.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}
