package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

const resourceImportMaxResponseBytes = 8 << 20

type resourcePipelineClient interface {
	Search(context.Context, resourcePipelineSearchRequest) (resourcePipelineSearchResponse, error)
	CreateImport(context.Context, string, string, resourcePipelineCreateRequest) (resourcePipelineTask, error)
	GetImport(context.Context, string, string) (resourcePipelineTask, error)
	CancelImport(context.Context, string, string) (resourcePipelineTask, error)
	RetryImport(context.Context, string, string) (resourcePipelineTask, error)
}

type resourcePipelineManualClient interface {
	PrepareManual(context.Context, resourcePipelineManualRequest) (resourcePipelineSearchResponse, error)
}

type resourcePipelineHTTPClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type resourcePipelineError struct {
	StatusCode   int
	Code         string
	Message      string
	Capabilities ResourceSearchCapabilities
	Duplicate    *ResourceImportDuplicate
}

func (e *resourcePipelineError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("media-pipeline request failed with status %d", e.StatusCode)
}

func (e *resourcePipelineError) HTTPStatus() int {
	if e == nil {
		return 0
	}
	return e.StatusCode
}

type resourcePipelineSearchRequest struct {
	OwnerID            string `json:"owner_id"`
	Query              string `json:"query"`
	Category           string `json:"category"`
	Source             string `json:"source,omitempty"`
	Limit              int    `json:"limit"`
	SubscriptionFollow bool   `json:"subscription_follow,omitempty"`
}

type resourcePipelineManualRequest struct {
	OwnerID  string `json:"owner_id"`
	Input    string `json:"input"`
	Category string `json:"category"`
}

type resourcePipelineSearchResponse struct {
	SessionID    string                     `json:"session_id"`
	ExpiresAt    int64                      `json:"expires_at"`
	Items        []map[string]any           `json:"items"`
	Metadata     map[string]any             `json:"metadata"`
	Capabilities ResourceSearchCapabilities `json:"capabilities"`
}

type resourcePipelineCreateRequest struct {
	OwnerID            string `json:"owner_id"`
	SearchSessionID    string `json:"search_session_id"`
	CandidateID        string `json:"candidate_id"`
	Category           string `json:"category"`
	LibraryID          string `json:"library_id"`
	RootID             string `json:"root_id"`
	RootOpenListPath   string `json:"root_openlist_path"`
	Provider           string `json:"provider"`
	MediaType          string `json:"media_type"`
	ForceDuplicate     bool   `json:"force_duplicate,omitempty"`
	UpgradeMediaID     string `json:"upgrade_media_id,omitempty"`
	UpgradeScope       string `json:"upgrade_scope,omitempty"`
	KeepOldVersion     bool   `json:"keep_old_version"`
	SubscriptionFollow bool   `json:"subscription_follow,omitempty"`
	SubscriptionID     string `json:"subscription_id,omitempty"`
	WorkKey            string `json:"work_key,omitempty"`
	Season             int    `json:"season,omitempty"`
	ExistingEpisodes   []int  `json:"existing_episodes,omitempty"`
	ReservedEpisodes   []int  `json:"reserved_episodes,omitempty"`
	TargetOpenListPath string `json:"target_openlist_path,omitempty"`
	TitleClass         string `json:"title_class,omitempty"`
}

type resourcePipelineTask struct {
	ID              string         `json:"id"`
	OwnerID         string         `json:"owner_id"`
	Status          string         `json:"status"`
	Stage           string         `json:"stage"`
	Message         string         `json:"message"`
	Error           string         `json:"error"`
	MsgMediaID      string         `json:"msg_media_id"`
	MsgMediaTitle   string         `json:"msg_media_title"`
	CancelRequested bool           `json:"cancel_requested"`
	Result          map[string]any `json:"result"`
	CreatedAt       int64          `json:"created_at"`
	UpdatedAt       int64          `json:"updated_at"`
	StartedAt       int64          `json:"started_at"`
	CompletedAt     int64          `json:"completed_at"`
}

func newResourcePipelineHTTPClient(cfg config.ResourceImportConfig) (*resourcePipelineHTTPClient, error) {
	base := strings.TrimRight(strings.TrimSpace(cfg.PipelineURL), "/")
	if base == "" {
		return nil, errors.New("resource_import.pipeline_url is required")
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("resource_import.pipeline_url must be an absolute HTTP URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("resource_import.pipeline_url must use http or https")
	}
	token := strings.TrimSpace(cfg.PipelineToken)
	if token == "" {
		return nil, errors.New("resource_import.pipeline_token is required")
	}
	timeout := time.Duration(cfg.SearchTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &resourcePipelineHTTPClient{
		baseURL: base,
		token:   token,
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *resourcePipelineHTTPClient) Search(ctx context.Context, in resourcePipelineSearchRequest) (resourcePipelineSearchResponse, error) {
	var out resourcePipelineSearchResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/search", in, "", &out)
	return out, err
}

func (c *resourcePipelineHTTPClient) PrepareManual(ctx context.Context, in resourcePipelineManualRequest) (resourcePipelineSearchResponse, error) {
	var out resourcePipelineSearchResponse
	err := c.doJSON(ctx, http.MethodPost, "/v1/manual-candidates", in, "", &out)
	return out, err
}

func (c *resourcePipelineHTTPClient) CreateImport(ctx context.Context, ownerID, idempotencyKey string, in resourcePipelineCreateRequest) (resourcePipelineTask, error) {
	in.OwnerID = ownerID
	var out resourcePipelineTask
	err := c.doJSON(ctx, http.MethodPost, "/v1/imports", in, idempotencyKey, &out)
	return out, err
}

func (c *resourcePipelineHTTPClient) GetImport(ctx context.Context, ownerID, id string) (resourcePipelineTask, error) {
	var out resourcePipelineTask
	endpoint := path.Join("/v1/imports", url.PathEscape(id)) + "?owner_id=" + url.QueryEscape(ownerID)
	err := c.doJSON(ctx, http.MethodGet, endpoint, nil, "", &out)
	return out, err
}

func (c *resourcePipelineHTTPClient) CancelImport(ctx context.Context, ownerID, id string) (resourcePipelineTask, error) {
	var out resourcePipelineTask
	err := c.doJSON(ctx, http.MethodPost, path.Join("/v1/imports", url.PathEscape(id), "cancel"), map[string]string{"owner_id": ownerID}, "", &out)
	return out, err
}

func (c *resourcePipelineHTTPClient) RetryImport(ctx context.Context, ownerID, id string) (resourcePipelineTask, error) {
	var out resourcePipelineTask
	err := c.doJSON(ctx, http.MethodPost, path.Join("/v1/imports", url.PathEscape(id), "retry"), map[string]string{"owner_id": ownerID}, "", &out)
	return out, err
}

func (c *resourcePipelineHTTPClient) doJSON(ctx context.Context, method, endpoint string, body any, idempotencyKey string, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(idempotencyKey) != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, resourceImportMaxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > resourceImportMaxResponseBytes {
		return errors.New("media-pipeline response exceeds size limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeResourcePipelineError(resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode media-pipeline response: %w", err)
	}
	return nil
}

func decodeResourcePipelineError(status int, raw []byte) error {
	var payload struct {
		Error struct {
			Code         string                     `json:"code"`
			Message      string                     `json:"message"`
			Capabilities ResourceSearchCapabilities `json:"capabilities"`
			Duplicate    *struct {
				CanForce bool   `json:"can_force"`
				MediaID  string `json:"media_id"`
				Title    string `json:"title"`
				Reason   string `json:"reason"`
			} `json:"duplicate"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &payload)
	err := &resourcePipelineError{
		StatusCode:   status,
		Code:         payload.Error.Code,
		Message:      payload.Error.Message,
		Capabilities: payload.Error.Capabilities,
	}
	if payload.Error.Duplicate != nil {
		err.Duplicate = &ResourceImportDuplicate{
			CanForce: payload.Error.Duplicate.CanForce,
			MediaID:  payload.Error.Duplicate.MediaID,
			Title:    payload.Error.Duplicate.Title,
			Reason:   payload.Error.Duplicate.Reason,
		}
	}
	return err
}
