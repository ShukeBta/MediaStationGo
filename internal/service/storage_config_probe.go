package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// Test runs a connection probe against the supplied (un-saved) config.
// The implementation is best-effort: it issues a single HEAD/PROPFIND
// to verify reachability, not full functionality.
func (s *StorageConfigService) Test(ctx context.Context, in StorageInput) error {
	cfg := in.Config
	if cfg == nil {
		return errors.New("config required")
	}
	client := s.clientForConfig(cfg)
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
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return validateStorageProbeStatus("alist", resp.StatusCode)
	case cloud.TypeOpenList:
		p, err := cloud.New(in.Type, cfg, client)
		if err != nil {
			return err
		}
		return p.Ping(ctx)
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
		resp, err := client.Do(req)
		if err != nil {
			return decorateStorageTransportError("webdav", u, err)
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
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return nil
	case cloud.Type115, cloud.TypeCloudDrive2:
		p, err := cloud.New(in.Type, cfg, client)
		if err != nil {
			return err
		}
		return p.Ping(ctx)
	default:
		return fmt.Errorf("unsupported storage type %q", in.Type)
	}
}

func (s *StorageConfigService) clientForConfig(cfg map[string]any) *http.Client {
	if s == nil || s.client == nil {
		return &http.Client{Timeout: 120 * time.Second}
	}
	timeout := storageTimeoutFromConfig(cfg, s.client.Timeout)
	if timeout == s.client.Timeout {
		return s.client
	}
	cp := *s.client
	cp.Timeout = timeout
	return &cp
}

func storageTimeoutFromConfig(cfg map[string]any, fallback time.Duration) time.Duration {
	if fallback <= 0 {
		fallback = 120 * time.Second
	}
	raw := ""
	for _, key := range []string{"timeout_seconds", "webdav_timeout_seconds", "request_timeout_seconds"} {
		if value := strr(cfg[key]); value != "" {
			raw = value
			break
		}
	}
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil {
		if f, ferr := strconv.ParseFloat(raw, 64); ferr == nil {
			seconds = int(f)
		}
	}
	if seconds <= 0 {
		return fallback
	}
	if seconds < 5 {
		seconds = 5
	}
	if seconds > 600 {
		seconds = 600
	}
	return time.Duration(seconds) * time.Second
}

func validateStorageProbeStatus(name string, status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("%s authentication failed: http %d；请检查 Token / 用户名密码", name, status)
	}
	if status >= 300 {
		return fmt.Errorf("%s returned %d", name, status)
	}
	return nil
}

func decorateStorageTransportError(name, target string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if strings.Contains(message, "server gave HTTP response to HTTPS client") {
		return fmt.Errorf("%s: %w；当前地址使用 https://，但服务端返回 HTTP。请改用 http:// 地址；OpenList 默认 WebDAV 通常是 http://host:5244/dav/，管理页面/API 地址通常是 http://host:5244", name, err)
	}
	if strings.Contains(message, "first record does not look like a TLS handshake") {
		return fmt.Errorf("%s: %w；疑似把 HTTP 服务配置成了 https://，请检查 %s 的协议头", name, err, target)
	}
	return err
}
