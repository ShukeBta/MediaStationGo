package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const subtitleDetectionSampleBytes = 1 << 20

type SubtitlePresenceResult struct {
	MediaID         string                  `json:"media_id"`
	HasChinese      bool                    `json:"has_chinese"`
	EmbeddedChecked bool                    `json:"embedded_checked"`
	Embedded        []SubtitlePresenceTrack `json:"embedded"`
	External        []SubtitlePresenceTrack `json:"external"`
	UnknownEmbedded int                     `json:"unknown_embedded"`
}

type SubtitlePresenceTrack struct {
	Kind     string `json:"kind"`
	Index    int    `json:"index,omitempty"`
	Codec    string `json:"codec,omitempty"`
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
	Name     string `json:"name,omitempty"`
	Source   string `json:"source,omitempty"`
	Chinese  bool   `json:"chinese"`
}

func (s *SubtitleService) Presence(ctx context.Context, mediaID string, stream *StreamService, probe mediaTrackProber) (SubtitlePresenceResult, error) {
	result := SubtitlePresenceResult{
		MediaID:  strings.TrimSpace(mediaID),
		Embedded: []SubtitlePresenceTrack{},
		External: []SubtitlePresenceTrack{},
	}
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return result, fmt.Errorf("subtitle service unavailable")
	}
	if result.MediaID == "" {
		return result, fmt.Errorf("media id is required")
	}
	external, err := s.DiscoverStrict(ctx, result.MediaID)
	if err != nil {
		return result, fmt.Errorf("discover external subtitles: %w", err)
	}
	for _, track := range external {
		chinese := subtitleDescriptorIsChinese(track.Lang, track.Label, track.Name)
		if !chinese && subtitleLanguageUnknown(track.Lang) {
			chinese, err = s.externalSubtitleBodyIsChinese(ctx, result.MediaID, track)
			if err != nil {
				return result, fmt.Errorf("inspect external subtitle %q: %w", track.Name, err)
			}
		}
		result.External = append(result.External, SubtitlePresenceTrack{
			Kind:     "external",
			Codec:    track.Codec,
			Language: track.Lang,
			Title:    track.Label,
			Name:     track.Name,
			Source:   track.Source,
			Chinese:  chinese,
		})
		result.HasChinese = result.HasChinese || chinese
	}
	if result.HasChinese {
		return result, nil
	}
	if stream == nil || probe == nil {
		return result, fmt.Errorf("media stream probe unavailable")
	}
	probeResult, _, err := stream.Inspect(ctx, result.MediaID, probe)
	if err != nil {
		return result, fmt.Errorf("inspect embedded subtitles: %w", err)
	}
	result.EmbeddedChecked = true
	for _, track := range probeResult.SubtitleStreams {
		chinese := subtitleDescriptorIsChinese(track.Language, track.Title)
		if !chinese && subtitleLanguageUnknown(track.Language) {
			result.UnknownEmbedded++
		}
		result.Embedded = append(result.Embedded, SubtitlePresenceTrack{
			Kind:     "embedded",
			Index:    track.Index,
			Codec:    track.Codec,
			Language: track.Language,
			Title:    track.Title,
			Source:   "ffprobe",
			Chinese:  chinese,
		})
		result.HasChinese = result.HasChinese || chinese
	}
	return result, nil
}

func (s *SubtitleService) externalSubtitleBodyIsChinese(ctx context.Context, mediaID string, track SubtitleTrack) (bool, error) {
	if path := strings.TrimSpace(track.Path); path != "" && !strings.Contains(path, "://") {
		if info, err := os.Stat(path); err == nil && info.Size() > 8<<20 {
			return false, fmt.Errorf("subtitle exceeds 8 MiB detection limit")
		}
	}
	sample := subtitleSampleWriter{remaining: subtitleDetectionSampleBytes}
	if err := s.ServeAs(ctx, mediaID, track.Path, ".vtt", &sample); err != nil {
		return false, err
	}
	return subtitleTextIsChinese(sample.buffer.String()), nil
}

type subtitleSampleWriter struct {
	buffer    bytes.Buffer
	remaining int
}

func (w *subtitleSampleWriter) Write(p []byte) (int, error) {
	written := len(p)
	if w.remaining <= 0 {
		return written, nil
	}
	if len(p) > w.remaining {
		p = p[:w.remaining]
	}
	_, _ = w.buffer.Write(p)
	w.remaining -= len(p)
	return written, nil
}

var chineseSubtitleCodePattern = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:zh(?:[-_](?:cn|tw|hans|hant|hk|sg))?|zho|chi|cmn|chs|cht|sc|tc|chinese)(?:$|[^a-z0-9])`)

func subtitleDescriptorIsChinese(values ...string) bool {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if chineseSubtitleCodePattern.MatchString(value) {
			return true
		}
		for _, marker := range []string{"中文", "中字", "简体", "簡體", "繁体", "繁體", "中英双语", "中英雙語"} {
			if strings.Contains(value, marker) {
				return true
			}
		}
	}
	return false
}

func subtitleLanguageUnknown(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "und", "unknown":
		return true
	default:
		return false
	}
}

func subtitleTextIsChinese(value string) bool {
	han := 0
	kana := 0
	for _, r := range value {
		switch {
		case r >= 0x3400 && r <= 0x9fff, r >= 0xf900 && r <= 0xfaff:
			han++
		case r >= 0x3040 && r <= 0x30ff:
			kana++
		}
	}
	return han >= 4 && han > kana*2
}
