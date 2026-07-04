package service

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func recognitionWordHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialRecognitionWordContext
	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return validateRecognitionWordURL(req.Context(), req.URL.String())
		},
	}
}

func dialRecognitionWordContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if isLocalHostname(host) {
		return nil, fmt.Errorf("recognition word URL host is not allowed: %s", host)
	}
	dialer := &net.Dialer{Timeout: 20 * time.Second}
	if ip := net.ParseIP(host); ip != nil {
		if err := validateRecognitionWordIP(host, ip); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve recognition word URL host %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("resolve recognition word URL host %s: no addresses", host)
	}
	for _, addr := range addrs {
		if err := validateRecognitionWordIP(host, addr.IP); err != nil {
			return nil, err
		}
	}
	var lastErr error
	for _, addr := range addrs {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (s *RecognitionWordsService) fetchSharedWords(ctx context.Context, rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if err := validateRecognitionWordURL(ctx, rawURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("fetch %s failed: %s", rawURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func validateRecognitionWordURL(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid recognition word URL %q: %w", rawURL, err)
	}
	if parsed.User != nil {
		return fmt.Errorf("recognition word URL must not include userinfo: %s", rawURL)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("recognition word URL must use http or https: %s", rawURL)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("recognition word URL host is required: %s", rawURL)
	}
	if isLocalHostname(host) {
		return fmt.Errorf("recognition word URL host is not allowed: %s", host)
	}
	if ip := net.ParseIP(host); ip != nil {
		return validateRecognitionWordIP(host, ip)
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve recognition word URL host %s: %w", host, err)
	}
	if len(addrs) == 0 {
		return fmt.Errorf("resolve recognition word URL host %s: no addresses", host)
	}
	for _, addr := range addrs {
		if err := validateRecognitionWordIP(host, addr.IP); err != nil {
			return err
		}
	}
	return nil
}

func isLocalHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return host == "localhost" || host == "localhost.localdomain"
}

func validateRecognitionWordIP(host string, ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("recognition word URL host %s resolved to an invalid address", host)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return fmt.Errorf("recognition word URL host %s resolved to a restricted address: %s", host, ip.String())
	}
	return nil
}
