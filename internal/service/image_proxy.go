// Package service — image proxy.
//
// Some deployments cannot reach image.tmdb.org directly (GFW, internal-only
// networks). ImageProxy fronts a remote image URL so the browser only ever
// talks to the MediaStationGo origin. The proxy:
//
//   - validates the URL scheme is http/https,
//   - streams bytes through with a small disk cache under cache/images,
//   - falls back to a transparent 1×1 PNG on upstream failure so the UI
//     never breaks layout,
//   - honors HTTP(S)_PROXY environment variables so users behind GFW can
//     route image fetches through their proxy.
package service

import (
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// ImageProxy fetches and caches remote images on behalf of the browser.
type ImageProxy struct {
	cfg      *config.Config
	log      *zap.Logger
	client   *http.Client
	cacheDir string
	mu       sync.Mutex

	// libraryRootsFn returns the configured media library roots so that
	// sidecar poster/artwork files stored alongside media (under arbitrary
	// per-library paths) are allowed by isAllowedLocalPath. It is provided
	// by the service container after construction and may be nil in tests.
	libraryRootsFn func() []string
	libRootsMu     sync.Mutex
	libRootsCache  []string
	libRootsAt     time.Time
}

const (
	imageBrowserCacheControl     = "public, max-age=2592000, immutable"
	imagePlaceholderCacheControl = "no-store"
	imageNegativeCacheTTL        = 6 * time.Hour
)

// NewImageProxy is the constructor.
func NewImageProxy(cfg *config.Config, log *zap.Logger) *ImageProxy {
	// Honor HTTP(S)_PROXY env vars so deployments behind GFW can pull
	// from image.tmdb.org via their HTTP proxy without extra config. On
	// Windows we also honor the current user's system proxy settings.
	transport := NewExternalTransport()
	return &ImageProxy{
		cfg:      cfg,
		log:      log,
		cacheDir: filepath.Join(cfg.Cache.CacheDir, "images"),
		client:   &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

// SetLibraryRootsProvider injects a callback that returns the current set of
// media library root directories. Sidecar posters live under these roots
// (which are arbitrary, user-defined, and not necessarily under the
// configured movies/tv/anime dirs), so they must be treated as allowed
// local-image locations.
func (p *ImageProxy) SetLibraryRootsProvider(fn func() []string) {
	p.libraryRootsFn = fn
}

// libraryRoots returns the cached library roots, refreshing at most every
// 30 seconds to avoid a DB hit per image request (posters load in bulk).
func (p *ImageProxy) libraryRoots() []string {
	if p.libraryRootsFn == nil {
		return nil
	}
	p.libRootsMu.Lock()
	defer p.libRootsMu.Unlock()
	if p.libRootsCache != nil && time.Since(p.libRootsAt) < 30*time.Second {
		return p.libRootsCache
	}
	p.libRootsCache = p.libraryRootsFn()
	p.libRootsAt = time.Now()
	return p.libRootsCache
}
