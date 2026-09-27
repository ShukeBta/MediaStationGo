package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func loadCloudSubtitles(ctx context.Context, s *SubtitleService, m model.Media) ([]SubtitleTrack, error) {
	return loadCloudSubtitlesForMaterialization(ctx, s, m, false)
}

func loadCloudSubtitlesForMaterialization(ctx context.Context, s *SubtitleService, m model.Media, force bool) ([]SubtitleTrack, error) {
	if s == nil || s.storage == nil {
		return nil, errors.New("cloud storage service unavailable")
	}
	typ, mediaRef, ok := cloudSubtitleMediaRef(m)
	if !ok {
		return []SubtitleTrack{}, nil
	}
	dirRef, mediaName := splitCloudRef(mediaRef)
	if mediaName == "" {
		return []SubtitleTrack{}, nil
	}
	base := strings.TrimSuffix(mediaName, filepath.Ext(mediaName))
	entries, err := listCloudSubtitleDirectory(ctx, s.storage, typ, dirRef, force)
	if err != nil {
		return nil, fmt.Errorf("list cloud subtitle directory %q: %w", dirRef, err)
	}
	tracks := cloudSubtitleTracks(typ, entries, base, true)
	sharedMediaDirectory := cloudVideoFileCount(entries) > 1
	for _, entry := range entries {
		if !entry.IsDir || !isSubtitleDirectory(entry.Name) || strings.TrimSpace(entry.ID) == "" {
			continue
		}
		subEntries, err := listCloudSubtitleDirectory(ctx, s.storage, typ, entry.ID, force)
		if err != nil {
			return tracks, fmt.Errorf("list cloud subtitle subdirectory %q: %w", entry.Name, err)
		}
		tracks = append(tracks, cloudSubtitleTracks(typ, subEntries, base, sharedMediaDirectory)...)
	}
	return tracks, nil
}

func listCloudSubtitleDirectory(ctx context.Context, storage *StorageConfigService, typ, dirRef string, force bool) ([]cloud.FileEntry, error) {
	if force {
		return storage.CloudListRefresh(ctx, typ, dirRef)
	}
	return storage.CloudList(ctx, typ, dirRef)
}

func cloudSubtitleTracks(typ string, entries []cloud.FileEntry, base string, requireBaseMatch bool) []SubtitleTrack {
	tracks := make([]SubtitleTrack, 0)
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
		if requireBaseMatch && !cloudSubtitleBaseMatches(fullName, base) {
			continue
		}
		ref := cloudEntryRef(typ, entry.ID, entry.PickCode)
		if ref == "" {
			continue
		}
		lang, label := detectLangLabel(fullName, base)
		tracks = append(tracks, SubtitleTrack{
			Lang:   lang,
			Label:  label,
			Name:   entry.Name,
			Path:   buildCloudSubtitlePath(typ, ref, entry.Name),
			Codec:  codec,
			Source: typ,
		})
	}
	return tracks
}

func cloudSubtitleBaseMatches(name, base string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	base = strings.ToLower(strings.TrimSpace(base))
	if name == "" || base == "" || !strings.HasPrefix(name, base) {
		return false
	}
	if len(name) == len(base) {
		return true
	}
	switch name[len(base)] {
	case '.', '-', '_', ' ', '[', '(':
		return true
	default:
		return false
	}
}

func cloudVideoFileCount(entries []cloud.FileEntry) int {
	count := 0
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		if _, ok := videoExtensions[strings.ToLower(filepath.Ext(entry.Name))]; ok {
			count++
		}
	}
	return count
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

func serveCloudSubtitle(ctx context.Context, s *SubtitleService, m model.Media, typ, ref, name, format string, w io.Writer) error {
	if s == nil || s.storage == nil {
		return errors.New("cloud storage service unavailable")
	}
	mediaTyp, _, ok := cloudSubtitleMediaRef(m)
	if !ok || mediaTyp != typ {
		return ErrCloudPlaybackUnavailable
	}
	allowed := false
	tracks, err := s.discoverCloudSubtitlesCached(ctx, m)
	if err != nil {
		return err
	}
	for _, track := range tracks {
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
	return writeSubtitleBody(w, []byte(body), cloudSubtitleSourceExtension(name, ref), format)
}

func cloudSubtitleSourceExtension(name, ref string) string {
	return filepath.Ext(firstNonEmpty(name, ref))
}
