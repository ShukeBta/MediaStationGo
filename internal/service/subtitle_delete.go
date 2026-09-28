package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *SubtitleService) Delete(ctx context.Context, mediaID, subtitlePath string) error {
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return errors.New("subtitle service unavailable")
	}
	mediaID = strings.TrimSpace(mediaID)
	subtitlePath = strings.TrimSpace(subtitlePath)
	if mediaID == "" || subtitlePath == "" {
		return errors.New("media id and subtitle path are required")
	}
	media, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return err
	}
	if media == nil {
		return errors.New("media not found")
	}
	tracks, err := s.Discover(ctx, mediaID)
	if err != nil {
		return err
	}
	allowed := false
	for _, track := range tracks {
		if track.Path == subtitlePath {
			allowed = true
			break
		}
	}
	if !allowed {
		return errors.New("subtitle does not belong to this media")
	}

	if localMediaID, _, ok := parseMaterializedSubtitlePath(subtitlePath); ok {
		if localMediaID != mediaID {
			return errors.New("subtitle media mismatch")
		}
		return errors.New("cloud media subtitles are managed by cloud subtitle refresh")
	}
	if localMediaID, filename, ok := parseLocalSubtitlePath(subtitlePath); ok {
		if localMediaID != mediaID {
			return errors.New("subtitle media mismatch")
		}
		return deleteLocalCachedSubtitle(s, mediaID, filename)
	}
	if typ, ref, _, ok := parseCloudSubtitlePath(subtitlePath); ok {
		mediaType, _, valid := cloudSubtitleMediaRef(*media)
		if !valid || mediaType != typ || s.storage == nil {
			return errors.New("cloud subtitle storage mismatch")
		}
		if err := s.storage.DeleteCloudFile(ctx, typ, ref); err != nil {
			return err
		}
		s.InvalidateCloudDiscovery(mediaID, typ)
		return nil
	}

	abs, err := filepath.Abs(subtitlePath)
	if err != nil {
		return err
	}
	mediaDir, err := filepath.Abs(filepath.Dir(media.Path))
	if err != nil {
		return err
	}
	if !pathWithin(abs, mediaDir) {
		return errors.New("subtitle path escape")
	}
	if _, ok := normalizeSubtitleExtension(filepath.Ext(abs)); !ok {
		return errors.New("unsupported subtitle format")
	}
	if err := os.Remove(abs); err != nil {
		return fmt.Errorf("delete subtitle: %w", err)
	}
	return nil
}

func deleteLocalCachedSubtitle(s *SubtitleService, mediaID, filename string) error {
	mediaDir, ok := localSubtitleMediaDir(s.localCacheDir, mediaID)
	if !ok || !safeLocalSubtitleFilename(filename) {
		return errors.New("subtitle cache unavailable")
	}
	indexPath := filepath.Join(mediaDir, "tracks.json")
	raw, err := os.ReadFile(indexPath) // #nosec G304 -- mediaDir is constrained by localSubtitleMediaDir.
	if err != nil {
		return fmt.Errorf("read subtitle index: %w", err)
	}
	var index localSubtitleIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return fmt.Errorf("decode subtitle index: %w", err)
	}
	remaining := make([]localSubtitleIndexTrack, 0, len(index.Tracks))
	found := false
	for _, track := range index.Tracks {
		if strings.TrimSpace(track.Filename) == filename && (strings.TrimSpace(track.MediaID) == "" || strings.TrimSpace(track.MediaID) == mediaID) {
			found = true
			continue
		}
		remaining = append(remaining, track)
	}
	if !found {
		return errors.New("subtitle cache entry not found")
	}
	target := filepath.Join(mediaDir, filename)
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	mediaDirAbs, err := filepath.Abs(mediaDir)
	if err != nil {
		return err
	}
	if !pathWithin(targetAbs, mediaDirAbs) {
		return errors.New("subtitle path escape")
	}
	if err := os.Remove(targetAbs); err != nil {
		return fmt.Errorf("delete cached subtitle: %w", err)
	}
	if len(remaining) == 0 {
		if err := os.Remove(indexPath); err != nil {
			return fmt.Errorf("delete subtitle index: %w", err)
		}
		return nil
	}
	index.Tracks = remaining
	encoded, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(indexPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write subtitle index: %w", err)
	}
	return nil
}
