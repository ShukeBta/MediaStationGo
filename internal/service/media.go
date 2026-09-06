// Package service — library / media bookkeeping.
package service

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// MediaService offers high-level CRUD over libraries and media items.
type MediaService struct {
	cfg              *config.Config
	log              *zap.Logger
	repo             *repository.Container
	cache            *RuntimeCacheService
	ai               *AIService
	cloudDeleter     CloudMediaDeleter
	purgeMu          sync.Mutex
	tasks            *TaskTrackerService
	titleCleanupMu   sync.Mutex
	titleCleanupJobs map[string]*MediaTitleCleanupJob
}

type CloudMediaDeleter interface {
	DeleteCloudFile(ctx context.Context, provider, ref string) error
	PruneEmptyCloudParents(ctx context.Context, provider, ref, rootRef string) error
}

type MediaVisibility struct {
	IncludeNSFW       bool
	AllowedLibraryIDs []string
	HiddenLibraryIDs  []string
}

const maxMediaSearchLimit = 50000
const maxMediaSearchPageSize = 2000

func (v MediaVisibility) Allows(media *model.Media) bool {
	if media == nil {
		return false
	}
	if !v.IncludeNSFW && media.NSFW {
		return false
	}
	for _, id := range v.HiddenLibraryIDs {
		if id == media.LibraryID {
			return false
		}
	}
	if len(v.AllowedLibraryIDs) == 0 {
		return true
	}
	for _, id := range v.AllowedLibraryIDs {
		if id == media.LibraryID {
			return true
		}
	}
	return false
}

// NewMediaService is the constructor.
func NewMediaService(cfg *config.Config, log *zap.Logger, repo *repository.Container) *MediaService {
	if repo != nil && repo.Media != nil {
		repo.Media.SetSeriesKeyFunc(MediaSeriesKey)
	}
	return &MediaService{cfg: cfg, log: log, repo: repo}
}

func (s *MediaService) SetRuntimeCache(cache *RuntimeCacheService) *MediaService {
	if s != nil {
		s.cache = cache
	}
	return s
}

func (s *MediaService) SetTaskTracker(tasks *TaskTrackerService) *MediaService {
	if s != nil {
		s.tasks = tasks
	}
	return s
}

func (s *MediaService) SetCloudMediaDeleter(deleter CloudMediaDeleter) *MediaService {
	if s != nil {
		s.cloudDeleter = deleter
	}
	return s
}
