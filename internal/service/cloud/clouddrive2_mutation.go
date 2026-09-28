package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

func (p *cloudDrive2Provider) Mkdir(ctx context.Context, parentDir, name string) (*FileEntry, error) {
	cleanName, err := cleanCloudEntryName(name)
	if err != nil {
		return nil, err
	}
	parent := normalizeCloudDAVPath(parentDir)
	target := joinOpenListAPIPath(parent, cleanName)
	if p.typ == TypeOpenList && p.apiBase != nil && p.hasOpenListAPICredentials() {
		if err := p.openListAPIMkdir(ctx, target); err != nil {
			return nil, err
		}
		return &FileEntry{ID: target, Name: cleanName, IsDir: true}, nil
	}
	if err := p.webDAVMkdir(ctx, target); err != nil {
		return nil, err
	}
	return &FileEntry{ID: target, Name: cleanName, IsDir: true}, nil
}

func (p *cloudDrive2Provider) Rename(ctx context.Context, ref, name string) (*FileEntry, error) {
	cleanName, err := cleanCloudEntryName(name)
	if err != nil {
		return nil, err
	}
	source := normalizeCloudDAVPath(ref)
	if source == "/" {
		return nil, fmt.Errorf("%s: cannot rename root directory", p.name)
	}
	target := joinOpenListAPIPath(path.Dir(source), cleanName)
	if p.typ == TypeOpenList && p.apiBase != nil && p.hasOpenListAPICredentials() {
		if err := p.openListAPIRename(ctx, source, cleanName); err != nil {
			return nil, err
		}
		return &FileEntry{ID: target, Name: cleanName, IsDir: true}, nil
	}
	if err := p.webDAVRename(ctx, source, target); err != nil {
		return nil, err
	}
	return &FileEntry{ID: target, Name: cleanName, IsDir: true}, nil
}

func (p *cloudDrive2Provider) Move(ctx context.Context, ref, targetDir, name string) (*FileEntry, error) {
	source := normalizeCloudDAVPath(ref)
	if source == "/" {
		return nil, fmt.Errorf("%s: cannot move root directory", p.name)
	}
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		cleanName = path.Base(source)
	}
	var err error
	cleanName, err = cleanCloudEntryName(cleanName)
	if err != nil {
		return nil, err
	}
	targetDir = normalizeCloudDAVPath(targetDir)
	target := joinOpenListAPIPath(targetDir, cleanName)
	if sameCloudDAVPath(source, target) {
		return &FileEntry{ID: target, Name: cleanName}, nil
	}
	if p.typ == TypeOpenList && p.apiBase != nil && p.hasOpenListAPICredentials() {
		if err := p.openListAPIMove(ctx, source, targetDir, cleanName); err != nil {
			return nil, err
		}
		return &FileEntry{ID: target, Name: cleanName}, nil
	}
	if err := p.webDAVRename(ctx, source, target); err != nil {
		return nil, err
	}
	return &FileEntry{ID: target, Name: cleanName}, nil
}

func (p *cloudDrive2Provider) Delete(ctx context.Context, ref string) error {
	source := normalizeCloudDAVPath(ref)
	if source == "/" {
		return fmt.Errorf("%s: cannot delete root directory", p.name)
	}
	if p.typ == TypeOpenList && p.apiBase != nil && p.hasOpenListAPICredentials() {
		return p.openListAPIRemove(ctx, source)
	}
	return p.webDAVDelete(ctx, source)
}

func cleanCloudEntryName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("entry name is required")
	}
	if strings.ContainsAny(name, `/\`) {
		return "", fmt.Errorf("entry name cannot contain path separators")
	}
	return name, nil
}

func (p *cloudDrive2Provider) openListAPIMkdir(ctx context.Context, target string) error {
	return p.openListAPIPost(ctx, "/api/fs/mkdir", map[string]string{"path": normalizeCloudDAVPath(target)}, "mkdir")
}

func (p *cloudDrive2Provider) openListAPIRename(ctx context.Context, source, name string) error {
	return p.openListAPIPost(ctx, "/api/fs/rename", map[string]string{
		"path": normalizeCloudDAVPath(source),
		"name": name,
	}, "rename")
}

func (p *cloudDrive2Provider) openListAPIMove(ctx context.Context, source, targetDir, targetName string) error {
	targetDir = normalizeCloudDAVPath(targetDir)
	sourceName := path.Base(normalizeCloudDAVPath(source))
	if sameCloudDAVPath(path.Dir(source), targetDir) {
		if sourceName == targetName {
			return nil
		}
		return p.openListAPIRename(ctx, source, targetName)
	}
	if err := p.openListAPIPost(ctx, "/api/fs/move", map[string]any{
		"src_dir": normalizeCloudDAVPath(path.Dir(source)),
		"dst_dir": targetDir,
		"names":   []string{sourceName},
	}, "move"); err != nil {
		return err
	}
	if sourceName != targetName {
		moved := joinOpenListAPIPath(targetDir, sourceName)
		return p.openListAPIRename(ctx, moved, targetName)
	}
	return nil
}

func (p *cloudDrive2Provider) openListAPIRemove(ctx context.Context, source string) error {
	source = normalizeCloudDAVPath(source)
	err := p.openListAPIPost(ctx, "/api/fs/remove", map[string]any{
		"dir":   normalizeCloudDAVPath(path.Dir(source)),
		"names": []string{path.Base(source)},
	}, "remove")
	if IsOpenListAlreadyAbsentError(err) {
		return nil
	}
	return err
}

// IsOpenListAlreadyAbsentError reports whether OpenList says the requested
// file or directory has already disappeared. Delete and cleanup callers use
// the same predicate so an idempotent delete cannot fail in a later phase.
func IsOpenListAlreadyAbsentError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	message = strings.ToLower(strings.TrimSpace(message))
	for _, marker := range []string{"430004", "file not found", "object not found", "文件不存在", "不存在或已删除"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func (p *cloudDrive2Provider) openListAPIPost(ctx context.Context, apiPath string, payload any, action string) error {
	token, err := p.openListAPIToken(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.openListAPIURL(apiPath), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", p.ua)
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return decorateDAVTransportError(p.name, p.openListAPIURL(apiPath), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: api %s returned http %d", p.name, action, resp.StatusCode)
	}
	var decoded struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&decoded); err != nil {
		return fmt.Errorf("%s: decode api %s: %w", p.name, action, err)
	}
	if decoded.Code != 0 && decoded.Code != 200 {
		msg := strings.TrimSpace(decoded.Message)
		if msg == "" {
			msg = fmt.Sprintf("code %d", decoded.Code)
		}
		return fmt.Errorf("%s: api %s failed: %s", p.name, action, msg)
	}
	return nil
}

func (p *cloudDrive2Provider) webDAVMkdir(ctx context.Context, target string) error {
	req, err := http.NewRequestWithContext(ctx, "MKCOL", p.urlFor(target), nil)
	if err != nil {
		return err
	}
	p.auth(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return decorateDAVTransportError(p.name, p.urlFor(target), err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK, http.StatusNoContent:
		return nil
	case http.StatusMethodNotAllowed:
		return fmt.Errorf("%s: mkdir %s returned http %d; the folder may already exist or this WebDAV backend is read-only", p.name, target, resp.StatusCode)
	default:
		return p.decorateDAVMutationStatusError(resp, "mkdir", target)
	}
}

func (p *cloudDrive2Provider) webDAVRename(ctx context.Context, source, target string) error {
	req, err := http.NewRequestWithContext(ctx, "MOVE", p.urlFor(source), nil)
	if err != nil {
		return err
	}
	p.auth(req)
	req.Header.Set("Destination", p.webDAVDestination(target))
	req.Header.Set("Overwrite", "F")
	resp, err := p.client.Do(req)
	if err != nil {
		return decorateDAVTransportError(p.name, p.urlFor(source), err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK, http.StatusNoContent:
		return nil
	default:
		return p.decorateDAVMutationStatusError(resp, "rename", source)
	}
}

func (p *cloudDrive2Provider) webDAVDelete(ctx context.Context, source string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, p.urlFor(source), nil)
	if err != nil {
		return err
	}
	p.auth(req)
	resp, err := p.client.Do(req)
	if err != nil {
		return decorateDAVTransportError(p.name, p.urlFor(source), err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusAccepted, http.StatusNoContent, http.StatusNotFound:
		return nil
	default:
		return p.decorateDAVMutationStatusError(resp, "delete", source)
	}
}

func (p *cloudDrive2Provider) webDAVDestination(target string) string {
	raw := p.urlFor(target)
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (p *cloudDrive2Provider) decorateDAVMutationStatusError(resp *http.Response, action, target string) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	detail := compactDAVErrorBody(string(body))
	if detail == "" {
		return fmt.Errorf("%s: %s %s returned http %d", p.name, action, target, resp.StatusCode)
	}
	return fmt.Errorf("%s: %s %s returned http %d：%s", p.name, action, target, resp.StatusCode, detail)
}
