package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ProxySTRM proxies a STRM target and preserves Range requests for players.
func (s *STRMService) ProxySTRM(ctx context.Context, id string, req *http.Request, w http.ResponseWriter) error {
	record, err := s.repo.STRM.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if record == nil {
		return ErrSTRMNotFound
	}
	if !model.IsAllowedProtocol(record.Protocol) {
		return ErrSTRMProtocolInvalid
	}

	targetURL, err := validateSTRMProxyURL(record.URL)
	if err != nil {
		return err
	}
	proxyReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL.String(), nil)
	if err != nil {
		return fmt.Errorf("create proxy request: %w", err)
	}
	copySTRMRequestHeaders(req, proxyReq)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(proxyReq) // #nosec G107,G704 -- STRM proxy target is validated by validateSTRMProxyURL before request creation.
	if err != nil {
		return fmt.Errorf("proxy request failed: %w", err)
	}
	defer resp.Body.Close()

	copySTRMResponseHeaders(resp, w)
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	return err
}

func validateSTRMProxyURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, ErrSTRMURLInvalid
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return nil, ErrSTRMProtocolInvalid
	}
	if isPrivateHost(u.Hostname()) {
		return nil, ErrSTRMURLInvalid
	}
	return u, nil
}

func copySTRMRequestHeaders(src *http.Request, dst *http.Request) {
	for _, header := range []string{
		"Range", "If-Range", "If-Match", "If-None-Match",
		"If-Modified-Since", "If-Unmodified-Since",
		"Accept", "Accept-Encoding", "Accept-Language",
	} {
		if v := src.Header.Get(header); v != "" {
			dst.Header.Set(header, v)
		}
	}
}

func copySTRMResponseHeaders(src *http.Response, dst http.ResponseWriter) {
	for _, header := range []string{
		"Content-Type", "Content-Length", "Content-Range",
		"Accept-Ranges", "Last-Modified", "ETag",
		"Cache-Control", "Content-Disposition",
	} {
		if v := src.Header.Get(header); v != "" {
			dst.Header().Set(header, v)
		}
	}
}
