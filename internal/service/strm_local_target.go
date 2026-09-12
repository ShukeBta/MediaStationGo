package service

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func isLocalSTRMFile(path string) bool {
	return !strings.Contains(path, "://") && strings.EqualFold(filepath.Ext(path), ".strm")
}

// mediaSTRMTarget also repairs the playback path for rows scanned before local
// STRM targets were supported. Cloud STRM files never authorize local reads.
func mediaSTRMTarget(m *model.Media) (string, error) {
	if target := strings.TrimSpace(m.STRMURL); target != "" {
		return target, nil
	}
	if isLocalSTRMFile(m.Path) {
		return readLocalSTRMTarget(m.Path)
	}
	return "", nil
}

func internalSTRMTarget(raw string) bool {
	return strings.HasPrefix(raw, "/api/") || strings.HasPrefix(raw, "/Videos/") || strings.HasPrefix(raw, "/videos/")
}

func isSTRMRedirectTarget(raw string) bool {
	if internalSTRMTarget(raw) {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "webdav", "davs", "alist", "alists", "openlist", "openlists":
		return true
	}
	return false
}

// localSTRMTargetPath resolves only video files referenced by a local STRM.
// Relative paths are relative to the STRM, not the server working directory.
func localSTRMTargetPath(strmPath, raw string) (string, bool) {
	if !isLocalSTRMFile(strmPath) {
		return "", false
	}
	target := strings.TrimSpace(raw)
	if target == "" || internalSTRMTarget(target) || strings.ContainsAny(target, "\x00\r\n") {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(target), "file:") {
		u, err := url.Parse(target)
		if err != nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
			return "", false
		}
		target = u.Path
		if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			if runtime.GOOS != "windows" {
				return "", false
			}
			target = "//" + u.Host + target
		}
		if len(target) > 3 && target[0] == '/' && isASCIIAlpha(target[1]) && target[2] == ':' {
			target = target[1:]
		}
	} else if !isWindowsStyleClientPath(target) {
		// Raw filesystem paths may contain literal '%' and '#' characters.
		// Detect a URI scheme without URL-decoding an ordinary path.
		if colon := strings.IndexByte(target, ':'); colon >= 0 && !strings.ContainsAny(target[:colon], `/\`) {
			return "", false
		}
	}
	ext := strings.ToLower(filepath.Ext(target))
	if _, ok := videoExtensions[ext]; !ok || ext == ".strm" {
		return "", false
	}
	if !filepath.IsAbs(target) && !strings.HasPrefix(target, "/") && !isWindowsStyleClientPath(target) {
		target = filepath.Join(filepath.Dir(strmPath), filepath.FromSlash(target))
	}
	return filepath.Clean(filepath.FromSlash(target)), true
}

// localMediaPlaybackPath is shared by direct playback, probing and transcoding
// so none of them accidentally consume the STRM text as video bytes.
func localMediaPlaybackPath(m *model.Media) (string, error) {
	path := m.Path
	if isLocalSTRMFile(path) {
		target, err := mediaSTRMTarget(m)
		if err != nil {
			return "", ErrMediaNotFound
		}
		var ok bool
		path, ok = localSTRMTargetPath(path, target)
		if !ok {
			return "", ErrMediaNotFound
		}
	}
	resolved, info, err := resolveAccessibleMappedPath(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", ErrMediaNotFound
	}
	return resolved, nil
}

func strmTargetContainer(m *model.Media) string {
	target, err := mediaSTRMTarget(m)
	if err != nil || target == "" {
		return ""
	}
	if local, ok := localSTRMTargetPath(m.Path, target); ok {
		target = local
	} else if _, ref, ok := parseCloudMediaPlaybackURL(target); ok {
		target = ref
	} else if u, err := url.Parse(target); err == nil {
		target = u.Path
	}
	ext := strings.ToLower(filepath.Ext(target))
	if _, ok := videoExtensions[ext]; ok && ext != ".strm" {
		return strings.TrimPrefix(ext, ".")
	}
	return ""
}
