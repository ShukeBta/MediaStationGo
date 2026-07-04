package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

type organizeTargetInput struct {
	Root      string
	MediaType string
	Category  string
	Title     string
	Source    string
	Ext       string
	Year      int
	Season    int
	Episode   int
	Series    bool
}

type organizeTargetPath struct {
	Dir        string
	Path       string
	EpisodeTag string
}

func (o *OrganizerService) buildOrganizeTargetPath(ctx context.Context, in organizeTargetInput) (organizeTargetPath, error) {
	root := filepath.Clean(strings.TrimSpace(in.Root))
	if root == "" || root == "." {
		return organizeTargetPath{}, fmt.Errorf("organize target root required")
	}
	title := sanitizeFilename(strings.TrimSpace(in.Title))
	if title == "" {
		title = "Unknown"
	}
	ext := strings.TrimSpace(in.Ext)
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	episodeTag := ""
	if in.Series {
		season := in.Season
		if season < 0 {
			season = 1
		}
		episode := in.Episode
		if episode <= 0 {
			episode = 1
		}
		episodeTag = fmt.Sprintf("S%02dE%02d", season, episode)
	}

	template := strings.TrimSpace(o.organizeNamingFormat(ctx, in.MediaType, in.Series))
	var rel string
	if template == "" {
		rel = defaultOrganizeRelativePath(title, ext, in.Year, in.Season, in.Episode, in.Series)
	} else {
		rel = renderOrganizeNamingTemplate(template, organizeNamingData{
			Title:       title,
			Year:        in.Year,
			Season:      in.Season,
			Episode:     in.Episode,
			Ext:         strings.TrimPrefix(ext, "."),
			FileExt:     ext,
			Category:    sanitizeFilename(in.Category),
			MediaType:   normalizeOrganizeMediaType(in.MediaType),
			EpisodeTag:  episodeTag,
			VideoFormat: extractOrganizeReleaseTag(in.Source),
			Part:        extractOrganizeReleaseTag(in.Source),
		})
		rel = cleanOrganizeRelativePath(rel)
		if rel == "" {
			rel = defaultOrganizeRelativePath(title, ext, in.Year, in.Season, in.Episode, in.Series)
		}
		if ext != "" && !strings.EqualFold(filepath.Ext(rel), ext) {
			rel += ext
		}
	}
	dst := filepath.Join(root, rel)
	return organizeTargetPath{
		Dir:        filepath.Dir(dst),
		Path:       dst,
		EpisodeTag: episodeTag,
	}, nil
}

func (o *OrganizerService) organizeNamingFormat(ctx context.Context, mediaType string, series bool) string {
	if o == nil || o.repo == nil || o.repo.Setting == nil {
		return ""
	}
	key := "organize.movie_format"
	if series {
		if normalizeOrganizeMediaType(mediaType) == "anime" {
			key = "organize.anime_format"
		} else {
			key = "organize.tv_format"
		}
	}
	value, err := o.repo.Setting.Get(ctx, key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func defaultOrganizeRelativePath(title, ext string, year, season, episode int, series bool) string {
	if series {
		if season < 0 {
			season = 1
		}
		if episode <= 0 {
			episode = 1
		}
		episodeTag := fmt.Sprintf("S%02dE%02d", season, episode)
		return filepath.Join(title, fmt.Sprintf("Season %02d", season), fmt.Sprintf("%s - %s%s", title, episodeTag, ext))
	}
	folder := title
	if year > 0 {
		folder = fmt.Sprintf("%s (%d)", title, year)
	}
	return filepath.Join(folder, folder+ext)
}
