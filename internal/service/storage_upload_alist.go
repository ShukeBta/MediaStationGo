package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

type alistUploader struct {
	name     string
	server   string
	token    string
	username string
	password string
	client   *http.Client
}

func newAlistUploader(cfg map[string]any) *alistUploader {
	return newNamedAlistUploader("alist", cfg)
}

func newNamedAlistUploader(name string, cfg map[string]any) *alistUploader {
	return &alistUploader{
		name:     name,
		server:   strings.TrimRight(strr(cfg["server"]), "/"),
		token:    strr(cfg["token"]),
		username: strr(cfg["username"]),
		password: strr(cfg["password"]),
		client:   &http.Client{},
	}
}

func (a *alistUploader) ensureDir(ctx context.Context, remoteDir string) error {
	if a.server == "" {
		return fmt.Errorf("%s missing server", a.name)
	}
	if err := a.ensureToken(ctx); err != nil {
		return err
	}
	remoteDir = normalizeRemotePath(remoteDir)
	if remoteDir == "/" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(strings.Trim(remoteDir, "/"), "/") {
		current = normalizeRemotePath(path.Join(current, part))
		payload, _ := json.Marshal(map[string]string{"path": current})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.server+"/api/fs/mkdir", bytes.NewReader(payload))
		if err != nil {
			return err
		}
		a.auth(req)
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.client.Do(req)
		if err != nil {
			return decorateStorageTransportError(a.name, a.server, err)
		}
		err = a.checkJSON(resp, "alist mkdir")
		if err != nil && !isAlreadyExistsMessage(err.Error()) {
			return err
		}
	}
	return nil
}

func (a *alistUploader) exists(ctx context.Context, remotePath string) (bool, error) {
	if err := a.ensureToken(ctx); err != nil {
		return false, err
	}
	payload, _ := json.Marshal(map[string]string{"path": normalizeRemotePath(remotePath)})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.server+"/api/fs/get", bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	a.auth(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return false, decorateStorageTransportError(a.name, a.server, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode >= 200 && resp.StatusCode < 300 && out.Code == 200, nil
}

func (a *alistUploader) upload(ctx context.Context, localPath, remotePath string, size int64) error {
	if err := a.ensureToken(ctx); err != nil {
		return err
	}
	f, err := os.Open(localPath) // #nosec G304 -- localPath is selected from configured local media files before upload.
	if err != nil {
		return err
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, a.server+"/api/fs/put", f)
	if err != nil {
		return err
	}
	a.auth(req)
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("File-Path", url.PathEscape(normalizeRemotePath(remotePath)))
	resp, err := a.client.Do(req)
	if err != nil {
		return decorateStorageTransportError(a.name, a.server, err)
	}
	return a.checkJSON(resp, "alist upload")
}

func (a *alistUploader) ensureToken(ctx context.Context) error {
	if strings.TrimSpace(a.token) != "" {
		return nil
	}
	if strings.TrimSpace(a.username) == "" || a.password == "" {
		return nil
	}
	if a.server == "" {
		return fmt.Errorf("%s missing server", a.name)
	}
	payload, _ := json.Marshal(map[string]string{
		"username": a.username,
		"password": a.password,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.server+"/api/auth/login", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return decorateStorageTransportError(a.name, a.server, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s login: http %d: %s", a.name, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("%s login: decode response: %w", a.name, err)
	}
	if out.Code != 0 && out.Code != 200 {
		msg := strings.TrimSpace(out.Message)
		if msg == "" {
			msg = fmt.Sprintf("code %d", out.Code)
		}
		return fmt.Errorf("%s login: %s", a.name, msg)
	}
	a.token = strings.TrimSpace(out.Data.Token)
	if a.token == "" {
		return fmt.Errorf("%s login returned empty token", a.name)
	}
	return nil
}

func (a *alistUploader) auth(req *http.Request) {
	if a.token != "" {
		req.Header.Set("Authorization", a.token)
	}
}

func (a *alistUploader) checkJSON(resp *http.Response, op string) error {
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: http %d: %s", op, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	if out.Code != 0 && out.Code != 200 {
		return fmt.Errorf("%s: %s", op, out.Message)
	}
	return nil
}

func isAlreadyExistsMessage(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "exist") || strings.Contains(message, "已存在")
}
