package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// normalize 填充派生默认值并自愈空的关键字段。
func (c *Config) normalize() error {
	if c.App.DataDir == "" {
		c.App.DataDir = "./data"
	}
	if c.Database.DBPath == "" {
		c.Database.DBPath = filepath.Join(c.App.DataDir, "mediastation.db")
	}
	if c.Database.Type == "" {
		c.Database.Type = "auto"
	}
	if c.App.MaxCPUThreads < 1 {
		c.App.MaxCPUThreads = 1
	}
	if c.App.MaxCPUThreads > 8 {
		c.App.MaxCPUThreads = 8
	}
	if c.App.CloudScanMaxConcurrent < 1 {
		c.App.CloudScanMaxConcurrent = 1
	}
	if c.App.CloudScanMaxConcurrent > 16 {
		c.App.CloudScanMaxConcurrent = 16
	}
	if c.Database.MaxOpenConns <= 0 {
		c.Database.MaxOpenConns = defaultDatabaseMaxOpenConns
	}
	if c.Database.MaxIdleConns <= 0 || c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		c.Database.MaxIdleConns = defaultDatabaseMaxIdleConns
		if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
			c.Database.MaxIdleConns = c.Database.MaxOpenConns
		}
	}
	if c.Cache.CacheDir == "" {
		c.Cache.CacheDir = filepath.Join(c.App.DataDir, "cache")
	}
	if c.Cache.RedisPrefix == "" {
		c.Cache.RedisPrefix = "mediastationgo"
	}
	if c.Cache.MediaTTLSeconds < 1 {
		c.Cache.MediaTTLSeconds = 15
	}
	c.Search.Backend = strings.ToLower(strings.TrimSpace(c.Search.Backend))
	if c.Search.Index == "" {
		c.Search.Index = "mediastation_media"
	}
	if c.Secrets.JWTSecret == "" {
		// 持久化自动生成的密钥以在操作员忘记配置时保持会话稳定。
		path := filepath.Join(c.App.DataDir, ".jwt_secret")
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 { // #nosec G304 -- path is fixed to .jwt_secret under configured DataDir.
			c.Secrets.JWTSecret = strings.TrimSpace(string(data))
		} else {
			buf := make([]byte, 32)
			if _, err := rand.Read(buf); err != nil {
				return fmt.Errorf("generate jwt secret: %w", err)
			}
			c.Secrets.JWTSecret = hex.EncodeToString(buf)
			_ = os.MkdirAll(c.App.DataDir, 0o750)
			_ = os.WriteFile(path, []byte(c.Secrets.JWTSecret), 0o600)
		}
	}
	return nil
}
