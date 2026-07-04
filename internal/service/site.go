// Package service — site management (PT/BT tracker CRUD + connection test).
//
// SiteService owns the lifecycle of Site rows and exposes a cross-site
// search dispatcher that fans out a keyword query to every enabled site's
// adapter, collects results and returns them merged + sorted.
package service

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// SiteService manages PT/BT site configurations.
type SiteService struct {
	log             *zap.Logger
	repo            *repository.Container
	flareSolverrURL string
	apiRateLimiter  siteAPIRateLimiter
	portalMu        sync.Mutex
	portalNext      map[string]time.Time
	portalCache     map[string]sitePortalCacheEntry
	portalCooldown  map[string]sitePortalCooldownEntry
}

// NewSiteService is the constructor.
func NewSiteService(log *zap.Logger, repo *repository.Container, flareSolverrURL string) *SiteService {
	return &SiteService{
		log:             log,
		repo:            repo,
		flareSolverrURL: flareSolverrURL,
		apiRateLimiter:  newPersistentSiteAPIRateLimiter(repo),
		portalNext:      map[string]time.Time{},
		portalCache:     map[string]sitePortalCacheEntry{},
		portalCooldown:  map[string]sitePortalCooldownEntry{},
	}
}
