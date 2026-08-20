package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestProductionLoggerWritesConfiguredInfoToAppLogAndSplitsWarnError(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	cfg.App.DataDir = dir
	cfg.Logging.Level = "info"
	cfg.Logging.Format = "json"
	cfg.Logging.OutputPath = filepath.Join(dir, "logs")
	cfg.Logging.EnableRotation = true
	cfg.Logging.MaxSizeMB = 1
	cfg.Logging.MaxBackups = 2

	log, closeFn, err := newLoggerWithCloser(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	log.Info("info should be stored")
	log.Warn("warning only", zap.String("kind", "warn"))
	log.Error("error only", zap.String("kind", "error"))
	_ = log.Sync()

	appBytes, err := os.ReadFile(filepath.Join(dir, "logs", "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	warnBytes, err := os.ReadFile(filepath.Join(dir, "logs", "warn.log"))
	if err != nil {
		t.Fatal(err)
	}
	errorBytes, err := os.ReadFile(filepath.Join(dir, "logs", "error.log"))
	if err != nil {
		t.Fatal(err)
	}
	appLog := string(appBytes)
	warnLog := string(warnBytes)
	errorLog := string(errorBytes)
	if !strings.Contains(appLog, "info should be stored") ||
		!strings.Contains(appLog, "warning only") ||
		!strings.Contains(appLog, "error only") {
		t.Fatalf("app log should contain all enabled levels: %s", appLog)
	}
	if strings.Contains(warnLog, "info should be stored") || strings.Contains(errorLog, "info should be stored") {
		t.Fatal("split warn/error logs should not contain info")
	}
	if !strings.Contains(warnLog, "warning only") || strings.Contains(warnLog, "error only") {
		t.Fatalf("warn log not isolated: %s", warnLog)
	}
	if !strings.Contains(errorLog, "error only") || strings.Contains(errorLog, "warning only") {
		t.Fatalf("error log not isolated: %s", errorLog)
	}
}

func TestProductionLoggerDefaultsToWarnInAppLog(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{}
	cfg.App.DataDir = dir
	cfg.Logging.Format = "json"
	cfg.Logging.OutputPath = filepath.Join(dir, "logs")
	cfg.Logging.EnableRotation = true

	log, closeFn, err := newLoggerWithCloser(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	log.Info("info should stay quiet by default")
	log.Warn("warning should be stored")
	_ = log.Sync()

	appBytes, err := os.ReadFile(filepath.Join(dir, "logs", "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	appLog := string(appBytes)
	if strings.Contains(appLog, "info should stay quiet by default") {
		t.Fatalf("default logger should not store info: %s", appLog)
	}
	if !strings.Contains(appLog, "warning should be stored") {
		t.Fatalf("default logger should store warn: %s", appLog)
	}
}

func TestRotatingFileWriterCapsFileSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	writer, err := newRotatingFileWriter(path, config.LoggingConfig{
		EnableRotation: true,
		MaxSizeMB:      1,
		MaxBackups:     2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	chunk := strings.Repeat("x", 700*1024)
	if _, err := writer.Write([]byte(chunk)); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(chunk)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected rotated backup: %v", err)
	}
	_ = writer.Sync()
}
