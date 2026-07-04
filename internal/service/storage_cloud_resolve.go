package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type cloudResolveCacheEntry struct {
	link      *cloud.DirectLink
	expiresAt time.Time
	hits      int
	lastHit   time.Time
}

type cloudResolveCall struct {
	done chan struct{}
	link *cloud.DirectLink
	err  error
}

const (
	cloudResolveHotHitThreshold      = 3
	cloudResolveBackgroundRefreshMax = 30 * time.Second
)

// CloudResolve resolves a cloud file reference to a direct link.
//
// clientUA is the User-Agent of the playback client that will follow the 302
// redirect. Some provider CDN links are bound to the UA used to request them,
// so we resolve with the client's own UA. When clientUA is empty the provider's
// default UA is used.
func (s *StorageConfigService) CloudResolve(ctx context.Context, typ, fileRef, clientUA string) (*cloud.DirectLink, error) {
	if s == nil {
		return nil, errors.New("storage config service unavailable")
	}
	cacheKey := s.resolveCacheKey(typ, fileRef, clientUA)
	if link, ok, refresh := s.cachedResolve(cacheKey, typ); ok {
		if refresh {
			s.refreshResolveInBackground(cacheKey, typ, fileRef, clientUA)
		}
		return link, nil
	}
	if call, owner := s.beginResolve(cacheKey); !owner {
		select {
		case <-call.done:
			if call.err != nil {
				return nil, call.err
			}
			return cloneDirectLink(call.link), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	} else {
		defer s.finishResolve(cacheKey, call)
		p, err := s.cloudProviderWithUA(ctx, typ, clientUA)
		if err != nil {
			call.err = err
			return nil, err
		}
		link, err := p.Resolve(ctx, fileRef)
		if err != nil {
			call.err = err
			return nil, err
		}
		call.link = cloneDirectLink(link)
		s.storeResolvedLink(cacheKey, typ, link)
		return cloneDirectLink(link), nil
	}
}

func (s *StorageConfigService) resolveCacheKey(typ, fileRef, clientUA string) string {
	return strings.TrimSpace(typ) + "\x00" + strings.TrimSpace(fileRef) + "\x00" + strings.TrimSpace(clientUA)
}

func (s *StorageConfigService) cachedResolve(key, typ string) (*cloud.DirectLink, bool, bool) {
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	if s.resolveCache == nil {
		s.resolveCache = make(map[string]cloudResolveCacheEntry)
		return nil, false, false
	}
	entry, ok := s.resolveCache[key]
	now := time.Now()
	if !ok || now.After(entry.expiresAt) {
		if ok {
			delete(s.resolveCache, key)
		}
		return nil, false, false
	}
	entry.hits++
	entry.lastHit = now
	s.resolveCache[key] = entry
	refreshWindow := cloudResolveHotRefreshWindow(cloudResolveCacheTTL(typ))
	shouldRefresh := entry.hits >= cloudResolveHotHitThreshold &&
		refreshWindow > 0 &&
		now.Add(refreshWindow).After(entry.expiresAt)
	return cloneDirectLink(entry.link), true, shouldRefresh
}

func (s *StorageConfigService) beginResolve(key string) (*cloudResolveCall, bool) {
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	if s.resolveFlight == nil {
		s.resolveFlight = make(map[string]*cloudResolveCall)
	}
	if call := s.resolveFlight[key]; call != nil {
		return call, false
	}
	call := &cloudResolveCall{done: make(chan struct{})}
	s.resolveFlight[key] = call
	return call, true
}

func (s *StorageConfigService) finishResolve(key string, call *cloudResolveCall) {
	s.resolveMu.Lock()
	if current := s.resolveFlight[key]; current == call {
		delete(s.resolveFlight, key)
	}
	s.resolveMu.Unlock()
	close(call.done)
}

func (s *StorageConfigService) refreshResolveInBackground(key, typ, fileRef, clientUA string) {
	if s == nil {
		return
	}
	go func() {
		call, owner := s.beginResolve(key)
		if !owner {
			return
		}
		defer s.finishResolve(key, call)
		ctx, cancel := context.WithTimeout(context.Background(), cloudResolveBackgroundRefreshMax)
		defer cancel()
		p, err := s.cloudProviderWithUA(ctx, typ, clientUA)
		if err != nil {
			call.err = err
			if s.log != nil {
				s.log.Debug("refresh cloud direct link failed", zap.String("provider", typ), zap.Error(err))
			}
			return
		}
		link, err := p.Resolve(ctx, fileRef)
		if err != nil {
			call.err = err
			if s.log != nil {
				s.log.Debug("refresh cloud direct link failed", zap.String("provider", typ), zap.Error(err))
			}
			return
		}
		call.link = cloneDirectLink(link)
		s.storeResolvedLink(key, typ, link)
	}()
}

func (s *StorageConfigService) storeResolvedLink(key, typ string, link *cloud.DirectLink) {
	if link == nil || strings.TrimSpace(link.URL) == "" {
		return
	}
	ttl := cloudResolveCacheTTL(typ)
	if ttl <= 0 {
		return
	}
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	if s.resolveCache == nil {
		s.resolveCache = make(map[string]cloudResolveCacheEntry)
	}
	now := time.Now()
	hits := 0
	if existing, ok := s.resolveCache[key]; ok {
		hits = existing.hits
	}
	s.resolveCache[key] = cloudResolveCacheEntry{link: cloneDirectLink(link), expiresAt: now.Add(ttl), hits: hits, lastHit: now}
}

func cloudResolveHotRefreshWindow(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return 0
	}
	window := ttl / 4
	if window < 15*time.Second {
		window = 15 * time.Second
	}
	if window > 2*time.Minute {
		window = 2 * time.Minute
	}
	return window
}

func cloudResolveCacheTTL(typ string) time.Duration {
	switch typ {
	case cloud.Type115, cloud.TypeCloudDrive2, cloud.TypeOpenList:
		return 2 * time.Minute
	default:
		return 5 * time.Minute
	}
}

func cloneDirectLink(link *cloud.DirectLink) *cloud.DirectLink {
	if link == nil {
		return nil
	}
	out := &cloud.DirectLink{
		URL:     link.URL,
		Headers: make(map[string]string, len(link.Headers)),
		Proxy:   link.Proxy,
	}
	for k, v := range link.Headers {
		out.Headers[k] = v
	}
	return out
}

func (s *StorageConfigService) clearResolveCacheForType(typ string) {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		return
	}
	prefix := typ + "\x00"
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	for key := range s.resolveCache {
		if strings.HasPrefix(key, prefix) {
			delete(s.resolveCache, key)
		}
	}
	for key, call := range s.resolveFlight {
		if strings.HasPrefix(key, prefix) && call != nil {
			call.err = fmt.Errorf("%s storage config changed", typ)
		}
	}
}

func (s *StorageConfigService) CloudResolveUncached(ctx context.Context, typ, fileRef, clientUA string) (*cloud.DirectLink, error) {
	p, err := s.cloudProviderWithUA(ctx, typ, clientUA)
	if err != nil {
		return nil, err
	}
	return p.Resolve(ctx, fileRef)
}

// cloudProviderWithUA builds a provider, overriding the request UA when a
// non-empty clientUA is supplied.
func (s *StorageConfigService) cloudProviderWithUA(ctx context.Context, typ, clientUA string) (cloud.Provider, error) {
	if !cloud.IsCloudType(typ) {
		return nil, fmt.Errorf("not a cloud provider: %q", typ)
	}
	view, err := s.Get(ctx, typ)
	if err != nil {
		return nil, err
	}
	if view == nil {
		return nil, fmt.Errorf("%s storage not configured", typ)
	}
	if !view.Enabled {
		return nil, fmt.Errorf("%s storage disabled", typ)
	}
	cfg := view.Config
	if strings.TrimSpace(clientUA) != "" {
		// Copy so we never mutate the cached view config.
		cp := make(map[string]any, len(cfg)+1)
		for k, v := range cfg {
			cp[k] = v
		}
		cp["ua"] = clientUA
		cfg = cp
	}
	return cloud.New(typ, cfg, s.clientForConfig(cfg))
}
