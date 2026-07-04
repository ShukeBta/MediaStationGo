package service

import (
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// ScraperService coordinates metadata enrichment across providers.
type ScraperService struct {
	cfg     *config.Config
	log     *zap.Logger
	repo    *repository.Container
	tmdb    *TMDbProvider
	bangumi *BangumiProvider
	thetvdb *TheTVDBProvider
	douban  *DoubanProvider
	fanart  *FanartProvider
	adult   *AdultProvider
	hub     *Hub
	notify  *NotifyChannelService
	cache   *RuntimeCacheService
	images  *ImageProxy
}

// NewScraperService is the constructor.
func NewScraperService(
	cfg *config.Config,
	log *zap.Logger,
	repo *repository.Container,
	tmdb *TMDbProvider,
	bangumi *BangumiProvider,
	thetvdb *TheTVDBProvider,
	fanart *FanartProvider,
	hub *Hub,
	adult ...*AdultProvider,
) *ScraperService {
	var adultProvider *AdultProvider
	if len(adult) > 0 {
		adultProvider = adult[0]
	}
	return &ScraperService{
		cfg: cfg, log: log, repo: repo,
		tmdb: tmdb, bangumi: bangumi, thetvdb: thetvdb, fanart: fanart, adult: adultProvider, hub: hub,
	}
}

func (s *ScraperService) SetDouban(douban *DoubanProvider) {
	s.douban = douban
}

func (s *ScraperService) SetNotifyChannels(notify *NotifyChannelService) {
	if s != nil {
		s.notify = notify
	}
}

func (s *ScraperService) SetRuntimeCache(cache *RuntimeCacheService) *ScraperService {
	if s != nil {
		s.cache = cache
	}
	return s
}

func (s *ScraperService) SetImageProxy(images *ImageProxy) *ScraperService {
	if s != nil {
		s.images = images
	}
	return s
}

var tmdbDetailsTimeout = 8 * time.Second

const (
	defaultScrapeDelayMinMS = 250
	defaultScrapeDelayMaxMS = 500
	maxScrapeDelayMS        = 5 * 60 * 1000
)

// AnyEnabled reports whether at least one provider can run.
func (s *ScraperService) AnyEnabled() bool {
	if s.tmdb != nil && s.tmdb.Enabled() {
		return true
	}
	if s.bangumi != nil && s.bangumi.Enabled() {
		return true
	}
	if s.thetvdb != nil && s.thetvdb.Enabled() {
		return true
	}
	if s.adult != nil && s.adult.Enabled() {
		return true
	}
	if s.douban != nil && s.douban.Enabled() {
		return true
	}
	return false
}

// determineMediaType returns "tv" for TV shows and "movie" for movies.
// It uses the library type as the primary signal.
func (s *ScraperService) determineMediaType(lib *model.Library, match *Match) string {
	return s.determineMediaTypeForMedia(lib, nil, match)
}

func (s *ScraperService) determineMediaTypeForMedia(lib *model.Library, media *model.Media, match *Match) string {
	if match != nil {
		switch normalizeOrganizeMediaType(match.MediaType) {
		case "tv", "anime", "variety":
			return "tv"
		case "movie", "adult":
			return "movie"
		}
	}
	if media != nil && mediaIsEpisodic(media, lib) {
		return "tv"
	}
	if lib != nil {
		switch lib.Type {
		case "tv", "anime", "variety", "show", "shows":
			return "tv"
		}
	}
	// Fallback: if Bangumi ID is present, treat as TV/anime.
	if match != nil && match.BangumiID > 0 {
		return "tv"
	}
	return "movie"
}
