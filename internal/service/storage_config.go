// Package service — external storage configuration management.
//
// StorageConfigService stores connection settings encrypted at rest
// (via CryptoService). It also exposes a Test() probe so the React UI
// can verify the credentials before saving.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// StorageConfigService encrypts + persists external storage configs.
type StorageConfigService struct {
	log           *zap.Logger
	repo          *repository.Container
	crypto        *CryptoService
	client        *http.Client
	resolveMu     sync.Mutex
	resolveCache  map[string]cloudResolveCacheEntry
	resolveFlight map[string]*cloudResolveCall
}

// NewStorageConfigService is the constructor.
func NewStorageConfigService(log *zap.Logger, repo *repository.Container, crypto *CryptoService) *StorageConfigService {
	return &StorageConfigService{
		log:           log,
		repo:          repo,
		crypto:        crypto,
		client:        &http.Client{Timeout: 120 * time.Second},
		resolveCache:  make(map[string]cloudResolveCacheEntry),
		resolveFlight: make(map[string]*cloudResolveCall),
	}
}

// StorageInput is the create / update payload accepted by the API.
// Config is a free-form map whose required keys depend on Type.
type StorageInput struct {
	Type    string         `json:"type" binding:"required"`
	Config  map[string]any `json:"config" binding:"required"`
	Enabled *bool          `json:"enabled,omitempty"`
}

// StorageView is what we return to the React UI. The actual ciphertext
// is decoded back to a map (with secret keys still redacted in the
// list endpoint via Redact).
type StorageView struct {
	model.StorageConfig
	Config map[string]any `json:"config"`
}

// Get returns the decrypted config view, or (nil, nil).
func (s *StorageConfigService) Get(ctx context.Context, kind string) (*StorageView, error) {
	row, err := s.repo.StorageConfig.Get(ctx, kind)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	plain := s.crypto.Decrypt(row.Config)
	var cfg map[string]any
	_ = json.Unmarshal([]byte(plain), &cfg)
	if cfg == nil {
		cfg = map[string]any{}
	}
	return &StorageView{StorageConfig: *row, Config: cfg}, nil
}

// List returns every config view (used by /admin/storage/status).
func (s *StorageConfigService) List(ctx context.Context) ([]StorageView, error) {
	rows, err := s.repo.StorageConfig.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]StorageView, 0, len(rows))
	for _, r := range rows {
		if !IsAdminStorageConfigurable(r.Type) {
			continue
		}
		plain := s.crypto.Decrypt(r.Config)
		var cfg map[string]any
		_ = json.Unmarshal([]byte(plain), &cfg)
		// Redact secrets when listing.
		for _, k := range []string{"password", "secret_key", "token", "cookie", "access_key"} {
			if v, ok := cfg[k]; ok && fmt.Sprint(v) != "" {
				cfg[k] = "********"
			}
		}
		out = append(out, StorageView{StorageConfig: r, Config: cfg})
	}
	return out, nil
}

// Save inserts or updates the config row.
func (s *StorageConfigService) Save(ctx context.Context, in StorageInput) (*StorageView, error) {
	if !validStorageType(in.Type) {
		return nil, fmt.Errorf("unsupported storage type %q", in.Type)
	}
	cfg := cloneStorageConfigMap(in.Config)
	if shouldPreserveStorageSecretsOnSave(in.Enabled) {
		merged, err := s.mergeExistingStorageSecrets(ctx, in.Type, cfg)
		if err != nil {
			return nil, err
		}
		cfg = merged
	}
	blob, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	cipher := s.crypto.Encrypt(string(blob))
	row := &model.StorageConfig{
		Type:    in.Type,
		Config:  cipher,
		Enabled: true,
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	if err := s.repo.StorageConfig.Upsert(ctx, row); err != nil {
		return nil, err
	}
	s.clearResolveCacheForType(in.Type)
	return s.Get(ctx, in.Type)
}

// Logout clears saved cloud login credentials, disables the storage backend,
// and removes virtual cloud libraries/media for that provider. It intentionally
// keeps non-secret connection hints such as server / WebDAV URL / timeout so
// the admin can log in again without rebuilding the form.
func (s *StorageConfigService) Logout(ctx context.Context, typ string) (*StorageView, error) {
	if !validStorageType(typ) {
		return nil, fmt.Errorf("unsupported storage type %q", typ)
	}
	if !cloud.IsCloudType(typ) {
		return nil, fmt.Errorf("not a cloud provider: %q", typ)
	}
	view, err := s.Get(ctx, typ)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, fmt.Errorf("%s storage not configured", typ)
	}
	cfg := make(map[string]any, len(view.Config))
	for k, v := range view.Config {
		if isStorageLoginSecretKey(k) || isDeprecatedStoragePlaybackKey(k) {
			continue
		}
		cfg[k] = v
	}
	enabled := false
	saved, err := s.Save(ctx, StorageInput{Type: typ, Config: cfg, Enabled: &enabled})
	if err != nil {
		return nil, err
	}
	purged, err := s.purgeCloudLibraries(ctx, typ)
	if err != nil {
		return nil, err
	}
	if s.log != nil {
		s.log.Info("storage logout cleared cloud libraries",
			zap.String("storage_type", typ),
			zap.Int("libraries_deleted", purged))
	}
	return saved, nil
}

func (s *StorageConfigService) purgeCloudLibraries(ctx context.Context, storageType string) (int, error) {
	if s == nil || s.repo == nil || s.repo.Library == nil || s.repo.Media == nil {
		return 0, nil
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return 0, fmt.Errorf("list libraries: %w", err)
	}
	var affectedLibs []string
	for _, lib := range libs {
		if mount, ok := ParseCloudLibraryMount(lib.Path); ok && mount.Provider == storageType {
			affectedLibs = append(affectedLibs, lib.ID)
		}
	}
	for _, libID := range affectedLibs {
		if err := s.repo.Media.PurgeByLibrary(ctx, libID); err != nil {
			if s.log != nil {
				s.log.Warn("purge media by library failed", zap.String("library_id", libID), zap.Error(err))
			}
			return len(affectedLibs), fmt.Errorf("purge media by library %s: %w", libID, err)
		}
	}
	for _, libID := range affectedLibs {
		if err := s.repo.Library.Delete(ctx, libID); err != nil {
			if s.log != nil {
				s.log.Warn("delete library failed", zap.String("library_id", libID), zap.Error(err))
			}
			return len(affectedLibs), fmt.Errorf("delete library %s: %w", libID, err)
		}
	}
	return len(affectedLibs), nil
}

func isStorageLoginSecretKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "cookie", "token", "username", "password", "access_key", "secret_key":
		return true
	default:
		return false
	}
}

func isDeprecatedStoragePlaybackKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "force_302", "force_proxy":
		return true
	default:
		return false
	}
}

func validStorageType(t string) bool {
	switch t {
	case "alist", "s3", "webdav", cloud.Type115, cloud.TypeCloudDrive2, cloud.TypeOpenList:
		return true
	}
	return false
}

// strr is a tiny helper to avoid importing fmt.Sprint just to coerce
// interface{} → string. (Named "strr" so it doesn't collide with the
// notify channel's `str` helper which already lives in this package.)
func strr(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// DeleteStorage 删除存储配置并清理关联数据
func (s *StorageConfigService) DeleteStorage(ctx context.Context, storageType string) error {
	// 查找配置
	cfg, err := s.repo.StorageConfig.Get(ctx, storageType)
	if err != nil || cfg == nil {
		return fmt.Errorf("storage config not found: %s", storageType)
	}

	affectedLibs, err := s.purgeCloudLibraries(ctx, storageType)
	if err != nil {
		return err
	}

	// 删除存储配置
	if err := s.repo.StorageConfig.Delete(ctx, cfg.ID); err != nil {
		return fmt.Errorf("delete storage config: %w", err)
	}

	s.log.Info("storage deleted",
		zap.String("storage_type", storageType),
		zap.Int("libraries_deleted", affectedLibs))

	return nil
}
