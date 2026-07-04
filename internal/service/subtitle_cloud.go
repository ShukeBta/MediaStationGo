package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func discoverCloudSubtitles(ctx context.Context, s *SubtitleService, m model.Media) []SubtitleTrack {
	if s == nil || s.storage == nil {
		return []SubtitleTrack{}
	}
	typ, mediaRef, ok := cloudSubtitleMediaRef(m)
	if !ok {
		return []SubtitleTrack{}
	}
	dirRef, mediaName := splitCloudRef(mediaRef)
	if mediaName == "" {
		return []SubtitleTrack{}
	}
	base := strings.TrimSuffix(mediaName, filepath.Ext(mediaName))
	entries, err := s.storage.CloudList(ctx, typ, dirRef)
	if err != nil {
		if s.log != nil {
			s.log.Debug("list cloud subtitles failed", zap.String("provider", typ), zap.String("dir", dirRef), zap.Error(err))
		}
		return []SubtitleTrack{}
	}
	tracks := cloudSubtitleTracks(typ, entries, base, false)
	for _, entry := range entries {
		if !entry.IsDir || !isSubtitleDirectory(entry.Name) || strings.TrimSpace(entry.ID) == "" {
			continue
		}
		subEntries, err := s.storage.CloudList(ctx, typ, entry.ID)
		if err != nil {
			continue
		}
		tracks = append(tracks, cloudSubtitleTracks(typ, subEntries, base, true)...)
	}
	return tracks
}

func cloudSubtitleTracks(typ string, entries []cloud.FileEntry, base string, subdir bool) []SubtitleTrack {
	tracks := make([]SubtitleTrack, 0)
	baseLower := strings.ToLower(base)
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name))
		codec, ok := extToCodec[ext]
		if !ok {
			continue
		}
		fullName := strings.TrimSuffix(entry.Name, ext)
		if !subdir && !strings.HasPrefix(strings.ToLower(fullName), baseLower) {
			continue
		}
		ref := cloudEntryRef(typ, entry.ID, entry.PickCode)
		if ref == "" {
			continue
		}
		lang := detectLang(fullName, base)
		tracks = append(tracks, SubtitleTrack{
			Lang:  lang,
			Label: lang,
			Path:  buildCloudSubtitlePath(typ, ref, entry.Name),
			Codec: codec,
		})
	}
	return tracks
}

func cloudSubtitleMediaRef(m model.Media) (typ, ref string, ok bool) {
	if info, parsed := ParseCloudLibraryMount(m.Path); parsed && strings.TrimSpace(info.DisplayDir) != "" {
		return info.Provider, info.DisplayDir, true
	}
	if typ, ref, parsed := parseCloudMediaPlaybackURL(m.STRMURL); parsed {
		return typ, ref, true
	}
	return "", "", false
}

func splitCloudRef(ref string) (dir, name string) {
	ref = strings.Trim(strings.ReplaceAll(strings.TrimSpace(ref), "\\", "/"), "/")
	if ref == "" {
		return "", ""
	}
	idx := strings.LastIndex(ref, "/")
	if idx < 0 {
		return "", ref
	}
	return ref[:idx], ref[idx+1:]
}

func isSubtitleDirectory(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "subs", "sub", ".sub", "subtitles", "subtitle":
		return true
	default:
		return false
	}
}

func buildCloudSubtitlePath(typ, ref, name string) string {
	u := url.URL{
		Scheme: "cloud",
		Host:   strings.TrimSpace(typ),
		Path:   "/" + strings.TrimLeft(strings.TrimSpace(ref), "/"),
	}
	q := u.Query()
	q.Set("name", strings.TrimSpace(name))
	u.RawQuery = q.Encode()
	return u.String()
}

func parseCloudSubtitlePath(raw string) (typ, ref, name string, ok bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || strings.ToLower(u.Scheme) != "cloud" || strings.TrimSpace(u.Host) == "" {
		return "", "", "", false
	}
	ref = strings.TrimLeft(u.EscapedPath(), "/")
	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	return strings.TrimSpace(u.Host), strings.TrimSpace(ref), strings.TrimSpace(u.Query().Get("name")), ref != ""
}

func serveCloudSubtitle(ctx context.Context, s *SubtitleService, m model.Media, typ, ref, name string, w io.Writer) error {
	if s == nil || s.storage == nil {
		return errors.New("cloud storage service unavailable")
	}
	mediaTyp, _, ok := cloudSubtitleMediaRef(m)
	if !ok || mediaTyp != typ {
		return ErrCloudPlaybackUnavailable
	}
	allowed := false
	for _, track := range discoverCloudSubtitles(ctx, s, m) {
		if track.Path == buildCloudSubtitlePath(typ, ref, name) {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("path escape")
	}
	body, err := s.storage.CloudReadText(ctx, typ, ref, 8<<20)
	if err != nil {
		return err
	}
	ext := strings.ToLower(filepath.Ext(firstNonEmpty(name, ref)))
	switch ext {
	case ".vtt":
		_, err = io.WriteString(w, body)
	case ".srt":
		_, err = io.WriteString(w, srtToVTT(body))
	case ".ass", ".ssa":
		_, err = io.WriteString(w, assToVTT(body))
	default:
		return errors.New("unsupported subtitle format")
	}
	return err
}
