package service

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestTelegramProxyCandidatesDefaultLocalFallbacks(t *testing.T) {
	got := telegramProxyCandidates(map[string]string{})
	joined := strings.Join(got, ",")
	for _, want := range []string{"127.0.0.1:10808", "127.0.0.1:7890", "host.docker.internal:7890", "172.17.0.1:7890"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("default proxy candidates %q missing %q", joined, want)
		}
	}
}

func TestTelegramHTTPClientsCustomAPIBaseSkipsDefaultProxyFallback(t *testing.T) {
	clients := telegramHTTPClients(time.Second, map[string]string{
		"api_base_url": "http://127.0.0.1:18080",
	})
	if len(clients) != 1 {
		t.Fatalf("clients = %d, want direct client only", len(clients))
	}
	if got := telegramClientProxyString(t, clients[0]); got != "" {
		t.Fatalf("custom api_base_url proxy = %q, want direct", got)
	}
}

func TestTelegramHTTPClientsPreferConfiguredProxy(t *testing.T) {
	clients := telegramHTTPClients(time.Second, map[string]string{
		"proxy_url": "http://proxy.example:7890",
	})
	if len(clients) == 0 {
		t.Fatal("expected telegram clients")
	}
	if got := telegramClientProxyString(t, clients[0]); got != "http://proxy.example:7890" {
		t.Fatalf("first client proxy = %q, want configured proxy", got)
	}
}

func telegramClientProxyString(t *testing.T, client *http.Client) string {
	t.Helper()
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.Proxy == nil {
		return ""
	}
	req, err := http.NewRequest(http.MethodGet, defaultTelegramAPIBaseURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL == nil {
		return ""
	}
	return proxyURL.String()
}
