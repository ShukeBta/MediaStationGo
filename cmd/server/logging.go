package main

import (
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// newLogger 根据 cfg.Logging 构建 Zap。
func newLogger(cfg *config.Config) (*zap.Logger, error) {
	if cfg.App.Debug {
		return zap.NewDevelopment()
	}
	level := configuredLogLevel(cfg.Logging.Level)
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	var encoder zapcore.Encoder
	if strings.EqualFold(strings.TrimSpace(cfg.Logging.Format), "console") {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}
	cores := []zapcore.Core{
		zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), level),
	}
	appPath, warnPath, errorPath := logFilePaths(cfg)
	if appPath != "" {
		appWriter, err := newRotatingFileWriter(appPath, cfg.Logging)
		if err != nil {
			return nil, err
		}
		cores = append(cores, zapcore.NewCore(encoder, appWriter, level))
	}
	if warnPath != "" {
		warnWriter, err := newRotatingFileWriter(warnPath, cfg.Logging)
		if err != nil {
			return nil, err
		}
		cores = append(cores, zapcore.NewCore(encoder, warnWriter, zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl == zapcore.WarnLevel && level.Enabled(lvl)
		})))
	}
	if errorPath != "" {
		errorWriter, err := newRotatingFileWriter(errorPath, cfg.Logging)
		if err != nil {
			return nil, err
		}
		cores = append(cores, zapcore.NewCore(encoder, errorWriter, zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
			return lvl >= zapcore.ErrorLevel && level.Enabled(lvl)
		})))
	}
	return zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel), zap.ErrorOutput(zapcore.Lock(os.Stderr))), nil
}

func configuredLogLevel(raw string) zapcore.Level {
	level := zapcore.WarnLevel
	raw = strings.TrimSpace(raw)
	if raw != "" {
		var parsed zapcore.Level
		if err := parsed.UnmarshalText([]byte(raw)); err == nil {
			level = parsed
		}
	}
	return level
}

func logFilePaths(cfg *config.Config) (string, string, string) {
	out := strings.TrimSpace(cfg.Logging.OutputPath)
	if strings.EqualFold(out, "stdout") || strings.EqualFold(out, "stderr") {
		return "", "", ""
	}
	if out == "" {
		out = filepath.Join(cfg.App.DataDir, "logs")
	}
	if ext := filepath.Ext(out); ext != "" {
		base := strings.TrimSuffix(out, ext)
		return out, base + ".warn" + ext, base + ".error" + ext
	}
	return filepath.Join(out, "app.log"), filepath.Join(out, "warn.log"), filepath.Join(out, "error.log")
}
