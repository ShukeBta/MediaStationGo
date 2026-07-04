package cloud

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (p *cloudDrive2Provider) decorateDAVStatusError(resp *http.Response, target string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := compactDAVErrorBody(string(body))
	if detail == "" {
		if resp.StatusCode == http.StatusMethodNotAllowed {
			return fmt.Errorf("%s: list %s returned http %d；请确认填写的是 WebDAV 地址（通常以 /dav 结尾），并且桥接网盘已在 OpenList/CloudDrive2 内完成登录或 Cookie 保存", p.name, target, resp.StatusCode)
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("%s: list %s returned http %d；请填写 OpenList/CloudDrive2 的 Token 或用户名密码，或确认 WebDAV 凭据可用", p.name, target, resp.StatusCode)
		}
		return fmt.Errorf("%s: list %s returned http %d", p.name, target, resp.StatusCode)
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return fmt.Errorf("%s: list %s returned http %d：%s；请确认填写的是 WebDAV 地址（通常以 /dav 结尾），并且桥接网盘已在 OpenList/CloudDrive2 内完成登录或 Cookie 保存", p.name, target, resp.StatusCode, detail)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%s: list %s returned http %d：%s；请检查 WebDAV 用户名/密码、Authorization Token，或先在 OpenList/CloudDrive2 中保存对应网盘 Cookie", p.name, target, resp.StatusCode, detail)
	}
	return fmt.Errorf("%s: list %s returned http %d：%s", p.name, target, resp.StatusCode, detail)
}

func compactDAVErrorBody(raw string) string {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\x00", ""))
	if raw == "" {
		return ""
	}
	raw = strings.Join(strings.Fields(raw), " ")
	if len([]rune(raw)) > 180 {
		return string([]rune(raw)[:180]) + "…"
	}
	return raw
}

func decorateDAVTransportError(name, target string, err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if strings.Contains(message, "server gave HTTP response to HTTPS client") {
		return fmt.Errorf("%s: %w；当前地址使用 https://，但服务端返回 HTTP。请改用 http:// 地址，例如 OpenList 默认 WebDAV 通常是 http://host:5244/dav/；如果必须使用 https，请在 OpenList 前配置反向代理和证书", name, err)
	}
	if strings.Contains(message, "first record does not look like a TLS handshake") {
		return fmt.Errorf("%s: %w；疑似把 HTTP 服务配置成了 https://，请检查 %s 的协议头", name, err, target)
	}
	return err
}
