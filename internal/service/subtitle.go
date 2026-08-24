// Package service — subtitle handling.
//
// SubtitleService finds external subtitle files next to a media file and
// converts SRT to WebVTT on the fly so the browser <track> element can
// load them directly.
//
// External-subtitle discovery rules (matching the legacy Python defaults):
//
//  1. Same directory, same basename, different extension.
//  2. Same directory, ".sub/" or "subs/" subdirectory.
//  3. Sibling languages e.g. movie.zh.srt / movie.en.srt → exposed as
//     ?lang=zh / ?lang=en.
//
// Supported extensions: .srt, .ass, .ssa, .vtt.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// SubtitleService is the discovery + conversion entry point.
type SubtitleService struct {
	log                  *zap.Logger
	repo                 *repository.Container
	storage              *StorageConfigService
	pipeline             subtitlePipelineClient
	apiConfig            *APIConfigService
	localCacheDir        string
	materializedCacheDir string
	cloudCache           *cloudSubtitleDiscoveryCache
	materializeMu        sync.Mutex
}

func (s *SubtitleService) SetAPIConfig(apiConfig *APIConfigService) *SubtitleService {
	if s != nil {
		s.apiConfig = apiConfig
	}
	return s
}

// NewSubtitleService is the constructor.
func NewSubtitleService(log *zap.Logger, repo *repository.Container, storage ...*StorageConfigService) *SubtitleService {
	s := &SubtitleService{
		log:                  log,
		repo:                 repo,
		localCacheDir:        subtitleCacheDirFromEnv(),
		materializedCacheDir: cloudSubtitleMaterializedDirFromEnv(),
		cloudCache:           newCloudSubtitleDiscoveryCache(cloudSubtitleDiscoveryTTLFromEnv(log)),
	}
	if len(storage) > 0 {
		s.storage = storage[0]
	}
	return s
}

func (s *SubtitleService) SetStorageConfig(storage *StorageConfigService) {
	if s != nil {
		s.storage = storage
		s.InvalidateCloudDiscovery("", "")
	}
}

// SubtitleTrack describes one external subtitle file.
type SubtitleTrack struct {
	Lang           string `json:"lang"`
	Label          string `json:"label"`
	Name           string `json:"name"`
	Path           string `json:"path"`
	URL            string `json:"url"`
	Codec          string `json:"codec"`
	Source         string `json:"source"`
	ApplicationKey string `json:"application_key"`
}

// extToCodec maps the file extension to the inner codec name.
var extToCodec = map[string]string{
	".srt": "srt",
	".vtt": "webvtt",
	".ass": "ass",
	".ssa": "ssa",
	".sup": "pgs",
}

// SubtitleContentType returns the response content type for a subtitle stream
// format used by Emby-compatible clients.
func SubtitleContentType(format string) (string, bool) {
	ext, ok := normalizeSubtitleExtension(format)
	if !ok {
		return "", false
	}
	switch ext {
	case ".vtt":
		return "text/vtt; charset=utf-8", true
	case ".srt":
		return "application/x-subrip; charset=utf-8", true
	case ".ass":
		return "text/x-ass; charset=utf-8", true
	case ".ssa":
		return "text/x-ssa; charset=utf-8", true
	case ".sup":
		return "application/octet-stream", true
	default:
		return "", false
	}
}

func normalizeSubtitleExtension(format string) (string, bool) {
	format = strings.ToLower(strings.TrimSpace(format))
	format = strings.TrimPrefix(format, ".")
	if format == "" {
		format = "vtt"
	}
	switch format {
	case "vtt", "webvtt":
		return ".vtt", true
	case "srt", "ass", "ssa", "sup":
		return "." + format, true
	default:
		return "", false
	}
}

// Discover lists every external subtitle file for a media row. The URL is
// relative; the caller should prepend /api/subtitles/<media_id>?path=...
// when serializing for the frontend.
func (s *SubtitleService) Discover(ctx context.Context, mediaID string) ([]SubtitleTrack, error) {
	return s.discover(ctx, mediaID, false)
}

func (s *SubtitleService) DiscoverStrict(ctx context.Context, mediaID string) ([]SubtitleTrack, error) {
	return s.discover(ctx, mediaID, true)
}

func (s *SubtitleService) discover(ctx context.Context, mediaID string, strict bool) ([]SubtitleTrack, error) {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("media not found")
	}
	localTracks, err := discoverLocalCachedSubtitles(s, mediaID, strict)
	if err != nil {
		return nil, err
	}
	// Cloud media discovery is strictly local. Remote listing and downloads only
	// run during ingest, explicit refresh, or an administrator backfill task.
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://") {
		materializedTracks, materializedErr := discoverMaterializedCloudSubtitles(s, *m, strict)
		if materializedErr != nil {
			return nil, materializedErr
		}
		return mergeSubtitleTracks(localTracks, materializedTracks), nil
	}
	dir := filepath.Dir(m.Path)
	base := strings.TrimSuffix(filepath.Base(m.Path), filepath.Ext(m.Path))

	candidates := make([]string, 0, 16)
	candidates = append(candidates, dir)
	for _, sub := range []string{"subs", "Subs", "sub", ".sub"} {
		candidates = append(candidates, filepath.Join(dir, sub))
	}

	tracks := make([]SubtitleTrack, 0)
	for _, c := range candidates {
		entries, err := os.ReadDir(c)
		if err != nil {
			if strict && (c == dir || !errors.Is(err, os.ErrNotExist)) {
				return nil, fmt.Errorf("read subtitle directory %q: %w", c, err)
			}
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			codec, ok := extToCodec[ext]
			if !ok {
				continue
			}
			fullName := strings.TrimSuffix(e.Name(), ext)
			if !strings.HasPrefix(strings.ToLower(fullName), strings.ToLower(base)) &&
				c == dir {
				// In the same directory we require a basename match;
				// inside subs/ subdirs we accept anything.
				continue
			}
			lang, label := detectLangLabel(fullName, base)
			tracks = append(tracks, SubtitleTrack{
				Lang:   lang,
				Label:  label,
				Name:   e.Name(),
				Path:   filepath.Join(c, e.Name()),
				Codec:  codec,
				Source: "media",
			})
		}
	}
	return mergeSubtitleTracks(tracks, localTracks), nil
}

// langTag matches the .zh / .zh-cn / .chs language sub-extensions.
var langTag = regexp.MustCompile(`(?i)\.([a-z]{2,3}(?:[-_][a-z]{2,4})?)$`)

func detectLang(name, base string) string {
	suffix := strings.TrimPrefix(name, base)
	suffix = strings.TrimPrefix(suffix, ".")
	if m := langTag.FindStringSubmatch("." + suffix); len(m) >= 2 {
		return strings.ToLower(m[1])
	}
	if suffix == "" {
		return "und" // undetermined
	}
	return strings.ToLower(suffix)
}

func detectLangLabel(name, base string) (string, string) {
	lang := detectLang(name, base)
	switch strings.ToLower(strings.ReplaceAll(lang, "_", "-")) {
	case "sc", "chs", "zh-cn", "zh-hans", "zh-sg":
		return "zh-Hans", "简体中文"
	case "tc", "cht", "zh-tw", "zh-hant", "zh-hk":
		return "zh-Hant", "繁体中文"
	case "jp", "jpn", "ja":
		return "ja", "日语"
	case "en", "eng":
		return "en", "English"
	case "und", "":
		return "und", "Subtitle"
	default:
		return lang, lang
	}
}

// Serve writes the subtitle file as WebVTT (.vtt). SRT/SSA files are converted
// minimally on the fly. Returns ErrSubtitleNotFound when the path is rejected.
func (s *SubtitleService) Serve(ctx context.Context, mediaID, sub string, w io.Writer) error {
	return s.ServeAs(ctx, mediaID, sub, ".vtt", w)
}

// ServeAs writes the subtitle in the requested Emby-compatible format. Native
// .srt/.ass/.ssa/.sup requests are served without conversion; .vtt requests
// convert supported text subtitle formats to WebVTT.
func (s *SubtitleService) ServeAs(ctx context.Context, mediaID, sub, format string, w io.Writer) error {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return errors.New("media not found")
	}
	if localMediaID, filename, ok := parseLocalSubtitlePath(sub); ok {
		return serveLocalCachedSubtitle(s, mediaID, localMediaID, filename, format, w)
	}
	if localMediaID, filename, ok := parseMaterializedSubtitlePath(sub); ok {
		return serveMaterializedCloudSubtitle(s, mediaID, localMediaID, filename, format, w)
	}
	if typ, ref, name, ok := parseCloudSubtitlePath(sub); ok {
		return serveCloudSubtitle(ctx, s, *m, typ, ref, name, format, w)
	}
	abs, err := filepath.Abs(sub)
	if err != nil {
		return err
	}
	mediaDir, _ := filepath.Abs(filepath.Dir(m.Path))
	if !pathWithin(abs, mediaDir) {
		return fmt.Errorf("path escape")
	}

	f, err := os.Open(abs) // #nosec G304 -- abs is constrained to the media file directory with pathWithin.
	if err != nil {
		return err
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	return writeSubtitleBody(w, body, filepath.Ext(abs), format)
}

func writeSubtitleBody(w io.Writer, body []byte, sourceExt, targetExt string) error {
	sourceExt, sourceOK := normalizeSubtitleExtension(sourceExt)
	if !sourceOK {
		return errors.New("unsupported subtitle format")
	}
	targetExt, targetOK := normalizeSubtitleExtension(targetExt)
	if !targetOK {
		return errors.New("unsupported subtitle target format")
	}
	if targetExt != ".vtt" {
		if targetExt != sourceExt {
			return errors.New("subtitle target format does not match source")
		}
		_, err := w.Write(body)
		return err
	}
	switch sourceExt {
	case ".vtt":
		_, err := w.Write(body)
		return err
	case ".srt":
		_, err := w.Write([]byte(srtToVTT(string(body))))
		return err
	case ".ass", ".ssa":
		_, err := w.Write([]byte(assToVTT(string(body))))
		return err
	default:
		return errors.New("unsupported subtitle format")
	}
}
