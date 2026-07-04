package service

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type organizeNamingData struct {
	Title       string
	Year        int
	Season      int
	Episode     int
	Ext         string
	FileExt     string
	Category    string
	MediaType   string
	EpisodeTag  string
	VideoFormat string
	Part        string
}

var organizeTemplateTokenRE = regexp.MustCompile(`\{([A-Za-z_]+)(?::([^}]+))?\}`)
var organizeTemplateMustacheRE = regexp.MustCompile(`\{\{\s*([A-Za-z_]+)(?::([^}]+))?\s*\}\}`)
var organizeTemplateIfRE = regexp.MustCompile(`(?s)\{%\s*if\s+([A-Za-z_]+)\s*%\}(.*?)\{%\s*endif\s*%\}`)

func renderOrganizeNamingTemplate(template string, data organizeNamingData) string {
	template = renderOrganizeConditionals(template, data)
	template = organizeTemplateMustacheRE.ReplaceAllStringFunc(template, func(token string) string {
		matches := organizeTemplateMustacheRE.FindStringSubmatch(token)
		if len(matches) == 0 {
			return token
		}
		format := ""
		if len(matches) > 2 {
			format = matches[2]
		}
		return organizeTemplateValue(matches[1], format, data, token)
	})
	return organizeTemplateTokenRE.ReplaceAllStringFunc(template, func(token string) string {
		matches := organizeTemplateTokenRE.FindStringSubmatch(token)
		if len(matches) == 0 {
			return token
		}
		format := ""
		if len(matches) > 2 {
			format = matches[2]
		}
		return organizeTemplateValue(matches[1], format, data, token)
	})
}

func renderOrganizeConditionals(template string, data organizeNamingData) string {
	for {
		next := organizeTemplateIfRE.ReplaceAllStringFunc(template, func(token string) string {
			matches := organizeTemplateIfRE.FindStringSubmatch(token)
			if len(matches) < 3 {
				return token
			}
			if organizeTemplateTruthy(matches[1], data) {
				return matches[2]
			}
			return ""
		})
		if next == template {
			return next
		}
		template = next
	}
}

func organizeTemplateTruthy(name string, data organizeNamingData) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "title", "name":
		return data.Title != ""
	case "year":
		return data.Year > 0
	case "season":
		return data.Season >= 0 && data.Episode > 0
	case "episode", "ep":
		return data.Episode > 0
	case "ext", "extension":
		return data.Ext != ""
	case "fileext", "file_ext":
		return data.FileExt != ""
	case "category":
		return data.Category != ""
	case "type", "media_type":
		return data.MediaType != ""
	case "episode_tag", "episodetag", "season_episode":
		return data.EpisodeTag != ""
	case "videoformat", "video_format":
		return data.VideoFormat != ""
	case "part":
		return data.Part != ""
	default:
		return false
	}
}

func organizeTemplateValue(name, format string, data organizeNamingData, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "title", "name":
		return data.Title
	case "year":
		if data.Year <= 0 {
			return ""
		}
		return strconv.Itoa(data.Year)
	case "season":
		return formatOrganizeNumberAllowZero(data.Season, format, 1)
	case "episode", "ep":
		return formatOrganizeNumber(data.Episode, format, 1)
	case "ext", "extension":
		return data.Ext
	case "fileext", "file_ext":
		return data.FileExt
	case "category":
		return data.Category
	case "type", "media_type":
		return data.MediaType
	case "episode_tag", "episodetag", "season_episode":
		return data.EpisodeTag
	case "videoformat", "video_format":
		return data.VideoFormat
	case "part":
		return data.Part
	default:
		return fallback
	}
}

var organizeReleaseTailRE = regexp.MustCompile(`(?i)(?:^|[.\s_\-\[\(])((?:2160p|1080p|720p|480p|4k|uhd|fhd|web[\s._-]*dl|web[\s._-]*rip|bluray|blu[\s._-]*ray|bdrip|hdtv|remux|x26[45]|h[\s._-]*26[45]|hevc|avc|10bit|8bit|hdr10?\+?|dovi|dv|120fps|60fps|dts|ddp?|eac3|aac|flac|truehd|atmos).*)$`)

func extractOrganizeReleaseTag(source string) string {
	base := strings.TrimSuffix(filepath.Base(strings.TrimSpace(source)), filepath.Ext(source))
	if base == "" {
		return ""
	}
	matches := organizeReleaseTailRE.FindStringSubmatch(base)
	if len(matches) < 2 {
		return ""
	}
	value := strings.Trim(matches[1], " ._-[]()")
	value = patSEnE.ReplaceAllString(value, " ")
	value = patDanglingSE.ReplaceAllString(value, " ")
	value = patNxE.ReplaceAllString(value, " ")
	value = patEP.ReplaceAllString(value, " ")
	value = patCN.ReplaceAllString(value, " ")
	if value == "" {
		return ""
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ' ', '_', '[', ']', '(', ')', '{', '}':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(part, ".-")
		if part != "" {
			out = append(out, part)
		}
	}
	return sanitizeFilename(strings.Join(out, "."))
}

func formatOrganizeNumber(value int, format string, fallback int) string {
	if value <= 0 {
		value = fallback
	}
	format = strings.TrimSpace(strings.TrimSuffix(format, "d"))
	if strings.HasPrefix(format, "0") {
		width, err := strconv.Atoi(strings.TrimPrefix(format, "0"))
		if err == nil && width > 0 {
			return fmt.Sprintf("%0*d", width, value)
		}
	}
	return strconv.Itoa(value)
}

func formatOrganizeNumberAllowZero(value int, format string, fallback int) string {
	if value < 0 {
		value = fallback
	}
	format = strings.TrimSpace(strings.TrimSuffix(format, "d"))
	if strings.HasPrefix(format, "0") {
		width, err := strconv.Atoi(strings.TrimPrefix(format, "0"))
		if err == nil && width > 0 {
			return fmt.Sprintf("%0*d", width, value)
		}
	}
	return strconv.Itoa(value)
}

func cleanOrganizeRelativePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = sanitizeFilename(strings.TrimSpace(part))
		part = strings.Trim(part, ". ")
		if part == "" || part == "." || part == ".." {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return ""
	}
	return filepath.Join(out...)
}
