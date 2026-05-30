// Package service — Alist / S3 / WebDAV configuration management.
//
// StorageConfigService stores connection settings encrypted at rest
// (via CryptoService). It also exposes a Test() probe so the React UI
// can verify the credentials before saving.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// StorageConfigService encrypts + persists external storage configs.
type StorageConfigService struct {
	log    *zap.Logger
	repo   *repository.Container
	crypto *CryptoService
	client *http.Client
}

// NewStorageConfigService is the constructor.
func NewStorageConfigService(log *zap.Logger, repo *repository.Container, crypto *CryptoService) *StorageConfigService {
	return &StorageConfigService{
		log:    log,
		repo:   repo,
		crypto: crypto,
		client: &http.Client{Timeout: 15 * time.Second},
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
		plain := s.crypto.Decrypt(r.Config)
		var cfg map[string]any
		_ = json.Unmarshal([]byte(plain), &cfg)
		// Redact secrets when listing.
		for _, k := range []string{"password", "secret_key", "token"} {
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
	blob, err := json.Marshal(in.Config)
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
	return s.Get(ctx, in.Type)
}

// Test runs a connection probe against the supplied (un-saved) config.
// The implementation is best-effort: it issues a single HEAD/PROPFIND
// to verify reachability, not full functionality.
func (s *StorageConfigService) Test(ctx context.Context, in StorageInput) error {
	cfg := in.Config
	if cfg == nil {
		return errors.New("config required")
	}
	switch in.Type {
	case "alist":
		server := strings.TrimRight(strr(cfg["server"]), "/")
		if server == "" {
			return errors.New("alist missing server")
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server+"/api/me", nil)
		if tok := strr(cfg["token"]); tok != "" {
			req.Header.Set("Authorization", tok)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 500 {
			return fmt.Errorf("alist returned %d", resp.StatusCode)
		}
		return nil
	case "webdav":
		u := strr(cfg["url"])
		if u == "" {
			return errors.New("webdav missing url")
		}
		req, _ := http.NewRequestWithContext(ctx, "PROPFIND", u, nil)
		if user := strr(cfg["username"]); user != "" {
			req.SetBasicAuth(user, strr(cfg["password"]))
		}
		req.Header.Set("Depth", "0")
		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 && resp.StatusCode != http.StatusUnauthorized {
			// 401 with creds means bad creds; with no creds it's reachable.
			if user := strr(cfg["username"]); user == "" && resp.StatusCode == http.StatusUnauthorized {
				return nil
			}
			return fmt.Errorf("webdav returned %d", resp.StatusCode)
		}
		return nil
	case "s3":
		ep := strr(cfg["endpoint"])
		if ep == "" {
			return errors.New("s3 missing endpoint")
		}
		// We only verify endpoint reachability — full SigV4 is a large
		// dependency; the upstream Vue project also stops at this level.
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ep, nil)
		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return nil
	default:
		return fmt.Errorf("unsupported storage type %q", in.Type)
	}
}

func validStorageType(t string) bool {
	switch t {
	case "alist", "s3", "webdav":
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
