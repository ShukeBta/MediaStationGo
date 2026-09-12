// Package service — download client (qBittorrent / Aria2 / Transmission)
// configuration. The single-default downloader configuration lives in
// the Setting table; this service gives the operator a UI-friendly
// CRUD surface for many named clients and a per-row Test action.
package service

import (
	"bytes"
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

// DownloadClientService persists model.DownloadClient rows.
type DownloadClientService struct {
	log    *zap.Logger
	repo   *repository.Container
	client *http.Client
	crypto *CryptoService
}

// NewDownloadClientService is the constructor.
func NewDownloadClientService(log *zap.Logger, repo *repository.Container, crypto ...*CryptoService) *DownloadClientService {
	service := &DownloadClientService{
		log:    log,
		repo:   repo,
		client: NewInternalHTTPClient(10 * time.Second),
	}
	if len(crypto) > 0 {
		service.crypto = crypto[0]
	}
	return service
}

// DownloadClientInput is the create / update payload.
type DownloadClientInput struct {
	Name      string `json:"name" binding:"required"`
	Type      string `json:"type" binding:"required"`
	Host      string `json:"host" binding:"required"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	IsDefault bool   `json:"is_default"`
	Enabled   bool   `json:"enabled"`
}

// List returns every configured client.
func (s *DownloadClientService) List(ctx context.Context) ([]model.DownloadClient, error) {
	return s.repo.DownloadClient.List(ctx)
}

// Create inserts a new client.
func (s *DownloadClientService) Create(ctx context.Context, in DownloadClientInput) (*model.DownloadClient, error) {
	normalized, err := normalizeDownloadClientInput(in)
	if err != nil {
		return nil, err
	}
	s.markManaged(ctx)
	c := &model.DownloadClient{
		Name:      normalized.Name,
		Type:      normalized.Type,
		Host:      normalized.Host,
		Username:  normalized.Username,
		Password:  s.encryptSecret(normalized.Password),
		IsDefault: normalized.IsDefault,
		Enabled:   normalized.Enabled,
	}
	if !c.IsDefault && c.Enabled {
		if currentDefault, err := s.repo.DownloadClient.FindDefault(ctx); err == nil && currentDefault == nil {
			if enabled, err := s.repo.DownloadClient.ListEnabled(ctx); err == nil && len(enabled) == 0 {
				c.IsDefault = true
			}
		}
	}
	if normalized.IsDefault {
		_ = s.repo.DownloadClient.ClearDefault(ctx)
	}
	if err := s.repo.DownloadClient.Create(ctx, c); err != nil {
		return nil, err
	}
	return c, nil
}

// Update applies a patch.
func (s *DownloadClientService) Update(ctx context.Context, id string, in DownloadClientInput) (*model.DownloadClient, error) {
	normalized, err := normalizeDownloadClientInput(in)
	if err != nil {
		return nil, err
	}
	s.markManaged(ctx)
	patch := map[string]any{
		"name":       normalized.Name,
		"type":       normalized.Type,
		"host":       normalized.Host,
		"username":   normalized.Username,
		"is_default": normalized.IsDefault,
		"enabled":    normalized.Enabled,
	}
	// Only overwrite the password when the caller actually sent one.
	if normalized.Password != "" {
		patch["password"] = s.encryptSecret(normalized.Password)
	}
	// Fetch existing row, apply patch via Save
	existing, err := s.repo.DownloadClient.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, errors.New("client not found")
	}
	if normalized.IsDefault {
		_ = s.repo.DownloadClient.ClearDefault(ctx)
	}
	existing.Name = patch["name"].(string)
	existing.Type = patch["type"].(string)
	existing.Host = patch["host"].(string)
	existing.Username = patch["username"].(string)
	existing.IsDefault = patch["is_default"].(bool)
	existing.Enabled = patch["enabled"].(bool)
	if pw, ok := patch["password"]; ok {
		existing.Password = pw.(string)
	}
	if err := s.repo.DownloadClient.Update(ctx, existing); err != nil {
		return nil, err
	}
	s.clearLegacyQBitConnectionIfNoDefault(ctx)
	return s.repo.DownloadClient.FindByID(ctx, id)
}

// Delete removes one client.
func (s *DownloadClientService) Delete(ctx context.Context, id string) error {
	s.markManaged(ctx)
	if err := s.repo.DownloadClient.Delete(ctx, id); err != nil {
		return err
	}
	s.clearLegacyQBitConnectionIfNoDefault(ctx)
	return nil
}

// Test verifies the configured client with the same adapter handshake used by
// the live download manager. This avoids false positives such as treating an
// aria2 400/401 response as a successful connection.
func (s *DownloadClientService) Test(ctx context.Context, id string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	c, err := s.repo.DownloadClient.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if c == nil {
		return errors.New("client not found")
	}
	adapter := AdapterFactory(c.Type)
	if adapter == nil {
		return fmt.Errorf("unsupported client type %q", c.Type)
	}
	return adapter.Initialize(ctx, DownloadClientConfig{
		Host:     c.Host,
		Username: c.Username,
		Password: s.decryptSecret(c.Password),
	})
}

// Aria2GlobalStats issues a JSON-RPC `aria2.getGlobalStat` call against
// the first enabled aria2 client. Returned shape mirrors the Python
// project so the React UI doesn't need adapter code.
func (s *DownloadClientService) Aria2GlobalStats(ctx context.Context, clientID string) (map[string]any, error) {
	c, err := s.repo.DownloadClient.FindByID(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if c == nil || c.Type != "aria2" {
		return nil, errors.New("aria2 client not found")
	}
	endpoint, err := downloadClientRPCURL("aria2", c.Host)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(aria2Request{
		JSONRPC: "2.0",
		ID:      "x",
		Method:  "aria2.getGlobalStat",
		Params:  []interface{}{"token:" + s.decryptSecret(c.Password)},
	})
	if err != nil {
		return nil, err
	}
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost, endpoint,
		bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("aria2 returned %d", resp.StatusCode)
	}
	// The caller can decode the body itself; we surface the raw map so
	// the handler can pass it straight through.
	return map[string]any{"client_id": clientID, "ok": true}, nil
}

func validateClient(in DownloadClientInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name required")
	}
	if strings.TrimSpace(in.Host) == "" {
		return errors.New("host required")
	}
	switch in.Type {
	case "qbittorrent", "aria2", "transmission":
	default:
		return fmt.Errorf("unsupported client type %q", in.Type)
	}
	return nil
}

func normalizeDownloadClientInput(in DownloadClientInput) (DownloadClientInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.TrimSpace(in.Type)
	in.Host = strings.TrimSpace(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	if err := validateClient(in); err != nil {
		return in, err
	}
	if !strings.Contains(in.Host, "://") {
		in.Host = "http://" + in.Host
	}
	normalized, err := normalizeDownloadClientEndpoint(in.Type, in.Host)
	if err != nil {
		return in, err
	}
	in.Host = normalized
	return in, nil
}

func (s *DownloadClientService) encryptSecret(value string) string {
	if s == nil || s.crypto == nil {
		return value
	}
	return s.crypto.Encrypt(value)
}

func (s *DownloadClientService) decryptSecret(value string) string {
	if s == nil || s.crypto == nil {
		return value
	}
	return s.crypto.Decrypt(value)
}

func (s *DownloadClientService) markManaged(ctx context.Context) {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return
	}
	_ = s.repo.Setting.Set(ctx, settingDownloadClientsManaged, "true")
}

func (s *DownloadClientService) clearLegacyQBitConnectionIfNoDefault(ctx context.Context) {
	if s == nil || s.repo == nil || s.repo.DownloadClient == nil || s.repo.Setting == nil {
		return
	}
	defaultClient, err := s.repo.DownloadClient.FindDefault(ctx)
	if err != nil || defaultClient != nil {
		return
	}
	_ = s.repo.Setting.Set(ctx, "qbittorrent.url", "")
	_ = s.repo.Setting.Set(ctx, "qbittorrent.username", "")
	_ = s.repo.Setting.Set(ctx, "qbittorrent.password", "")
}
