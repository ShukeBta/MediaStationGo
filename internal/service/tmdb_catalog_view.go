package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type TMDbCatalogEpisode struct {
	model.TMDbCatalogItem
	MediaID   string `json:"media_id,omitempty"`
	Available bool   `json:"available"`
}

type TMDbCatalogSeason struct {
	model.TMDbCatalogItem
	Episodes []TMDbCatalogEpisode  `json:"episodes"`
	Recheck  *model.TMDbCatalogJob `json:"recheck,omitempty"`
}

type TMDbSeriesCatalog struct {
	Series  *model.TMDbCatalogItem `json:"series,omitempty"`
	Seasons []TMDbCatalogSeason    `json:"seasons"`
}

func (s *TMDbCatalogService) SeriesCatalog(ctx context.Context, media *model.Media, visible func(*model.Media) bool) (*TMDbSeriesCatalog, error) {
	items, err := s.catalog.Series(ctx, media.TMDbID)
	if err != nil {
		return nil, err
	}
	result := &TMDbSeriesCatalog{Seasons: []TMDbCatalogSeason{}}
	seasons := map[int]*TMDbCatalogSeason{}
	ensureSeason := func(number int) *TMDbCatalogSeason {
		if seasons[number] == nil {
			seasons[number] = &TMDbCatalogSeason{TMDbCatalogItem: model.TMDbCatalogItem{Key: fmt.Sprintf("tv/%d/season/%d", media.TMDbID, number), Kind: "season", SeasonNum: number, RootID: media.TMDbID, Title: fmt.Sprintf("第 %d 季", number)}, Episodes: []TMDbCatalogEpisode{}}
		}
		return seasons[number]
	}
	for _, item := range items {
		switch item.Kind {
		case "series":
			copy := item
			result.Series = &copy
		case "season":
			season := ensureSeason(item.SeasonNum)
			season.TMDbCatalogItem = item
			season.Recheck, err = s.catalog.State(ctx, item.Key)
			if err != nil {
				return nil, err
			}
		case "episode":
			ensureSeason(item.SeasonNum).Episodes = append(ensureSeason(item.SeasonNum).Episodes, TMDbCatalogEpisode{TMDbCatalogItem: item})
		}
	}
	var local []model.Media
	err = s.repos.DB.WithContext(ctx).Where("library_id = ? AND tm_db_id = ? AND episode_num > 0", media.LibraryID, media.TMDbID).Order("id").Find(&local).Error
	if err != nil {
		return nil, err
	}
	for i := range local {
		entry := &local[i]
		if visible != nil && !visible(entry) {
			continue
		}
		season := ensureSeason(entry.SeasonNum)
		end := max(entry.EpisodeNum, entry.EpisodeEndNum)
		// Bad imported ranges must not allocate an unbounded catalog.
		end = min(end, entry.EpisodeNum+1000)
		for number := entry.EpisodeNum; number <= end; number++ {
			attachCatalogMedia(season, entry, number)
		}
	}
	for _, season := range seasons {
		if season.Recheck == nil {
			season.Recheck, err = s.catalog.State(ctx, season.Key)
			if err != nil {
				return nil, err
			}
		}
		sort.Slice(season.Episodes, func(i, j int) bool { return season.Episodes[i].EpisodeNum < season.Episodes[j].EpisodeNum })
		result.Seasons = append(result.Seasons, *season)
	}
	sort.Slice(result.Seasons, func(i, j int) bool { return result.Seasons[i].SeasonNum < result.Seasons[j].SeasonNum })
	return result, nil
}

func attachCatalogMedia(season *TMDbCatalogSeason, media *model.Media, number int) {
	for i := range season.Episodes {
		if season.Episodes[i].EpisodeNum == number {
			if !season.Episodes[i].Available {
				season.Episodes[i].Available, season.Episodes[i].MediaID = true, media.ID
			}
			return
		}
	}
	season.Episodes = append(season.Episodes, TMDbCatalogEpisode{Available: true, MediaID: media.ID, TMDbCatalogItem: model.TMDbCatalogItem{
		Kind: "episode", RootID: media.TMDbID, SeasonNum: media.SeasonNum, EpisodeNum: number, Title: media.EpisodeTitle, Overview: media.Overview, ReleaseDate: media.ReleaseDate, StillURL: media.BackdropURL,
	}})
}

func (s *TMDbCatalogService) StartSeriesRefresh(ctx context.Context, rootID int) (string, error) {
	if s.tasks == nil {
		return "", fmt.Errorf("task tracker unavailable")
	}
	task, started := s.tasks.StartUnique(fmt.Sprintf("tmdb_catalog_%d", rootID), "TMDb 完整季集目录更新", TaskUpdate{Stage: "catalog", Message: "正在获取全部季集目录"})
	if !started {
		return "", ErrSchedulerJobAlreadyRunning
	}
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		err := s.RefreshSeries(ctx, rootID)
		task.Finish(err, TaskUpdate{Stage: "done", Message: "季集目录更新结束"})
	}()
	return task.ID(), nil
}
