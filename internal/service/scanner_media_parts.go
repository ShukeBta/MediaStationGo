package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var mediaPartPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(.*?)[ ._-]+[([]?(cd|dvd|part|pt|disc|disk)[ ._-]*([0-9]+|[a-d])[]) ]?$`),
	regexp.MustCompile(`(?i)^(.*?[])}])[(]?(cd|dvd|part|pt|disc|disk)[ ._-]*([0-9]+|[a-d])[)]?$`),
}

type mediaPartCandidate struct {
	baseName string
	partType string
	index    int
}

// parseMediaPartCandidate 只识别文件名末尾、边界明确的标准 multipart 标记。
func parseMediaPartCandidate(path string) (mediaPartCandidate, bool) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for _, pattern := range mediaPartPatterns {
		match := pattern.FindStringSubmatch(name)
		if len(match) != 4 {
			continue
		}
		baseName := strings.TrimSpace(strings.TrimRight(match[1], " ._-"))
		index, ok := mediaPartIndex(match[3])
		if baseName == "" || !ok {
			return mediaPartCandidate{}, false
		}
		return mediaPartCandidate{baseName: baseName, partType: strings.ToLower(match[2]), index: index}, true
	}
	return mediaPartCandidate{}, false
}

func mediaPartIndex(value string) (int, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) == 1 && value[0] >= 'a' && value[0] <= 'd' {
		return int(value[0]-'a') + 1, true
	}
	index, err := strconv.Atoi(value)
	return index, err == nil && index > 0
}

func mediaPartCandidateKey(libraryID, path string, candidate mediaPartCandidate) string {
	raw := strings.Join([]string{
		strings.TrimSpace(libraryID),
		filepath.Clean(filepath.Dir(path)),
		strings.ToLower(candidate.baseName),
		candidate.partType,
	}, "\x00")
	sum := sha256.Sum256([]byte(raw))
	return autoMediaPartPrefix + hex.EncodeToString(sum[:])
}

func mediaPartBasePath(path string, candidate mediaPartCandidate) string {
	return filepath.Join(filepath.Dir(path), candidate.baseName+filepath.Ext(path))
}

// activeMediaPartCandidate 要求同目录至少有两个不同序号，避免把作品名中的 Part 1 当成文件分段。
func activeMediaPartCandidate(libraryID, path string) (mediaPartCandidate, string, bool) {
	candidate, ok := parseMediaPartCandidate(path)
	if !ok {
		return mediaPartCandidate{}, "", false
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return mediaPartCandidate{}, "", false
	}
	// ponytail: 只为 multipart 候选读取同目录；候选密集目录出现实测瓶颈时再下沉扫描批次索引。
	indexes := map[int]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if _, video := videoExtensions[ext]; !video {
			continue
		}
		other, matched := parseMediaPartCandidate(filepath.Join(filepath.Dir(path), entry.Name()))
		if !matched || !strings.EqualFold(other.baseName, candidate.baseName) || other.partType != candidate.partType {
			continue
		}
		if _, duplicate := indexes[other.index]; duplicate {
			return mediaPartCandidate{}, "", false
		}
		indexes[other.index] = struct{}{}
	}
	if len(indexes) < 2 {
		return mediaPartCandidate{}, "", false
	}
	return candidate, mediaPartCandidateKey(libraryID, path, candidate), true
}

const autoMediaPartPrefix = "auto-part:"

// Reconcile only groups owned by the scanner. Explicit manual groups and
// alternate-version assignments keep their existing presentation.
func (s *ScannerService) reconcileMediaParts(ctx context.Context, libraryID, directory string) (int, error) {
	var rows []model.Media
	if err := s.repo.DB.WithContext(ctx).Where("library_id = ? AND path NOT LIKE ?", libraryID, "cloud://%").Find(&rows).Error; err != nil {
		return 0, err
	}
	type partRow struct {
		media     model.Media
		candidate mediaPartCandidate
		key       string
	}
	candidates := make([]partRow, 0)
	groups := make(map[string]map[int]bool)
	invalid := make(map[string]bool)
	for _, row := range rows {
		if directory != "" && !sameLibraryPath(filepath.Dir(row.Path), directory) && !pathWithin(filepath.Dir(row.Path), directory) {
			continue
		}
		if row.PartGroupKey != "" && !strings.HasPrefix(row.PartGroupKey, autoMediaPartPrefix) {
			continue
		}
		if row.VersionGroupKey != "" {
			continue
		}
		part, ok := parseMediaPartCandidate(row.Path)
		key := ""
		if ok {
			key = mediaPartCandidateKey(libraryID, row.Path, part)
			if groups[key] == nil {
				groups[key] = make(map[int]bool)
			}
			if groups[key][part.index] {
				invalid[key] = true
			}
			groups[key][part.index] = true
		}
		if ok || strings.HasPrefix(row.PartGroupKey, autoMediaPartPrefix) {
			candidates = append(candidates, partRow{row, part, key})
		}
	}
	changed := 0
	err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := make([]string, 0)
		for _, row := range candidates {
			key, index, title := "", 0, ""
			if row.key != "" && len(groups[row.key]) >= 2 && !invalid[row.key] {
				key, index, title = row.key, row.candidate.index, row.candidate.baseName
			}
			if row.media.PartGroupKey == key && row.media.PartIndex == index && row.media.PartGroupTitle == title {
				continue
			}
			if err := tx.Model(&model.Media{}).Where("id = ? AND part_group_key = ?", row.media.ID, row.media.PartGroupKey).
				Updates(map[string]any{"part_group_key": key, "part_index": index, "part_group_title": title, "media_version_key_version": 0, "emby_key_version": 0}).Error; err != nil {
				return err
			}
			ids = append(ids, row.media.ID)
		}
		changed = len(ids)
		if len(ids) > 0 {
			return s.repo.Media.RefreshEmbyKeys(ctx, tx, ids)
		}
		return nil
	})
	return changed, err
}
