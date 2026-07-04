package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// aria2Request 是 Aria2 JSON-RPC 请求结构。
type aria2Request struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	ID      string        `json:"id"`
	Params  []interface{} `json:"params"`
}

// aria2Response 是 Aria2 JSON-RPC 响应结构。
type aria2Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *aria2Error     `json:"error"`
}

// aria2Error 是 Aria2 JSON-RPC 错误结构。
type aria2Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Initialize 配置并初始化 Aria2 RPC 连接。
func (a *Aria2Adapter) Initialize(ctx context.Context, cfg DownloadClientConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	endpoint, err := normalizeDownloadClientEndpoint("aria2", cfg.Host)
	if err != nil {
		return err
	}
	cfg.Host = endpoint
	a.cfg = cfg
	a.idSeq = 0
	return a.getVersionLocked(ctx)
}

// Ping 测试连接。
func (a *Aria2Adapter) Ping(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.getVersionLocked(ctx)
}

// getVersionLocked 内部版本检查（调用者必须持有锁）。
func (a *Aria2Adapter) getVersionLocked(ctx context.Context) error {
	rpcURL, err := downloadClientRPCURL("aria2", a.cfg.Host)
	if err != nil {
		return err
	}

	req := &aria2Request{
		JSONRPC: "2.0",
		Method:  "aria2.getVersion",
		ID:      a.nextID(),
		Params:  []interface{}{"token:" + a.cfg.Password},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := newDownloadClientHTTPRequest(ctx, http.MethodPost, rpcURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.cfg.Username != "" {
		httpReq.SetBasicAuth(a.cfg.Username, a.cfg.Password)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("aria2 rpc: %d", resp.StatusCode)
	}
	return nil
}

// rpcLocked 发送 JSON-RPC 请求（调用者必须持有锁）。
func (a *Aria2Adapter) rpcLocked(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	rpcURL, err := downloadClientRPCURL("aria2", a.cfg.Host)
	if err != nil {
		return nil, err
	}

	if params == nil {
		params = []interface{}{}
	}

	// 如果 secret 不在 params 中，添加到第一位
	if len(params) > 0 {
		if secret, ok := params[0].(string); ok && strings.HasPrefix(secret, "token:") {
			// 已经有 secret
		} else {
			newParams := make([]interface{}, 0, len(params)+1)
			newParams = append(newParams, "token:"+a.cfg.Password)
			newParams = append(newParams, params...)
			params = newParams
		}
	} else {
		params = []interface{}{"token:" + a.cfg.Password}
	}

	req := &aria2Request{
		JSONRPC: "2.0",
		Method:  method,
		ID:      a.nextID(),
		Params:  params,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := newDownloadClientHTTPRequest(ctx, http.MethodPost, rpcURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if a.cfg.Username != "" {
		httpReq.SetBasicAuth(a.cfg.Username, a.cfg.Password)
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rpcResp aria2Response
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, err
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("aria2 rpc error [%d]: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

// nextID 生成递增的请求 ID。
func (a *Aria2Adapter) nextID() string {
	a.idSeq++
	return fmt.Sprintf("msg-%d", a.idSeq)
}
