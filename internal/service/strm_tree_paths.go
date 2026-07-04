package service

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

func strmTreeRelativeSource(source, sourceRoot string) string {
	source = normalizeSTRMTreeSource(source)
	root := normalizeRemotePath(sourceRoot)
	if root != "/" && strings.HasPrefix(source, root+"/") {
		return strings.TrimPrefix(strings.TrimPrefix(source, root), "/")
	}
	return strings.TrimPrefix(source, "/")
}

func strmTreeCloudRef(source, sourceRoot string) string {
	source = normalizeSTRMTreeSource(source)
	if strings.HasPrefix(source, "/") {
		return source
	}
	if strings.TrimSpace(sourceRoot) != "" {
		return joinRemotePath(sourceRoot, source)
	}
	return normalizeRemotePath(source)
}

func strmTreeOutputRelativePath(source string) (string, error) {
	return strmTreeOutputRelativePathWithLinkExtension(source, videoExtensions, ".strm", false)
}

func (s *STRMService) strmTreeRecognizedOutputRelativePath(ctx context.Context, source, sourceRoot string) (string, error) {
	source = normalizeSTRMTreeSource(source)
	season, episode := ParseEpisode(source)
	title, year := s.strmTreeCleanQuery(ctx, source)
	if season > 0 || episode > 0 {
		show := s.strmTreeRecognizedSeriesTitle(ctx, source, sourceRoot, title)
		if show == "" {
			return "", errors.New("empty recognized series title")
		}
		if season <= 0 {
			season = 1
		}
		if episode <= 0 {
			return "", errors.New("missing recognized episode number")
		}
		name := fmt.Sprintf("%s S%02dE%02d.strm", show, season, episode)
		return filepath.Join(sanitizeFilename(show), fmt.Sprintf("Season %02d", season), sanitizeFilename(name)), nil
	}
	if title == "" {
		return "", errors.New("empty recognized movie title")
	}
	movie := sanitizeFilename(titleCaseWords(title))
	if movie == "" {
		return "", errors.New("empty recognized movie filename")
	}
	folder := movie
	if year > 0 && !strings.Contains(folder, strconv.Itoa(year)) {
		folder = fmt.Sprintf("%s (%d)", movie, year)
	}
	return filepath.Join(sanitizeFilename(folder), sanitizeFilename(folder)+".strm"), nil
}

func (s *STRMService) strmTreeRecognizedSeriesTitle(ctx context.Context, source, sourceRoot, fallback string) string {
	rel := strmTreeRelativeSource(source, sourceRoot)
	parts := strings.Split(strings.Trim(strings.ReplaceAll(rel, "\\", "/"), "/"), "/")
	if len(parts) >= 2 {
		dir := parts[len(parts)-2]
		if strings.TrimSpace(dir) != "" && !seriesSeasonDirRE.MatchString(dir) {
			if title, _ := s.strmTreeCleanQuery(ctx, dir); title != "" {
				return sanitizeFilename(titleCaseWords(title))
			}
			return sanitizeFilename(strings.TrimSpace(dir))
		}
	}
	if len(parts) >= 3 {
		dir := parts[len(parts)-3]
		if title, _ := s.strmTreeCleanQuery(ctx, dir); title != "" {
			return sanitizeFilename(titleCaseWords(title))
		}
		return sanitizeFilename(strings.TrimSpace(dir))
	}
	if fallback != "" {
		return sanitizeFilename(titleCaseWords(fallback))
	}
	return ""
}

func (s *STRMService) strmTreeCleanQuery(ctx context.Context, raw string) (string, int) {
	if s == nil {
		return CleanQuery(raw)
	}
	return CleanQueryWithRecognition(ctx, s.repo, raw)
}

func strmTreeOutputSubtitleLinkRelativePath(source string) (string, error) {
	return strmTreeOutputRelativePathWithLinkExtension(source, strmTreeSubtitleExtensions, ".strm", true)
}

func strmTreeOutputRelativePathWithLinkExtension(source string, allowedExtensions map[string]struct{}, linkExtension string, appendLinkExtension bool) (string, error) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(source, "\\", "/"), "/"), "/")
	if len(parts) == 0 {
		return "", errors.New("empty source path")
	}
	out := make([]string, 0, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe path segment %q", part)
		}
		if i == len(parts)-1 {
			ext := strings.ToLower(path.Ext(part))
			if _, ok := allowedExtensions[ext]; !ok {
				return "", fmt.Errorf("unsupported media extension %q", ext)
			}
			if linkExtension != "" {
				if appendLinkExtension {
					part += linkExtension
				} else {
					part = strings.TrimSuffix(part, path.Ext(part)) + linkExtension
				}
			}
		}
		safe := sanitizeFilename(part)
		if safe == "" {
			return "", errors.New("empty sanitized path segment")
		}
		out = append(out, safe)
	}
	return filepath.Join(out...), nil
}

func strmTreeSubtitleMatchesVideo(subtitle strmTreeSource, videos []strmTreeSource) bool {
	subDir, subBase := strmTreeDirAndBase(subtitle.Path)
	if subBase == "" {
		return false
	}
	for _, video := range videos {
		if video.Kind != "" && video.Kind != strmTreeSourceKindVideo {
			continue
		}
		if !strings.EqualFold(subtitle.Provider, video.Provider) {
			continue
		}
		videoDir, videoBase := strmTreeDirAndBase(video.Path)
		if !strings.EqualFold(subDir, videoDir) || videoBase == "" {
			continue
		}
		if strings.EqualFold(subBase, videoBase) || strings.HasPrefix(strings.ToLower(subBase), strings.ToLower(videoBase)+".") {
			return true
		}
	}
	return false
}

func strmTreeDirAndBase(source string) (string, string) {
	source = normalizeSTRMTreeSource(source)
	dir := path.Dir(source)
	name := path.Base(source)
	base := strings.TrimSuffix(name, path.Ext(name))
	return strings.ToLower(strings.Trim(dir, "/")), strings.ToLower(base)
}

func strmTreeOutputPrefixPath(prefix string) (string, error) {
	parts := strings.Split(strings.Trim(strings.ReplaceAll(prefix, "\\", "/"), "/"), "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "." || part == ".." {
			return "", fmt.Errorf("unsafe output prefix segment %q", part)
		}
		safe := sanitizeFilename(part)
		if safe == "" {
			return "", errors.New("empty sanitized output prefix segment")
		}
		out = append(out, safe)
	}
	return filepath.Join(out...), nil
}
