package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	localSubtitleScheme   = "local-subtitle"
	maxLocalSubtitleBytes = 16 << 20
)

var (
	safeSubtitleMediaIDRE  = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	safeSubtitleFilenameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type localSubtitleIndex struct {
	Tracks []localSubtitleIndexTrack `json:"tracks"`
}

type localSubtitleIndexTrack struct {
	MediaID    string `json:"media_id"`
	Filename   string `json:"filename"`
	Name       string `json:"name,omitempty"`
	Lang       string `json:"lang"`
	Label      string `json:"label"`
	Path       string `json:"path,omitempty"`
	Source     string `json:"source,omitempty"`
	ProviderID string `json:"provider_id,omitempty"`
	Query      string `json:"query,omitempty"`
	Score      int    `json:"score,omitempty"`
}

func subtitleCacheDirFromEnv() string {
	for _, key := range []string{"MEDIASTATION_SUBTITLE_CACHE_DIR", "SUBTITLE_CACHE_DIR"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func (s *SubtitleService) SetLocalCacheDir(dir string) {
	if s != nil {
		s.localCacheDir = strings.TrimSpace(dir)
	}
}

func discoverLocalCachedSubtitles(s *SubtitleService, mediaID string, strict bool) ([]SubtitleTrack, error) {
	if s == nil {
		return []SubtitleTrack{}, nil
	}
	mediaDir, ok := localSubtitleMediaDir(s.localCacheDir, mediaID)
	if !ok {
		return []SubtitleTrack{}, nil
	}
	raw, err := os.ReadFile(filepath.Join(mediaDir, "tracks.json")) // #nosec G304 -- mediaDir is constrained by localSubtitleMediaDir.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []SubtitleTrack{}, nil
		}
		if strict {
			return nil, fmt.Errorf("read cached subtitle index: %w", err)
		}
		return []SubtitleTrack{}, nil
	}
	var index localSubtitleIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		if strict {
			return nil, fmt.Errorf("parse cached subtitle index: %w", err)
		}
		return []SubtitleTrack{}, nil
	}
	tracks := make([]SubtitleTrack, 0, len(index.Tracks))
	for _, item := range index.Tracks {
		if strings.TrimSpace(item.MediaID) != "" && strings.TrimSpace(item.MediaID) != strings.TrimSpace(mediaID) {
			if strict {
				return nil, errors.New("cached subtitle index contains a mismatched media id")
			}
			continue
		}
		filename := strings.TrimSpace(item.Filename)
		if !safeLocalSubtitleFilename(filename) {
			if strict {
				return nil, errors.New("cached subtitle index contains an invalid filename")
			}
			continue
		}
		if _, err := os.Stat(filepath.Join(mediaDir, filename)); err != nil {
			if strict {
				return nil, fmt.Errorf("stat cached subtitle %q: %w", filename, err)
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(filename))
		codec, ok := extToCodec[ext]
		if !ok {
			if strict {
				return nil, fmt.Errorf("cached subtitle %q has unsupported format", filename)
			}
			continue
		}
		lang := strings.TrimSpace(item.Lang)
		if lang == "" {
			lang, _ = detectLangLabel(strings.TrimSuffix(filename, ext), "")
		}
		label := strings.TrimSpace(item.Label)
		if label == "" {
			_, label = detectLangLabel(strings.TrimSuffix(filename, ext), "")
		}
		source := firstNonEmpty(strings.TrimSpace(item.Source), "cache")
		tracks = append(tracks, SubtitleTrack{
			Lang:           lang,
			Label:          label,
			Name:           firstNonEmpty(strings.TrimSpace(item.Name), filename),
			Path:           localSubtitleURI(mediaID, filename),
			Codec:          codec,
			Source:         source,
			ApplicationKey: subtitleApplicationKey(source, item.ProviderID),
		})
	}
	return tracks, nil
}

func subtitleApplicationKey(source, providerID string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	providerID = strings.TrimSpace(providerID)
	if source == "" || providerID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(source + "\x00" + providerID))
	return hex.EncodeToString(sum[:])
}

func localSubtitleURI(mediaID, filename string) string {
	return cachedSubtitleURI(localSubtitleScheme, mediaID, filename)
}

func cachedSubtitleURI(scheme, mediaID, filename string) string {
	u := url.URL{
		Scheme: strings.TrimSpace(scheme),
		Host:   strings.TrimSpace(mediaID),
		Path:   "/" + strings.TrimSpace(filename),
	}
	return u.String()
}

func parseLocalSubtitlePath(raw string) (mediaID, filename string, ok bool) {
	return parseSubtitleURI(raw, localSubtitleScheme)
}

func parseSubtitleURI(raw, scheme string) (mediaID, filename string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, strings.TrimSpace(scheme)) {
		return "", "", false
	}
	mediaID = strings.TrimSpace(u.Host)
	filename = strings.TrimLeft(u.EscapedPath(), "/")
	if decoded, err := url.PathUnescape(filename); err == nil {
		filename = decoded
	}
	if !safeSubtitleMediaID(mediaID) || !safeLocalSubtitleFilename(filename) {
		return "", "", false
	}
	return mediaID, filename, true
}

func serveLocalCachedSubtitle(s *SubtitleService, mediaID, localMediaID, filename, format string, w io.Writer) error {
	if s == nil {
		return errors.New("subtitle service unavailable")
	}
	return serveCachedSubtitleFromRoot(s.localCacheDir, mediaID, localMediaID, filename, format, w)
}

func serveCachedSubtitleFromRoot(root, mediaID, localMediaID, filename, format string, w io.Writer) error {
	if strings.TrimSpace(mediaID) != strings.TrimSpace(localMediaID) {
		return errors.New("subtitle media mismatch")
	}
	mediaDir, ok := localSubtitleMediaDir(root, mediaID)
	if !ok {
		return errors.New("subtitle cache unavailable")
	}
	path := filepath.Join(mediaDir, filename)
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	mediaDirAbs, _ := filepath.Abs(mediaDir)
	if !pathWithin(abs, mediaDirAbs) {
		return errors.New("path escape")
	}
	stat, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if stat.IsDir() || stat.Size() > maxLocalSubtitleBytes {
		return errors.New("subtitle file invalid")
	}
	body, err := os.ReadFile(abs) // #nosec G304 -- abs is constrained to the subtitle cache media directory.
	if err != nil {
		return err
	}
	return writeSubtitleBody(w, body, filepath.Ext(filename), format)
}

func localSubtitleMediaDir(root, mediaID string) (string, bool) {
	root = strings.TrimSpace(root)
	mediaID = strings.TrimSpace(mediaID)
	if root == "" || !safeSubtitleMediaID(mediaID) {
		return "", false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	mediaDir := filepath.Join(rootAbs, mediaID)
	if !pathWithin(mediaDir, rootAbs) {
		return "", false
	}
	return mediaDir, true
}

func safeSubtitleMediaID(value string) bool {
	return safeSubtitleMediaIDRE.MatchString(strings.TrimSpace(value))
}

func safeLocalSubtitleFilename(filename string) bool {
	filename = strings.TrimSpace(filename)
	if filename == "" || strings.ContainsAny(filename, `/\`) || filename == "." || filename == ".." {
		return false
	}
	if !safeSubtitleFilenameRE.MatchString(filename) {
		return false
	}
	_, ok := normalizeSubtitleExtension(filepath.Ext(filename))
	return ok
}

func mergeSubtitleTracks(trackLists ...[]SubtitleTrack) []SubtitleTrack {
	merged := make([]SubtitleTrack, 0)
	seen := map[string]struct{}{}
	for _, tracks := range trackLists {
		for _, track := range tracks {
			key := strings.TrimSpace(track.Path)
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, track)
		}
	}
	return merged
}
