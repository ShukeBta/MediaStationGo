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

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// SubtitleService is the discovery + conversion entry point.
type SubtitleService struct {
	log     *zap.Logger
	repo    *repository.Container
	storage *StorageConfigService
}

// NewSubtitleService is the constructor.
func NewSubtitleService(log *zap.Logger, repo *repository.Container, storage ...*StorageConfigService) *SubtitleService {
	s := &SubtitleService{log: log, repo: repo}
	if len(storage) > 0 {
		s.storage = storage[0]
	}
	return s
}

func (s *SubtitleService) SetStorageConfig(storage *StorageConfigService) {
	if s != nil {
		s.storage = storage
	}
}

// SubtitleTrack describes one external subtitle file.
type SubtitleTrack struct {
	Lang  string `json:"lang"`
	Label string `json:"label"`
	Path  string `json:"path"`
	URL   string `json:"url"`
	Codec string `json:"codec"`
}

// extToCodec maps the file extension to the inner codec name.
var extToCodec = map[string]string{
	".srt": "srt",
	".vtt": "vtt",
	".ass": "ass",
	".ssa": "ssa",
}

// Discover lists every external subtitle file for a media row. The URL is
// relative; the caller should prepend /api/subtitles/<media_id>?path=...
// when serializing for the frontend.
func (s *SubtitleService) Discover(ctx context.Context, mediaID string) ([]SubtitleTrack, error) {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errors.New("media not found")
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://") {
		return discoverCloudSubtitles(ctx, s, *m), nil
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
			lang := detectLang(fullName, base)
			tracks = append(tracks, SubtitleTrack{
				Lang:  lang,
				Label: lang,
				Path:  filepath.Join(c, e.Name()),
				Codec: codec,
			})
		}
	}
	return tracks, nil
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

// Serve writes the subtitle file as WebVTT (.vtt). SRT/SSA files are
// converted minimally on the fly. Returns ErrSubtitleNotFound when the
// path is rejected (path traversal / not in the media directory).
func (s *SubtitleService) Serve(ctx context.Context, mediaID, sub string, w io.Writer) error {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return errors.New("media not found")
	}
	if typ, ref, name, ok := parseCloudSubtitlePath(sub); ok {
		return serveCloudSubtitle(ctx, s, *m, typ, ref, name, w)
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

	switch strings.ToLower(filepath.Ext(abs)) {
	case ".vtt":
		_, err = w.Write(body)
	case ".srt":
		_, err = w.Write([]byte(srtToVTT(string(body))))
	case ".ass", ".ssa":
		_, err = w.Write([]byte(assToVTT(string(body))))
	default:
		return errors.New("unsupported subtitle format")
	}
	return err
}
