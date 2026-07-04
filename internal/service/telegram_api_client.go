package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const defaultTelegramAPIBaseURL = "https://api.telegram.org"

var telegramTokenPattern = regexp.MustCompile(`bot[0-9]+:[^/\s"'?]+`)

func telegramAPIBaseURL(cfg map[string]string) string {
	base := strings.TrimSpace(cfg["api_base_url"])
	if base == "" {
		base = strings.TrimSpace(os.Getenv("MEDIASTATION_TELEGRAM_API_BASE_URL"))
	}
	if base == "" {
		base = defaultTelegramAPIBaseURL
	}
	return strings.TrimRight(base, "/")
}

func telegramMethodURL(cfg map[string]string, botToken, method string) (string, error) {
	botToken = strings.TrimSpace(botToken)
	method = strings.TrimSpace(method)
	if botToken == "" {
		return "", errors.New("telegram bot_token required")
	}
	if method == "" {
		return "", errors.New("telegram method required")
	}
	base := telegramAPIBaseURL(cfg)
	if _, err := url.ParseRequestURI(base); err != nil {
		return "", fmt.Errorf("telegram api_base_url invalid")
	}
	return fmt.Sprintf("%s/bot%s/%s", base, botToken, method), nil
}

func telegramHTTPClient(timeout time.Duration, cfg map[string]string) *http.Client {
	clients := telegramHTTPClients(timeout, cfg)
	return clients[0]
}

func telegramHTTPClients(timeout time.Duration, cfg map[string]string) []*http.Client {
	clients := []*http.Client{}
	seen := map[string]bool{}
	customAPIBase := telegramUsesCustomAPIBase(cfg)
	for _, proxyRaw := range telegramProxyCandidates(cfg) {
		proxyURL, err := normalizeProxyURL(proxyRaw, "http")
		if err != nil || proxyURL == nil {
			continue
		}
		key := proxyURL.String()
		if seen[key] {
			continue
		}
		seen[key] = true
		transport := NewExternalTransport()
		transport.Proxy = http.ProxyURL(proxyURL)
		clients = append(clients, &http.Client{Timeout: timeout, Transport: transport})
	}
	transport := NewExternalTransport()
	if customAPIBase {
		transport = NewInternalTransport()
	}
	clients = append(clients, &http.Client{Timeout: timeout, Transport: transport})
	return clients
}

func telegramProxyCandidates(cfg map[string]string) []string {
	out := []string{}
	for _, value := range []string{
		cfg["proxy_url"],
		os.Getenv("MEDIASTATION_TELEGRAM_PROXY_URL"),
	} {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	if len(out) > 0 {
		return out
	}
	if telegramUsesCustomAPIBase(cfg) {
		return out
	}
	for _, value := range []string{
		"http://127.0.0.1:10808",
		"http://127.0.0.1:10809",
		"http://127.0.0.1:7890",
		"http://127.0.0.1:7891",
		"http://host.docker.internal:7890",
		"http://host.docker.internal:10808",
		"http://172.17.0.1:7890",
		"http://172.17.0.1:10808",
	} {
		out = append(out, value)
	}
	return out
}

func telegramUsesCustomAPIBase(cfg map[string]string) bool {
	return telegramAPIBaseURL(cfg) != defaultTelegramAPIBaseURL
}

func sanitizeTelegramError(err error) error {
	if err == nil {
		return nil
	}
	msg := sanitizeTelegramText(err.Error())
	if strings.Contains(msg, "Client.Timeout exceeded") || strings.Contains(msg, "context deadline exceeded") {
		return errors.New("telegram request timeout: 请检查 NAS/Docker 到 Telegram API 的代理、反代或网络连通性")
	}
	return errors.New(msg)
}

func sanitizeTelegramText(text string) string {
	return telegramTokenPattern.ReplaceAllString(text, "bot<redacted>")
}
