package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// transmissionRPCRequest 是 Transmission RPC 请求的通用结构。
type transmissionRPCRequest struct {
	Method    string                 `json:"method"`
	Arguments map[string]interface{} `json:"arguments"`
	Tag       int                    `json:"tag,omitempty"`
}

// transmissionRPCResponse 是 Transmission RPC 响应的通用结构。
type transmissionRPCResponse struct {
	Result    string                 `json:"result"`
	Arguments map[string]interface{} `json:"arguments"`
	Tag       int                    `json:"tag"`
}

// Initialize 配置并初始化 Transmission RPC 连接。
func (a *TransmissionAdapter) Initialize(ctx context.Context, cfg DownloadClientConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	endpoint, err := normalizeDownloadClientEndpoint("transmission", cfg.Host)
	if err != nil {
		return err
	}
	cfg.Host = endpoint
	a.cfg = cfg
	a.sessionID = ""
	a.tag = 0
	return a.pingLocked(ctx)
}

// Ping 测试连接。
func (a *TransmissionAdapter) Ping(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.pingLocked(ctx)
}

// pingLocked 内部 ping 实现（调用者必须持有锁）。
func (a *TransmissionAdapter) pingLocked(ctx context.Context) error {
	rpcURL, err := downloadClientRPCURL("transmission", a.cfg.Host)
	if err != nil {
		return err
	}
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodGet, rpcURL, nil)
	if err != nil {
		return err
	}
	if a.cfg.Username != "" {
		req.SetBasicAuth(a.cfg.Username, a.cfg.Password)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode == 409 {
		// 正常：需要 CSRF token
		a.sessionID = resp.Header.Get("X-Transmission-Session-Id")
		return nil
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("transmission rpc: %d", resp.StatusCode)
	}
	return nil
}

// rpcLocked 发送 RPC 请求（调用者必须持有锁）。
func (a *TransmissionAdapter) rpcLocked(ctx context.Context, method string, args map[string]interface{}) (*transmissionRPCResponse, error) {
	rpcURL, err := downloadClientRPCURL("transmission", a.cfg.Host)
	if err != nil {
		return nil, err
	}

	a.tag++
	body, err := json.Marshal(transmissionRPCRequest{
		Method:    method,
		Arguments: args,
		Tag:       a.tag,
	})
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost, rpcURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if a.sessionID != "" {
			req.Header.Set("X-Transmission-Session-Id", a.sessionID)
		}
		if a.cfg.Username != "" {
			req.SetBasicAuth(a.cfg.Username, a.cfg.Password)
		}

		resp, err := a.client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode == 409 {
			a.sessionID = resp.Header.Get("X-Transmission-Session-Id")
			continue
		}
		if resp.StatusCode >= 400 {
			raw, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("transmission rpc error: %d: %s", resp.StatusCode, string(raw))
		}

		var result transmissionRPCResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		if result.Result != "success" {
			return nil, fmt.Errorf("transmission rpc result: %s", result.Result)
		}
		return &result, nil
	}
	return nil, fmt.Errorf("transmission: failed after CSRF retry")
}
