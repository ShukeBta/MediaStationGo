package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// CloudReadText resolves a small cloud file and returns its text payload. It is
// used for cloud-hosted .strm files: the scanner reads the STRM target once and
// stores the real playback URL, while the media bytes still stay in the cloud.
func (s *StorageConfigService) CloudReadText(ctx context.Context, typ, fileRef string, limit int64) (string, error) {
	if limit <= 0 {
		limit = 64 << 10
	}
	link, err := s.CloudResolve(ctx, typ, fileRef, "")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link.URL, nil)
	if err != nil {
		return "", err
	}
	for k, v := range link.Headers {
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s: read strm returned http %d", typ, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > limit {
		return "", fmt.Errorf("%s: strm file is too large", typ)
	}
	return strings.TrimSpace(strings.TrimPrefix(string(body), "\ufeff")), nil
}
