package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

const (
	MediaScrapeStatusTitleCleaned     = "title_cleaned"
	mediaTitleExplicitGroupingVersion = 3
	currentMediaTitleCleanupVersion   = 3
)

var (
	ErrMediaTitleCleanupLibraryMode  = errors.New("仅保留原始文件名的媒体库支持 AI 标题清洗")
	ErrMediaTitleCleanupNoCandidates = errors.New("没有待处理的标题清洗记录")
)

type MediaTitleCleanupPreview struct {
	LibraryID      string                        `json:"library_id"`
	CandidateCount int                           `json:"candidate_count"`
	BatchCount     int                           `json:"batch_count"`
	RemainingCount int                           `json:"remaining_count"`
	Groups         []MediaTitleCleanupGroup      `json:"groups"`
	Suggestions    []MediaTitleCleanupSuggestion `json:"suggestions"`
}

type MediaTitleCleanupApplyRequest struct {
	Items []MediaTitleCleanupSuggestion `json:"items"`
}

type MediaTitleCleanupApplyResult struct {
	Updated int `json:"updated"`
}

func (s *MediaService) SetAI(ai *AIService) *MediaService {
	if s != nil {
		s.ai = ai
	}
	return s
}

func (s *MediaService) PreviewTitleCleanup(ctx context.Context, libraryID string, groupLimit int) (*MediaTitleCleanupPreview, error) {
	return s.previewTitleCleanup(ctx, libraryID, groupLimit, nil)
}

type mediaTitleCleanupProgress struct {
	Stage           string
	Message         string
	CompletedGroups int
	TotalGroups     int
}

func (s *MediaService) previewTitleCleanup(
	ctx context.Context,
	libraryID string,
	groupLimit int,
	onProgress func(mediaTitleCleanupProgress),
) (*MediaTitleCleanupPreview, error) {
	lib, err := s.titleCleanupLibrary(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if s.ai == nil {
		return nil, ErrAITitleCleanupUnavailable
	}
	rows, err := s.titleCleanupCandidates(ctx, lib.ID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrMediaTitleCleanupNoCandidates
	}
	groups := selectMediaTitleCleanupGroups(rows, lib, groupLimit, 120)
	if len(groups) == 0 {
		return nil, ErrMediaTitleCleanupNoCandidates
	}
	if onProgress != nil {
		onProgress(mediaTitleCleanupProgress{
			Stage: "cleaning", Message: "正在清洗标题和年份", TotalGroups: len(groups),
		})
	}
	suggestions, err := s.ai.CleanMediaTitlesWithProgress(ctx, groups, func(stage string, completed, total int) {
		if onProgress != nil {
			onProgress(mediaTitleCleanupProgress{
				Stage: stage, Message: fmt.Sprintf("标题清洗 %d/%d", completed, total),
				CompletedGroups: completed, TotalGroups: total,
			})
		}
	})
	if err != nil {
		return nil, err
	}
	if onProgress != nil {
		onProgress(mediaTitleCleanupProgress{
			Stage: "validating", Message: "正在校验标题清洗结果", CompletedGroups: len(groups), TotalGroups: len(groups),
		})
	}
	batchCount := 0
	for _, group := range groups {
		batchCount += len(group.Items)
	}
	return &MediaTitleCleanupPreview{
		LibraryID:      lib.ID,
		CandidateCount: len(rows),
		BatchCount:     batchCount,
		RemainingCount: len(rows) - batchCount,
		Groups:         groups,
		Suggestions:    suggestions,
	}, nil
}

func (s *MediaService) ApplyTitleCleanup(ctx context.Context, libraryID string, req MediaTitleCleanupApplyRequest) (*MediaTitleCleanupApplyResult, error) {
	lib, err := s.titleCleanupLibrary(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if len(req.Items) == 0 {
		return nil, errors.New("至少选择一条标题清洗结果")
	}
	ids := make([]string, 0, len(req.Items))
	for _, item := range req.Items {
		id := strings.TrimSpace(item.MediaID)
		if id == "" {
			return nil, errors.New("media_id 不能为空")
		}
		ids = append(ids, id)
	}

	var rows []model.Media
	if err := s.repo.DB.WithContext(ctx).
		Where("library_id = ? AND id IN ? AND deleted_at IS NULL", lib.ID, ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(ids) {
		return nil, errors.New("部分媒体不存在或不属于当前媒体库")
	}
	for _, row := range rows {
		if !mediaEligibleForTitleCleanup(row) {
			return nil, fmt.Errorf("媒体已不再满足标题清洗条件: %s", row.ID)
		}
	}
	groups := selectMediaTitleCleanupGroups(rows, lib, len(rows), len(rows))
	validated, err := validateMediaTitleCleanupSuggestions(groups, req.Items)
	if err != nil {
		return nil, err
	}

	byID := make(map[string]MediaTitleCleanupSuggestion, len(validated))
	for _, item := range validated {
		byID[item.MediaID] = item
	}
	err = s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			item := byID[row.ID]
			updates := map[string]any{
				"title":                 item.Title,
				"year":                  item.Year,
				"scrape_status":         MediaScrapeStatusTitleCleaned,
				"title_cleanup_version": currentMediaTitleCleanupVersion,
			}
			if err := tx.Model(&model.Media{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := s.repo.Media.RefreshSearchAliases(ctx, row.ID); err != nil {
			return nil, err
		}
	}
	s.invalidateMediaCache(ctx)
	return &MediaTitleCleanupApplyResult{Updated: len(rows)}, nil
}

func (s *MediaService) titleCleanupLibrary(ctx context.Context, libraryID string) (*model.Library, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil || s.repo.Library == nil {
		return nil, errors.New("media service unavailable")
	}
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, errors.New("library id required")
	}
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if !libraryPreservesSourceTitle(lib) {
		return nil, ErrMediaTitleCleanupLibraryMode
	}
	return lib, nil
}

func (s *MediaService) titleCleanupCandidates(ctx context.Context, libraryID string) ([]model.Media, error) {
	var rows []model.Media
	err := s.repo.DB.WithContext(ctx).
		Where("library_id = ? AND deleted_at IS NULL", libraryID).
		Where("(LOWER(COALESCE(scrape_status, '')) IN ? OR (LOWER(COALESCE(scrape_status, '')) = ? AND COALESCE(title_cleanup_version, 0) < ?))",
			[]string{"", "pending", "no_match"}, MediaScrapeStatusTitleCleaned, currentMediaTitleCleanupVersion).
		Where("COALESCE(tm_db_id, 0) = 0 AND COALESCE(bangumi_id, 0) = 0").
		Where("COALESCE(douban_id, '') = '' AND COALESCE(thetvdb_id, '') = ''").
		Order("created_at DESC, path ASC").
		Limit(1000).
		Find(&rows).Error
	return rows, err
}

func mediaEligibleForTitleCleanup(media model.Media) bool {
	status := strings.ToLower(strings.TrimSpace(media.ScrapeStatus))
	statusEligible := status == "" || status == "pending" || status == "no_match" ||
		(status == MediaScrapeStatusTitleCleaned && media.TitleCleanupVersion < currentMediaTitleCleanupVersion)
	return statusEligible &&
		media.TMDbID == 0 && media.BangumiID == 0 &&
		strings.TrimSpace(media.DoubanID) == "" && strings.TrimSpace(media.TheTVDBID) == ""
}

func selectMediaTitleCleanupGroups(rows []model.Media, lib *model.Library, groupLimit, itemLimit int) []MediaTitleCleanupGroup {
	if groupLimit <= 0 {
		groupLimit = 12
	}
	if groupLimit > 30 {
		groupLimit = 30
	}
	if itemLimit <= 0 {
		itemLimit = 120
	}
	type grouped struct {
		key   string
		group MediaTitleCleanupGroup
	}
	ordered := make([]grouped, 0)
	indexes := map[string]int{}
	for _, row := range rows {
		key, sourceDirectory, directoryChain := mediaTitleCleanupDirectory(row.Path, lib.Path)
		if idx, ok := indexes[key]; ok {
			ordered[idx].group.Items = append(ordered[idx].group.Items, mediaTitleCleanupSource(row, sourceDirectory, directoryChain))
			continue
		}
		indexes[key] = len(ordered)
		ordered = append(ordered, grouped{
			key: key,
			group: MediaTitleCleanupGroup{
				SourceDirectory: sourceDirectory,
				Items:           []MediaTitleCleanupSource{mediaTitleCleanupSource(row, sourceDirectory, directoryChain)},
				DirectoryKey:    key,
			},
		})
	}
	out := make([]MediaTitleCleanupGroup, 0, groupLimit)
	itemCount := 0
	for _, entry := range ordered {
		if len(out) >= groupLimit {
			break
		}
		if itemCount > 0 && itemCount+len(entry.group.Items) > itemLimit {
			break
		}
		sort.SliceStable(entry.group.Items, func(i, j int) bool {
			return entry.group.Items[i].Filename < entry.group.Items[j].Filename
		})
		out = append(out, entry.group)
		itemCount += len(entry.group.Items)
	}
	return out
}

func mediaTitleCleanupSource(media model.Media, sourceDirectory, directoryChain string) MediaTitleCleanupSource {
	return MediaTitleCleanupSource{
		MediaID:         media.ID,
		CurrentTitle:    media.Title,
		SourceDirectory: sourceDirectory,
		DirectoryChain:  directoryChain,
		Filename:        pathBaseSlash(media.Path),
	}
}

func mediaTitleCleanupDirectory(mediaPath, libraryPath string) (key, sourceDirectory, directoryChain string) {
	mediaPath = cleanSlashPath(mediaPath)
	root := comparableLibraryRoot(libraryPath)
	relative := strings.TrimPrefix(mediaPath, root)
	relative = strings.TrimLeft(relative, "/")
	parts := strings.Split(relative, "/")
	if len(parts) <= 1 {
		filename := pathBaseSlash(mediaPath)
		return mediaPath, sourceFilenameTitle(filename), ""
	}
	directories := parts[:len(parts)-1]
	sourceDirectory = strings.TrimSpace(directories[0])
	if sourceDirectory == "" {
		sourceDirectory = sourceFilenameTitle(parts[len(parts)-1])
	}
	directoryChain = strings.Join(directories, " / ")
	return root + "/" + sourceDirectory, sourceDirectory, directoryChain
}
