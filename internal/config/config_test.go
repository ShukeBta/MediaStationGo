package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadDefaults asserts that a Load on a clean working directory yields
// usable, normalized defaults.
func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.App.Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.App.Port)
	}
	if cfg.App.MaxCPUThreads != 2 {
		t.Fatalf("expected default MaxCPUThreads 2, got %d", cfg.App.MaxCPUThreads)
	}
	if cfg.Database.DBPath == "" {
		t.Fatalf("expected non-empty DBPath")
	}
	if cfg.Database.Type != "auto" {
		t.Fatalf("expected default database type auto, got %q", cfg.Database.Type)
	}
	if cfg.Logging.Level != "warn" || !cfg.Logging.EnableRotation || cfg.Logging.MaxSizeMB != 20 {
		t.Fatalf("expected warn rotating logs by default, got level=%q rotation=%v max=%d", cfg.Logging.Level, cfg.Logging.EnableRotation, cfg.Logging.MaxSizeMB)
	}
	if cfg.Database.MaxOpenConns != defaultDatabaseMaxOpenConns {
		t.Fatalf("expected default MaxOpenConns %d, got %d", defaultDatabaseMaxOpenConns, cfg.Database.MaxOpenConns)
	}
	if cfg.Cache.RedisPrefix != "mediastationgo" {
		t.Fatalf("expected default redis prefix, got %q", cfg.Cache.RedisPrefix)
	}
	if cfg.Cache.MediaTTLSeconds != 15 {
		t.Fatalf("expected default media cache ttl 15, got %d", cfg.Cache.MediaTTLSeconds)
	}
	if cfg.Cache.ImageCacheTTLHours != 30*24 || cfg.Cache.ImageCacheMaxMB != 1024 || cfg.Cache.ImageCachePruneIntervalMin != 15 {
		t.Fatalf("unexpected image cache defaults: %+v", cfg.Cache)
	}
	if cfg.Search.Index != "mediastation_media" {
		t.Fatalf("expected default search index, got %q", cfg.Search.Index)
	}
	if cfg.Database.MaxIdleConns != defaultDatabaseMaxIdleConns {
		t.Fatalf("expected default MaxIdleConns %d, got %d", defaultDatabaseMaxIdleConns, cfg.Database.MaxIdleConns)
	}
	if cfg.Secrets.JWTSecret == "" {
		t.Fatalf("expected auto-generated JWT secret")
	}
	if !cfg.Organizer.SmartClassify {
		t.Fatalf("expected organizer smart classify enabled by default")
	}
	if cfg.License.ServerURL != defaultLicenseServerURL || cfg.License.PublicKey != defaultLicensePublicKey || cfg.License.HMACSecret != "" {
		t.Fatalf("expected bundled license bridge defaults, got url=%q public_key=%q hmac=%q", cfg.License.ServerURL, cfg.License.PublicKey, cfg.License.HMACSecret)
	}
	// Re-loading must reuse the persisted secret on disk.
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("second Load() error: %v", err)
	}
	if cfg.Secrets.JWTSecret != cfg2.Secrets.JWTSecret {
		t.Fatalf("expected JWT secret to persist across Load() calls")
	}
	if _, err := os.Stat(filepath.Join(cfg.App.DataDir, ".jwt_secret")); err != nil {
		t.Fatalf("expected jwt secret file: %v", err)
	}
}

// TestEnvOverride checks that MEDIASTATION_* env vars override the defaults.
func TestEnvOverride(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Setenv("MEDIASTATION_APP_PORT", "9090")
	t.Setenv("MEDIASTATION_DATABASE_TYPE", "postgres")
	t.Setenv("MEDIASTATION_DATABASE_DSN", "postgres://msgo:secret@postgres:5432/msgo?sslmode=disable")
	t.Setenv("MEDIASTATION_CACHE_REDIS_URL", "redis://redis:6379/0")
	t.Setenv("MEDIASTATION_CACHE_MEDIA_TTL_SECONDS", "30")
	t.Setenv("MEDIASTATION_CACHE_IMAGE_CACHE_TTL_HOURS", "48")
	t.Setenv("MEDIASTATION_CACHE_IMAGE_CACHE_MAX_MB", "768")
	t.Setenv("MEDIASTATION_CACHE_IMAGE_CACHE_PRUNE_INTERVAL_MIN", "20")
	t.Setenv("MEDIASTATION_SEARCH_BACKEND", "opensearch")
	t.Setenv("MEDIASTATION_SEARCH_OPENSEARCH_URL", "http://opensearch:9200")
	t.Setenv("MEDIASTATION_LICENSE_SERVER_URL", "https://license.example.com")
	t.Setenv("MEDIASTATION_LICENSE_HMAC_SECRET", "override-secret")
	t.Setenv("MEDIASTATION_LICENSE_PUBLIC_KEY", "override-public-key")
	t.Setenv("MEDIASTATION_RESOURCE_IMPORT_ENABLED", "true")
	t.Setenv("MEDIASTATION_RESOURCE_IMPORT_PIPELINE_URL", "http://host.docker.internal:8765")
	t.Setenv("MEDIASTATION_RESOURCE_IMPORT_PIPELINE_TOKEN", "test-pipeline-token")
	t.Setenv("MEDIASTATION_RESOURCE_IMPORT_MAX_CONCURRENT", "4")
	t.Setenv("MEDIASTATION_RESOURCE_IMPORT_MAX_CONCURRENT_PER_USER", "2")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.App.Port != 9090 {
		t.Fatalf("expected port 9090 from env, got %d", cfg.App.Port)
	}
	if cfg.Database.Type != "postgres" || cfg.Database.DSN == "" {
		t.Fatalf("expected postgres database config from env, got type=%q dsn=%q", cfg.Database.Type, cfg.Database.DSN)
	}
	if cfg.Cache.RedisURL != "redis://redis:6379/0" || cfg.Cache.MediaTTLSeconds != 30 {
		t.Fatalf("expected redis cache config from env, got url=%q ttl=%d", cfg.Cache.RedisURL, cfg.Cache.MediaTTLSeconds)
	}
	if cfg.Cache.ImageCacheTTLHours != 48 || cfg.Cache.ImageCacheMaxMB != 768 || cfg.Cache.ImageCachePruneIntervalMin != 20 {
		t.Fatalf("unexpected image cache config from env: %+v", cfg.Cache)
	}
	if cfg.Search.Backend != "opensearch" || cfg.Search.OpenSearchURL != "http://opensearch:9200" {
		t.Fatalf("expected opensearch config from env, got backend=%q url=%q", cfg.Search.Backend, cfg.Search.OpenSearchURL)
	}
	if cfg.License.ServerURL != "https://license.example.com" || cfg.License.HMACSecret != "override-secret" || cfg.License.PublicKey != "override-public-key" {
		t.Fatalf("expected license config from env, got url=%q secret=%q public_key=%q", cfg.License.ServerURL, cfg.License.HMACSecret, cfg.License.PublicKey)
	}
	if !cfg.ResourceImport.Enabled || cfg.ResourceImport.PipelineURL != "http://host.docker.internal:8765" || cfg.ResourceImport.PipelineToken != "test-pipeline-token" {
		t.Fatalf("expected resource import config from env, got %+v", cfg.ResourceImport)
	}
	if cfg.ResourceImport.MaxConcurrent != 4 || cfg.ResourceImport.MaxConcurrentPerUser != 2 {
		t.Fatalf("unexpected resource import concurrency config: %+v", cfg.ResourceImport)
	}
}

func TestLoadAllowsExplicitSingleConnectionDatabaseConfig(t *testing.T) {
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.WriteFile("config.yaml", []byte("database:\n  max_open_conns: 1\n  max_idle_conns: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Database.MaxOpenConns != 1 {
		t.Fatalf("expected explicit MaxOpenConns=1 to be preserved, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 1 {
		t.Fatalf("expected explicit MaxIdleConns=1 to be preserved, got %d", cfg.Database.MaxIdleConns)
	}
}
