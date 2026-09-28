package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

// Mirrors the older api_configs schema that did not have transport controls.
type previousAPIConfigSchema struct {
	model.Base
	Provider     string `gorm:"size:64;uniqueIndex;not null"`
	APIKey       string `gorm:"size:512"`
	BaseURL      string `gorm:"size:512"`
	Extra        string `gorm:"type:text"`
	Enabled      bool   `gorm:"default:true"`
	Description  string `gorm:"size:255"`
	LastTestedAt *time.Time
	TestResult   string `gorm:"size:32"`
}

func (previousAPIConfigSchema) TableName() string { return "api_configs" }

func TestAPIConfigFullMigrationAndSeedDefaults(t *testing.T) {
	registered := 0
	for _, entry := range model.AllModels() {
		if _, ok := entry.(*model.APIConfig); ok {
			registered++
		}
	}
	if registered != 1 {
		t.Fatalf("api_configs registered %d times", registered)
	}
	for _, legacy := range []bool{false, true} {
		name := "fresh"
		if legacy {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Database.Type = "sqlite"
			cfg.Database.DBPath = filepath.Join(t.TempDir(), "api-config.db")
			db, err := database.Open(cfg, zap.NewNop())
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = sqlDB.Close() })
			crypto := NewCryptoService("schema-test-key", zap.NewNop())
			testedAt := time.Date(2026, 9, 28, 6, 0, 0, 0, time.UTC)
			encrypted := crypto.Encrypt(strings.Repeat("test-value-", 80))
			if legacy {
				if err := db.AutoMigrate(&previousAPIConfigSchema{}); err != nil {
					t.Fatal(err)
				}
				existing := previousAPIConfigSchema{Provider: "openai", APIKey: encrypted, Extra: `{"model":"saved-model","protocol":"openai"}`, LastTestedAt: &testedAt, TestResult: "success"}
				if err := db.Create(&existing).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := database.AutoMigrate(db); err != nil {
				t.Fatalf("full initial migration: %v", err)
			}
			api := NewAPIConfigService(zap.NewNop(), repository.New(db), crypto)
			if err := api.SeedDefaults(t.Context()); err != nil {
				t.Fatalf("seed after full migration: %v", err)
			}
			for _, column := range []string{"image_direct", "use_proxy_pool", "proxy_pool_type", "resin_proxy_url", "resin_proxy_token", "resin_account", "last_tested_at", "test_result"} {
				if !db.Migrator().HasColumn(&model.APIConfig{}, column) {
					t.Fatalf("missing api_configs.%s", column)
				}
			}
			if err := database.AutoMigrate(db); err != nil {
				t.Fatalf("repeat migration: %v", err)
			}
			if err := api.SeedDefaults(t.Context()); err != nil {
				t.Fatalf("repeat defaults: %v", err)
			}
			var total int64
			if err := db.Model(&model.APIConfig{}).Count(&total).Error; err != nil || total != 10 {
				t.Fatalf("seed count=%d err=%v", total, err)
			}
			if legacy {
				legacyView, err := repository.New(db).ApiConfig.FindByProvider(t.Context(), "openai")
				if err != nil || legacyView == nil || legacyView.APIKey != encrypted || legacyView.LastTestedAt == nil || !legacyView.LastTestedAt.Equal(testedAt) || legacyView.TestResult != "success" {
					t.Fatal("existing API config or connection test metadata changed")
				}
				resolved, err := api.Resolve(t.Context(), "openai")
				if err != nil || resolved.Model != "saved-model" || resolved.APIKey != strings.Repeat("test-value-", 80) {
					t.Fatal("stored model or encrypted API key did not survive migration")
				}
			}
			if err := db.Model(&model.APIConfig{}).Where("provider = ?", "tmdb").Updates(map[string]any{"image_direct": true, "use_proxy_pool": true, "proxy_pool_type": "resin", "resin_account": "test-account"}).Error; err != nil {
				t.Fatal(err)
			}
			if err := repository.New(db).ApiConfig.UpdateTestResult(t.Context(), "tmdb", "success"); err != nil {
				t.Fatal(err)
			}
			var current model.APIConfig
			if err := db.Where("provider = ?", "tmdb").First(&current).Error; err != nil {
				t.Fatal(err)
			}
			if !current.ImageDirect || !current.UseProxyPool || current.ResinAccount != "test-account" || current.LastTestedAt == nil || current.TestResult != "success" {
				t.Fatal("compatibility write discarded provider transport settings")
			}
		})
	}
}
