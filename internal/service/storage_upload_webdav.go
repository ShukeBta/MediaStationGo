package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

type webDAVUploader struct {
	base     *url.URL
	username string
	password string
	client   *http.Client
}

func newWebDAVUploader(cfg map[string]any) *webDAVUploader {
	u, _ := url.Parse(strings.TrimRight(strr(cfg["url"]), "/"))
	return &webDAVUploader{
		base:     u,
		username: strr(cfg["username"]),
		password: strr(cfg["password"]),
		client:   &http.Client{},
	}
}

func (w *webDAVUploader) ensureDir(ctx context.Context, remoteDir string) error {
	if w.base == nil || w.base.Scheme == "" || w.base.Host == "" {
		return errors.New("webdav missing url")
	}
	remoteDir = normalizeRemotePath(remoteDir)
	if remoteDir == "/" {
		return nil
	}
	current := ""
	for _, part := range strings.Split(strings.Trim(remoteDir, "/"), "/") {
		current = normalizeRemotePath(path.Join(current, part))
		req, err := http.NewRequestWithContext(ctx, "MKCOL", w.urlFor(current), nil)
		if err != nil {
			return err
		}
		w.auth(req)
		resp, err := w.client.Do(req)
		if err != nil {
			return decorateStorageTransportError("webdav", w.urlFor(current), err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			continue
		}
		if resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusConflict {
			continue
		}
		return fmt.Errorf("webdav mkdir %s: http %d", current, resp.StatusCode)
	}
	return nil
}

func (w *webDAVUploader) exists(ctx context.Context, remotePath string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, w.urlFor(remotePath), nil)
	if err != nil {
		return false, err
	}
	w.auth(req)
	resp, err := w.client.Do(req)
	if err != nil {
		return false, decorateStorageTransportError("webdav", w.urlFor(remotePath), err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return resp.StatusCode >= 200 && resp.StatusCode < 300, nil
}

func (w *webDAVUploader) upload(ctx context.Context, localPath, remotePath string, size int64) error {
	f, err := os.Open(localPath) // #nosec G304 -- localPath is selected from configured local media files before upload.
	if err != nil {
		return err
	}
	defer f.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, w.urlFor(remotePath), f)
	if err != nil {
		return err
	}
	w.auth(req)
	req.ContentLength = size
	resp, err := w.client.Do(req)
	if err != nil {
		return decorateStorageTransportError("webdav", w.urlFor(remotePath), err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webdav upload %s: http %d", remotePath, resp.StatusCode)
	}
	return nil
}

func (w *webDAVUploader) auth(req *http.Request) {
	if w.username != "" {
		req.SetBasicAuth(w.username, w.password)
	}
}

func (w *webDAVUploader) urlFor(remotePath string) string {
	u := *w.base
	basePath := strings.TrimRight(u.EscapedPath(), "/")
	segments := make([]string, 0)
	if basePath != "" && basePath != "/" {
		segments = append(segments, strings.Trim(basePath, "/"))
	}
	for _, part := range strings.Split(strings.Trim(normalizeRemotePath(remotePath), "/"), "/") {
		if part != "" {
			segments = append(segments, url.PathEscape(part))
		}
	}
	u.RawPath = ""
	u.Path = "/" + strings.Join(segments, "/")
	return u.String()
}
