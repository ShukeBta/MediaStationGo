package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type TMDbCatalogService struct {
	repos          *repository.Container
	catalog        *repository.TMDbCatalogRepository
	tmdb           *TMDbProvider
	tasks          *TaskTrackerService
	onMediaChanged func(context.Context)
}

func NewTMDbCatalogService(repos *repository.Container, tmdb *TMDbProvider, tasks *TaskTrackerService) *TMDbCatalogService {
	catalog := repository.NewTMDbCatalogRepository(repos.DB)
	if tmdb != nil {
		tmdb.SetCatalogRepository(catalog)
	}
	return &TMDbCatalogService{repos: repos, catalog: catalog, tmdb: tmdb, tasks: tasks}
}

// RunCatalogMaintenance only visits TMDB identities represented by local media.
// Every pass fixes its cutoff so a failed request cannot loop in the same run.
func (s *TMDbCatalogService) RunCatalogMaintenance(ctx context.Context, missingOnly bool) (resultErr error) {
	if s == nil || s.tmdb == nil || !s.tmdb.Enabled() {
		return nil
	}
	name := "TMDb 季集复查"
	if missingOnly {
		name = "TMDb 快照补全"
	}
	metrics := map[string]int64{}
	var task *TaskHandle
	if s.tasks != nil {
		var started bool
		task, started = s.tasks.StartUnique("tmdb_catalog", name, TaskUpdate{Stage: "catalog", Message: "正在核对 TMDb 目录与快照"})
		if !started {
			return ErrSchedulerJobAlreadyRunning
		}
		defer func() {
			task.Finish(resultErr, TaskUpdate{Stage: "done", Metrics: metrics, Message: "TMDb 目录核对结束"})
		}()
	}
	var roots []struct {
		TMDbID   int
		Episodic int
	}
	err := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Select("DISTINCT media.tm_db_id, CASE WHEN media.episode_num > 0 OR libraries.type IN ('tv','anime') THEN 1 ELSE 0 END AS episodic").
		Joins("JOIN libraries ON libraries.id = media.library_id AND libraries.deleted_at IS NULL").
		Where("media.tm_db_id > 0").Order("media.tm_db_id").Scan(&roots).Error
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC()
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return err
		}
		if root.Episodic == 0 && !missingOnly {
			continue
		}
		key := fmt.Sprintf("movie/%d", root.TMDbID)
		if root.Episodic != 0 {
			key = fmt.Sprintf("tv/%d", root.TMDbID)
		}
		err := s.recheckCatalogKey(ctx, key, cutoff, false, missingOnly, metrics)
		if err == nil && root.Episodic != 0 {
			err = s.recheckSeasons(ctx, root.TMDbID, cutoff, false, missingOnly, metrics)
		}
		if err != nil {
			metrics["errors"]++
			resultErr = errors.Join(resultErr, err)
		}
		metrics["processed"]++
		if task != nil {
			task.Update(TaskUpdate{Stage: "catalog", Metrics: metrics, Message: fmt.Sprintf("已核对 %d 个作品，更新 %d 个快照，错误 %d", metrics["processed"], metrics["updated"], metrics["errors"])})
		}
	}
	return resultErr
}

func (s *TMDbCatalogService) RefreshSeries(ctx context.Context, rootID int) error {
	if rootID <= 0 {
		return errors.New("invalid TMDB series ID")
	}
	metrics, cutoff := map[string]int64{}, time.Now().UTC()
	if err := s.recheckCatalogKey(ctx, fmt.Sprintf("tv/%d", rootID), cutoff, true, false, metrics); err != nil {
		return err
	}
	return s.recheckSeasons(ctx, rootID, cutoff, true, false, metrics)
}

func (s *TMDbCatalogService) recheckSeasons(ctx context.Context, rootID int, cutoff time.Time, force, missingOnly bool, metrics map[string]int64) error {
	items, err := s.catalog.Series(ctx, rootID)
	if err != nil {
		return err
	}
	known := map[int]bool{}
	for _, item := range items {
		if item.Kind == "season" {
			known[item.SeasonNum] = true
		}
	}
	var localSeasons []int
	if err := s.repos.DB.WithContext(ctx).Model(&model.Media{}).Where("tm_db_id = ? AND episode_num > 0 AND season_num >= 0", rootID).Distinct().Pluck("season_num", &localSeasons).Error; err != nil {
		return err
	}
	for _, season := range localSeasons {
		if !known[season] {
			items = append(items, model.TMDbCatalogItem{Kind: "season", Key: fmt.Sprintf("tv/%d/season/%d", rootID, season)})
		}
	}
	var resultErr error
	for _, item := range items {
		if item.Kind != "season" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.recheckCatalogKey(ctx, item.Key, cutoff, force, missingOnly, metrics); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}
	return resultErr
}

func (s *TMDbCatalogService) recheckCatalogKey(ctx context.Context, key string, cutoff time.Time, force, missingOnly bool, metrics map[string]int64) error {
	if s.tmdb == nil {
		return errors.New("TMDB provider unavailable")
	}
	item, err := s.catalog.Find(ctx, key)
	if err != nil {
		return err
	}
	if missingOnly && item != nil && item.Complete {
		return nil
	}
	job, err := s.catalog.Claim(ctx, key, cutoff, force)
	if err != nil || job == nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	err = s.tmdb.fetchCatalog(requestCtx, key)
	cancel()
	metrics["requests"]++
	status, reason, success := "pending", "", err == nil
	cooldown := s.catalogCooldown(ctx, key)
	if err != nil {
		reason, status = err.Error(), "retry"
		cooldown = min(5*time.Minute*time.Duration(1<<min(job.Attempts, 8)), 24*time.Hour)
		if strings.Contains(err.Error(), "HTTP 404") {
			status, cooldown = "not_found", s.catalogCooldown(ctx, key)
			reason = "TMDb 未找到该季/作品，已保留本地文件，等待下次复核"
			metrics["not_found"]++
			err = nil
		}
	} else {
		metrics["updated"]++
		if item, findErr := s.catalog.Find(ctx, key); findErr != nil {
			err = findErr
		} else if item != nil && item.Kind == "season" {
			err = s.fillLocalEpisodeMetadata(ctx, item.RootID, item.SeasonNum)
		}
		if err != nil {
			success, status, reason, cooldown = false, "retry", err.Error(), 5*time.Minute
		}
	}
	// A canceled request must release its lease without borrowing the canceled context.
	cleanup, finish := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finish()
	finishErr := s.catalog.Finish(cleanup, job, status, reason, time.Now().UTC().Add(cooldown), success)
	return errors.Join(err, finishErr)
}

func (s *TMDbCatalogService) catalogCooldown(ctx context.Context, key string) time.Duration {
	item, err := s.catalog.Find(ctx, key)
	if err != nil || item == nil || item.Kind != "season" {
		return 3 * 24 * time.Hour
	}
	items, err := s.catalog.Series(ctx, item.RootID)
	if err != nil {
		return 3 * 24 * time.Hour
	}
	dates := []string{}
	for _, entry := range items {
		if entry.Kind == "episode" && entry.SeasonNum == item.SeasonNum {
			dates = append(dates, entry.ReleaseDate)
		}
	}
	return TMDbSeasonCooldown(item.ReleaseDate, dates, time.Now().UTC())
}

// Fill only blanks with identity and empty-value predicates in the same UPDATE.
// Concurrent manual edits or a TMDB rematch cannot be overwritten by this pass.
func (s *TMDbCatalogService) fillLocalEpisodeMetadata(ctx context.Context, rootID, season int) error {
	changed := false
	defer func() {
		if changed && s.onMediaChanged != nil {
			s.onMediaChanged(ctx)
		}
	}()
	items, err := s.catalog.Series(ctx, rootID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Kind != "episode" || item.SeasonNum != season {
			continue
		}
		values := map[string]string{"episode_title": item.Title, "overview": item.Overview, "release_date": item.ReleaseDate, "backdrop_url": item.StillURL}
		for field, value := range values {
			if strings.TrimSpace(value) == "" {
				continue
			}
			result := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
				Where("tm_db_id = ? AND season_num = ? AND episode_num = ?", rootID, season, item.EpisodeNum).
				Where("("+field+" IS NULL OR "+field+" = '')").Update(field, value)
			if result.Error != nil {
				return result.Error
			}
			changed = changed || result.RowsAffected > 0
		}
	}
	return nil
}
