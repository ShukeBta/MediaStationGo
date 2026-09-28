package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var errProxyPoolUnavailable = errors.New("proxy pool configuration is unavailable")

const (
	proxyPoolHealthCheckURL     = "https://movie.douban.com/j/subject_suggest?q=mediastation-proxy-health-check"
	proxyPoolHealthCheckTimeout = 5 * time.Second
	proxyPoolHealthCheckWorkers = 200
	proxyPoolHealthMaxBodyBytes = 1 << 20
	proxyPoolCleanupTokenTTL    = 5 * time.Minute
)

// ProxyPoolItem is the credential-free administrator projection of a proxy.
type ProxyPoolItem struct {
	ID         string `json:"id"`
	DisplayURL string `json:"display_url"`
	HasAuth    bool   `json:"has_auth"`
}

// ProxyPoolInput replaces, retains, or creates one ordered proxy entry.
type ProxyPoolInput struct {
	ID  string  `json:"id,omitempty"`
	URL *string `json:"url,omitempty"`
}

// ProxyPoolConfigView is the credential-free global proxy-pool configuration.
type ProxyPoolConfigView struct {
	ProxyPoolType      string `json:"proxy_pool_type"`
	ResinProxyURL      string `json:"resin_proxy_url,omitempty"`
	ResinAccount       string `json:"resin_account,omitempty"`
	HasResinProxyToken bool   `json:"has_resin_proxy_token"`
}

// ProxyPoolConfigPatch updates the global proxy-pool mode and Resin gateway.
type ProxyPoolConfigPatch struct {
	ProxyPoolType   *string `json:"proxy_pool_type,omitempty"`
	ResinProxyURL   *string `json:"resin_proxy_url,omitempty"`
	ResinProxyToken *string `json:"resin_proxy_token,omitempty"`
	ResinAccount    *string `json:"resin_account,omitempty"`
}

type resolvedProxyPoolConfig struct {
	ProxyPoolType   string
	ResinProxyURL   string
	ResinProxyToken string
	ResinAccount    string
	Revision        uint64
}

// ProxyPoolCheckResult 汇总本次检测；清理令牌仅绑定本次确定不可用的代理。
type ProxyPoolCheckResult struct {
	Total        int    `json:"total"`
	Available    int    `json:"available"`
	Unavailable  int    `json:"unavailable"`
	Inconclusive int    `json:"inconclusive"`
	CleanupToken string `json:"cleanup_token,omitempty"`
}

// ProxyPoolCleanupResult 返回精确删除后的最新脱敏代理池。
type ProxyPoolCleanupResult struct {
	Items   []ProxyPoolItem `json:"items"`
	Removed int             `json:"removed"`
}

type proxyPoolHealthStatus uint8

const (
	proxyPoolHealthAvailable proxyPoolHealthStatus = iota
	proxyPoolHealthUnavailable
	proxyPoolHealthInconclusive
)

type proxyPoolHealthTarget struct {
	id     string
	client *http.Client
}

type proxyPoolPendingCleanup struct {
	token      string
	generation uint64
	expiresAt  time.Time
	ids        map[string]struct{}
}

type proxyPoolSnapshot struct {
	generation     uint64
	configRevision uint64
	clients        []*http.Client
	resin          *resolvedProxyPoolConfig
}

// ProxyPoolService owns encrypted proxy configuration and reusable transports.
type ProxyPoolService struct {
	repo   *repository.Container
	crypto *CryptoService

	mu             sync.Mutex
	loaded         bool
	generation     uint64
	configRevision uint64
	clients        []*http.Client
	pending        *proxyPoolPendingCleanup
}

func NewProxyPoolService(repo *repository.Container, crypto *CryptoService) *ProxyPoolService {
	return &ProxyPoolService{repo: repo, crypto: crypto}
}

// GetConfig returns the global proxy-pool configuration without its token.
func (s *ProxyPoolService) GetConfig(ctx context.Context) (ProxyPoolConfigView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config, err := s.resolveConfigLocked(ctx)
	if err != nil {
		return ProxyPoolConfigView{}, err
	}
	return publicProxyPoolConfig(config), nil
}

// UpdateConfig saves the global proxy-pool configuration atomically.
func (s *ProxyPoolService) UpdateConfig(ctx context.Context, patch ProxyPoolConfigPatch) (ProxyPoolConfigView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s == nil || s.repo == nil || s.repo.DB == nil || s.crypto == nil {
		return ProxyPoolConfigView{}, errProxyPoolUnavailable
	}

	var row model.APIConfig
	if err := s.repo.DB.WithContext(ctx).Where("provider = ?", "douban").First(&row).Error; err != nil {
		return ProxyPoolConfigView{}, err
	}
	proxyPoolType, err := normalizeProxyPoolType(row.ProxyPoolType)
	if err != nil {
		return ProxyPoolConfigView{}, err
	}
	resinProxyURL, err := normalizeResinProxyOrigin(row.ResinProxyURL)
	if err != nil {
		return ProxyPoolConfigView{}, err
	}
	resinAccount := strings.TrimSpace(row.ResinAccount)
	resinProxyToken := row.ResinProxyToken
	if patch.ProxyPoolType != nil {
		proxyPoolType, err = normalizeProxyPoolType(*patch.ProxyPoolType)
		if err != nil {
			return ProxyPoolConfigView{}, err
		}
	}
	if patch.ResinProxyURL != nil {
		resinProxyURL, err = normalizeResinProxyOrigin(*patch.ResinProxyURL)
		if err != nil {
			return ProxyPoolConfigView{}, err
		}
	}
	if patch.ResinAccount != nil {
		resinAccount = strings.TrimSpace(*patch.ResinAccount)
	}
	if utf8.RuneCountInString(resinAccount) > 128 {
		return ProxyPoolConfigView{}, errors.New("resin account must be at most 128 characters")
	}
	if patch.ResinProxyToken != nil {
		value := strings.TrimSpace(*patch.ResinProxyToken)
		switch value {
		case "":
		case "<clear>":
			resinProxyToken = ""
		default:
			resinProxyToken = s.crypto.Encrypt(value)
			if !s.crypto.IsEncrypted(resinProxyToken) {
				return ProxyPoolConfigView{}, errProxyPoolUnavailable
			}
		}
	}
	if resinProxyToken != "" && !s.crypto.IsEncrypted(resinProxyToken) {
		return ProxyPoolConfigView{}, errProxyPoolUnavailable
	}
	if proxyPoolType == ProxyPoolTypeResin && (resinProxyURL == "" || resinProxyToken == "") {
		return ProxyPoolConfigView{}, errors.New("resin proxy address and token are required")
	}
	if err := s.repo.DB.WithContext(ctx).Model(&row).Updates(map[string]any{
		"proxy_pool_type":   proxyPoolType,
		"resin_proxy_url":   resinProxyURL,
		"resin_proxy_token": resinProxyToken,
		"resin_account":     resinAccount,
	}).Error; err != nil {
		return ProxyPoolConfigView{}, err
	}
	s.configRevision++
	return ProxyPoolConfigView{
		ProxyPoolType:      proxyPoolType,
		ResinProxyURL:      resinProxyURL,
		ResinAccount:       resinAccount,
		HasResinProxyToken: resinProxyToken != "",
	}, nil
}

func (s *ProxyPoolService) resolveConfig(ctx context.Context) (resolvedProxyPoolConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resolveConfigLocked(ctx)
}

func (s *ProxyPoolService) resolveConfigLocked(ctx context.Context) (resolvedProxyPoolConfig, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil || s.crypto == nil {
		return resolvedProxyPoolConfig{}, errProxyPoolUnavailable
	}
	var row model.APIConfig
	if err := s.repo.DB.WithContext(ctx).Where("provider = ?", "douban").First(&row).Error; err != nil {
		return resolvedProxyPoolConfig{}, err
	}
	proxyPoolType, err := normalizeProxyPoolType(row.ProxyPoolType)
	if err != nil {
		return resolvedProxyPoolConfig{}, err
	}
	resinProxyURL, err := normalizeResinProxyOrigin(row.ResinProxyURL)
	if err != nil {
		return resolvedProxyPoolConfig{}, err
	}
	resinAccount := strings.TrimSpace(row.ResinAccount)
	if utf8.RuneCountInString(resinAccount) > 128 {
		return resolvedProxyPoolConfig{}, errProxyPoolUnavailable
	}
	config := resolvedProxyPoolConfig{
		ProxyPoolType: proxyPoolType,
		ResinProxyURL: resinProxyURL,
		ResinAccount:  resinAccount,
		Revision:      s.configRevision,
	}
	if row.ResinProxyToken != "" {
		if !s.crypto.IsEncrypted(row.ResinProxyToken) {
			return resolvedProxyPoolConfig{}, errProxyPoolUnavailable
		}
		config.ResinProxyToken = s.crypto.Decrypt(row.ResinProxyToken)
		if config.ResinProxyToken == row.ResinProxyToken {
			return resolvedProxyPoolConfig{}, errProxyPoolUnavailable
		}
	}
	return config, nil
}

func publicProxyPoolConfig(config resolvedProxyPoolConfig) ProxyPoolConfigView {
	return ProxyPoolConfigView{
		ProxyPoolType:      config.ProxyPoolType,
		ResinProxyURL:      config.ResinProxyURL,
		ResinAccount:       config.ResinAccount,
		HasResinProxyToken: config.ResinProxyToken != "",
	}
}

// List returns the ordered proxy list without authentication information.
func (s *ProxyPoolService) List(ctx context.Context) ([]ProxyPoolItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.loadRows(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]ProxyPoolItem, 0, len(rows))
	for i := range rows {
		u, err := s.decryptURL(rows[i].URL)
		if err != nil {
			return nil, err
		}
		items = append(items, publicProxyPoolItem(rows[i].ID, u))
	}
	return items, nil
}

// Replace atomically saves the complete ordered proxy list.
func (s *ProxyPoolService) Replace(ctx context.Context, input []ProxyPoolInput) ([]ProxyPoolItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.replaceLocked(ctx, input, true)
}

func (s *ProxyPoolService) replaceLocked(ctx context.Context, input []ProxyPoolInput, deduplicateURLs bool) ([]ProxyPoolItem, error) {
	existingRows, err := s.loadRows(ctx)
	if err != nil {
		return nil, err
	}
	existing := make(map[string]model.ProxyPoolEntry, len(existingRows))
	for _, row := range existingRows {
		existing[row.ID] = row
	}

	rows := make([]model.ProxyPoolEntry, 0, len(input))
	parsedURLs := make([]*url.URL, 0, len(input))
	seenIDs := make(map[string]struct{}, len(input))
	seenURLs := make(map[string]struct{}, len(input))
	for i, item := range input {
		id := strings.TrimSpace(item.ID)
		row, found := existing[id]
		if id != "" {
			if !found {
				return nil, proxyPoolInputError(i, "unknown id")
			}
			if _, duplicate := seenIDs[id]; duplicate {
				return nil, proxyPoolInputError(i, "duplicate id")
			}
			seenIDs[id] = struct{}{}
		} else {
			row = model.ProxyPoolEntry{}
		}

		var parsed *url.URL
		if item.URL == nil {
			if id == "" {
				return nil, proxyPoolInputError(i, "url is required")
			}
			parsed, err = s.decryptURL(row.URL)
		} else {
			parsed, err = normalizeProxyPoolURL(*item.URL)
		}
		if err != nil {
			return nil, proxyPoolInputError(i, err.Error())
		}
		normalized := parsed.String()
		if deduplicateURLs {
			if _, duplicate := seenURLs[normalized]; duplicate {
				continue
			}
			seenURLs[normalized] = struct{}{}
		}
		if item.URL != nil {
			row.URL, err = s.encryptURL(normalized)
			if err != nil {
				return nil, proxyPoolInputError(i, err.Error())
			}
		}
		row.Position = len(rows)
		rows = append(rows, row)
		parsedURLs = append(parsedURLs, parsed)
	}

	clients := make([]*http.Client, 0, len(parsedURLs))
	for _, parsed := range parsedURLs {
		transport := NewExternalTransport()
		transport.Proxy = http.ProxyURL(parsed)
		clients = append(clients, &http.Client{Transport: transport})
	}

	if err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&model.ProxyPoolEntry{}).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(&rows, 500).Error
	}); err != nil {
		closeProxyPoolClients(clients)
		return nil, err
	}

	oldClients := s.clients
	s.generation++
	s.clients = clients
	s.loaded = true
	s.pending = nil
	closeProxyPoolClients(oldClients)

	items := make([]ProxyPoolItem, 0, len(rows))
	for i := range rows {
		items = append(items, publicProxyPoolItem(rows[i].ID, parsedURLs[i]))
	}
	return items, nil
}

// Check 检测已保存代理；检测目标异常时不返回任何可删除代理。
func (s *ProxyPoolService) Check(ctx context.Context) (ProxyPoolCheckResult, error) {
	return s.check(ctx, probeProxyPoolHealth)
}

func (s *ProxyPoolService) check(ctx context.Context, probe func(context.Context, *http.Client) proxyPoolHealthStatus) (ProxyPoolCheckResult, error) {
	direct := NewExternalHTTPClient(proxyPoolHealthCheckTimeout)
	if probe(ctx, direct) != proxyPoolHealthAvailable {
		if err := ctx.Err(); err != nil {
			return ProxyPoolCheckResult{}, err
		}
		return ProxyPoolCheckResult{}, errors.New("proxy health check target unavailable")
	}

	if _, err := s.snapshot(ctx); err != nil {
		return ProxyPoolCheckResult{}, err
	}
	targets, generation, err := s.healthTargets(ctx)
	if err != nil {
		return ProxyPoolCheckResult{}, err
	}
	defer func() {
		for _, target := range targets {
			target.client.CloseIdleConnections()
		}
	}()

	result := ProxyPoolCheckResult{Total: len(targets)}
	unavailableIDs := make([]string, 0)
	jobs := make(chan proxyPoolHealthTarget, len(targets))
	statuses := make(chan struct {
		id     string
		status proxyPoolHealthStatus
	}, len(targets))
	for _, target := range targets {
		jobs <- target
	}
	close(jobs)

	workers := min(proxyPoolHealthCheckWorkers, len(targets))
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for target := range jobs {
				if ctx.Err() != nil {
					return
				}
				statuses <- struct {
					id     string
					status proxyPoolHealthStatus
				}{id: target.id, status: probe(ctx, target.client)}
			}
		}()
	}
	wg.Wait()
	close(statuses)
	if err := ctx.Err(); err != nil {
		return ProxyPoolCheckResult{}, err
	}
	for checked := range statuses {
		switch checked.status {
		case proxyPoolHealthAvailable:
			result.Available++
		case proxyPoolHealthUnavailable:
			result.Unavailable++
			unavailableIDs = append(unavailableIDs, checked.id)
		default:
			result.Inconclusive++
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = nil
	if s.generation != generation {
		return ProxyPoolCheckResult{}, errors.New("proxy pool changed during health check")
	}
	if len(unavailableIDs) > 0 {
		result.CleanupToken = uuid.NewString()
		ids := make(map[string]struct{}, len(unavailableIDs))
		for _, id := range unavailableIDs {
			ids[id] = struct{}{}
		}
		s.pending = &proxyPoolPendingCleanup{
			token: result.CleanupToken, generation: generation,
			expiresAt: time.Now().Add(proxyPoolCleanupTokenTTL), ids: ids,
		}
	}
	return result, nil
}

func (s *ProxyPoolService) healthTargets(ctx context.Context) ([]proxyPoolHealthTarget, uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.loadRows(ctx)
	if err != nil {
		return nil, 0, err
	}
	targets := make([]proxyPoolHealthTarget, 0, len(rows))
	for i := range rows {
		parsed, err := s.decryptURL(rows[i].URL)
		if err != nil {
			for _, target := range targets {
				target.client.CloseIdleConnections()
			}
			return nil, 0, err
		}
		transport := NewExternalTransport()
		transport.Proxy = http.ProxyURL(parsed)
		targets = append(targets, proxyPoolHealthTarget{
			id: rows[i].ID, client: &http.Client{Timeout: proxyPoolHealthCheckTimeout, Transport: transport},
		})
	}
	return targets, s.generation, nil
}

// Cleanup 使用一次性检测令牌删除确定不可用代理，并保留其他代理及其加密凭据。
func (s *ProxyPoolService) Cleanup(ctx context.Context, token string) (ProxyPoolCleanupResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token = strings.TrimSpace(token)
	if token == "" || s.pending == nil || token != s.pending.token || time.Now().After(s.pending.expiresAt) || s.generation != s.pending.generation {
		return ProxyPoolCleanupResult{}, errors.New("proxy cleanup token is invalid or expired")
	}
	remove := s.pending.ids
	rows, err := s.loadRows(ctx)
	if err != nil {
		return ProxyPoolCleanupResult{}, err
	}
	retained := make([]ProxyPoolInput, 0, len(rows))
	removed := 0
	for i := range rows {
		if _, found := remove[rows[i].ID]; found {
			removed++
			continue
		}
		retained = append(retained, ProxyPoolInput{ID: rows[i].ID})
	}
	items, err := s.replaceLocked(ctx, retained, false)
	if err != nil {
		return ProxyPoolCleanupResult{}, err
	}
	return ProxyPoolCleanupResult{Items: items, Removed: removed}, nil
}

func probeProxyPoolHealth(ctx context.Context, client *http.Client) proxyPoolHealthStatus {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyPoolHealthCheckURL, nil)
	if err != nil || client == nil {
		return proxyPoolHealthUnavailable
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/125.0 Safari/537.36")
	req.Header.Set("Referer", "https://movie.douban.com/")
	resp, err := client.Do(req)
	if err != nil {
		return proxyPoolHealthUnavailable
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, proxyPoolHealthMaxBodyBytes+1))
	_ = resp.Body.Close()
	if readErr != nil {
		if errors.Is(readErr, io.ErrUnexpectedEOF) {
			return proxyPoolHealthUnavailable
		}
		return proxyPoolHealthInconclusive
	}
	if len(body) > proxyPoolHealthMaxBodyBytes {
		return proxyPoolHealthInconclusive
	}
	switch resp.StatusCode {
	case http.StatusOK:
		if json.Valid(body) {
			return proxyPoolHealthAvailable
		}
		return proxyPoolHealthUnavailable
	case http.StatusBadRequest, http.StatusProxyAuthRequired:
		return proxyPoolHealthUnavailable
	default:
		return proxyPoolHealthInconclusive
	}
}

func (s *ProxyPoolService) snapshot(ctx context.Context) (proxyPoolSnapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		rows, err := s.loadRows(ctx)
		if err != nil {
			return proxyPoolSnapshot{}, err
		}
		clients := make([]*http.Client, 0, len(rows))
		for i := range rows {
			parsed, err := s.decryptURL(rows[i].URL)
			if err != nil {
				closeProxyPoolClients(clients)
				return proxyPoolSnapshot{}, err
			}
			transport := NewExternalTransport()
			transport.Proxy = http.ProxyURL(parsed)
			clients = append(clients, &http.Client{Transport: transport})
		}
		s.generation++
		s.clients = clients
		s.loaded = true
	}
	return proxyPoolSnapshot{
		generation: s.generation,
		clients:    append([]*http.Client(nil), s.clients...),
	}, nil
}

func (s *ProxyPoolService) loadRows(ctx context.Context) ([]model.ProxyPoolEntry, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil || s.crypto == nil {
		return nil, errProxyPoolUnavailable
	}
	var rows []model.ProxyPoolEntry
	if err := s.repo.DB.WithContext(ctx).Order("position asc, created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *ProxyPoolService) decryptURL(ciphertext string) (*url.URL, error) {
	if s.crypto == nil || !s.crypto.IsEncrypted(ciphertext) {
		return nil, errProxyPoolUnavailable
	}
	plain := s.crypto.Decrypt(ciphertext)
	if plain == ciphertext {
		return nil, errProxyPoolUnavailable
	}
	return normalizeProxyPoolURL(plain)
}

func (s *ProxyPoolService) encryptURL(plain string) (string, error) {
	if s.crypto == nil {
		return "", errProxyPoolUnavailable
	}
	ciphertext := s.crypto.Encrypt(plain)
	if !s.crypto.IsEncrypted(ciphertext) {
		return "", errProxyPoolUnavailable
	}
	return ciphertext, nil
}

func normalizeProxyPoolURL(raw string) (*url.URL, error) {
	u, err := normalizeProxyURL(raw, "http")
	if err != nil || u == nil || u.Hostname() == "" {
		return nil, errors.New("invalid proxy url")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errors.New("unsupported proxy protocol")
	}
	if u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("proxy url must contain only scheme, credentials, host, and port")
	}
	u.Path = ""
	u.RawPath = ""
	return u, nil
}

func publicProxyPoolItem(id string, u *url.URL) ProxyPoolItem {
	publicURL := *u
	hasAuth := publicURL.User != nil
	publicURL.User = nil
	return ProxyPoolItem{ID: id, DisplayURL: publicURL.String(), HasAuth: hasAuth}
}

func proxyPoolInputError(index int, reason string) error {
	return fmt.Errorf("proxy %d: %s", index+1, reason)
}

func closeProxyPoolClients(clients []*http.Client) {
	for _, client := range clients {
		client.CloseIdleConnections()
	}
}
