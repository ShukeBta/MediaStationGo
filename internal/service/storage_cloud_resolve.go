package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type cloudResolveCacheEntry struct {
	link         *cloud.DirectLink
	expiresAt    time.Time
	staleUntil   time.Time
	refreshAfter time.Time
	hits         int
	lastHit      time.Time
}

type cloudResolveCall struct {
	done         chan struct{}
	generation   uint64
	circuitProbe bool
	link         *cloud.DirectLink
	err          error
}

type cloudResolveFileGate struct {
	semaphore chan struct{}
	users     int
}

type cloudResolveSigningGate struct {
	semaphore chan struct{}
}

type cloudResolveFailureEntry struct {
	err       error
	expiresAt time.Time
}

type cloudResolveCircuitState struct {
	consecutiveFailures int
	lastFailure         time.Time
	openUntil           time.Time
	probeInFlight       bool
}

type cloudResolveCircuitOpenError struct {
	provider string
	retryAt  time.Time
}

func (e *cloudResolveCircuitOpenError) Error() string {
	return fmt.Sprintf("%s cloud resolve temporarily unavailable until %s", e.provider, e.retryAt.Format(time.RFC3339))
}

const (
	cloudResolveHotHitThreshold      = 3
	cloudResolveBackgroundRefreshMax = 30 * time.Second
	cloudResolveColdMaxDuration      = 8 * time.Second
	cloudResolveRefreshMinInterval   = 30 * time.Second
	cloudResolveCacheMaxEntries      = 2048
	cloudResolveFailureCacheTTL      = 15 * time.Second
	cloudResolveFailureMaxEntries    = 2048
	cloudResolveCircuitFailureWindow = 30 * time.Second
	cloudResolveCircuitCooldown      = 60 * time.Second
	cloudResolveCircuitThreshold     = 2
	cloudResolveSigningConcurrency   = 2
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
	if err, ok := s.cachedResolveFailure(cacheKey); ok {
		return nil, err
	}
	call, owner := s.beginResolve(cacheKey, typ)
	if owner {
		s.runResolve(cacheKey, typ, fileRef, clientUA, call, cloudResolveColdMaxDuration)
	}
	select {
	case <-call.done:
		if call.err != nil {
			return nil, call.err
		}
		return cloneDirectLink(call.link), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *StorageConfigService) cachedResolveFailure(key string) (error, bool) {
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	entry, ok := s.resolveErrors[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.resolveErrors, key)
		return nil, false
	}
	return entry.err, true
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
	if !ok {
		return nil, false, false
	}
	if strings.TrimSpace(typ) == cloud.TypeOpenList {
		// OpenList's 115 CDN URLs are finite and may be UA-bound. Reuse only a
		// still-fresh link; the next real request resolves again through singleflight.
		if entry.expiresAt.IsZero() || !now.Before(entry.expiresAt) {
			delete(s.resolveCache, key)
			return nil, false, false
		}
		entry.lastHit = now
		s.resolveCache[key] = entry
		return cloneDirectLink(entry.link), true, false
	}
	if entry.staleUntil.IsZero() {
		entry.staleUntil = entry.expiresAt
	}
	if now.After(entry.staleUntil) {
		if ok {
			delete(s.resolveCache, key)
		}
		return nil, false, false
	}
	entry.hits++
	entry.lastHit = now
	fresh := !now.After(entry.expiresAt)
	refreshWindow := cloudResolveHotRefreshWindow(cloudResolveCacheTTL(typ))
	shouldRefresh := !fresh || (entry.hits >= cloudResolveHotHitThreshold &&
		refreshWindow > 0 &&
		now.Add(refreshWindow).After(entry.expiresAt))
	if shouldRefresh && !entry.refreshAfter.IsZero() && now.Before(entry.refreshAfter) {
		shouldRefresh = false
	}
	if shouldRefresh {
		entry.refreshAfter = now.Add(cloudResolveRefreshMinInterval)
	}
	s.resolveCache[key] = entry
	return cloneDirectLink(entry.link), true, shouldRefresh
}

func (s *StorageConfigService) beginResolve(key, typ string) (*cloudResolveCall, bool) {
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	if s.resolveFlight == nil {
		s.resolveFlight = make(map[string]*cloudResolveCall)
	}
	if s.resolveGen == nil {
		s.resolveGen = make(map[string]uint64)
	}
	if call := s.resolveFlight[key]; call != nil {
		return call, false
	}
	call := &cloudResolveCall{
		done:       make(chan struct{}),
		generation: s.resolveGen[strings.TrimSpace(typ)],
	}
	s.resolveFlight[key] = call
	return call, true
}

func (s *StorageConfigService) runResolve(key, typ, fileRef, clientUA string, call *cloudResolveCall, maxDuration time.Duration) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), maxDuration)
		defer cancel()
		waitStarted := time.Now()
		release, err := s.acquireResolveFileGate(ctx, typ, fileRef)
		if err != nil {
			s.completeResolve(key, typ, call, nil, err)
			return
		}
		releaseSigning, err := s.acquireResolveSigningGate(ctx, typ)
		if err != nil {
			release()
			s.completeResolve(key, typ, call, nil, err)
			return
		}
		defer releaseSigning()
		defer release()
		if waited := time.Since(waitStarted); waited >= time.Millisecond && s.log != nil {
			s.log.Info("cloud resolve signing queue wait",
				zap.String("provider", strings.TrimSpace(typ)),
				zap.Int64("queue_ms", waited.Milliseconds()))
		}
		if err := s.prepareResolveCircuit(typ, call); err != nil {
			s.completeResolve(key, typ, call, nil, err)
			return
		}
		p, err := s.cloudProviderWithUA(ctx, typ, clientUA)
		if err != nil {
			s.completeResolve(key, typ, call, nil, err)
			return
		}
		link, err := p.Resolve(ctx, fileRef)
		s.completeResolve(key, typ, call, link, err)
	}()
}

func (s *StorageConfigService) acquireResolveFileGate(ctx context.Context, typ, fileRef string) (func(), error) {
	if strings.TrimSpace(typ) != cloud.TypeOpenList {
		return func() {}, nil
	}
	gateKey := strings.TrimSpace(typ) + "\x00" + strings.TrimSpace(fileRef)
	s.resolveMu.Lock()
	if s.resolveFileGate == nil {
		s.resolveFileGate = make(map[string]*cloudResolveFileGate)
	}
	gate := s.resolveFileGate[gateKey]
	if gate == nil {
		gate = &cloudResolveFileGate{semaphore: make(chan struct{}, 1)}
		s.resolveFileGate[gateKey] = gate
	}
	gate.users++
	s.resolveMu.Unlock()

	select {
	case gate.semaphore <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-gate.semaphore
			s.releaseResolveFileGateUser(gateKey, gate)
			return nil, err
		}
		return func() {
			<-gate.semaphore
			s.releaseResolveFileGateUser(gateKey, gate)
		}, nil
	case <-ctx.Done():
		s.releaseResolveFileGateUser(gateKey, gate)
		return nil, ctx.Err()
	}
}

func (s *StorageConfigService) releaseResolveFileGateUser(key string, gate *cloudResolveFileGate) {
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	gate.users--
	if gate.users == 0 && s.resolveFileGate[key] == gate {
		delete(s.resolveFileGate, key)
	}
}

func (s *StorageConfigService) acquireResolveSigningGate(ctx context.Context, typ string) (func(), error) {
	if strings.TrimSpace(typ) != cloud.TypeOpenList {
		return func() {}, nil
	}
	s.resolveMu.Lock()
	if s.resolveSigningGate == nil {
		s.resolveSigningGate = &cloudResolveSigningGate{semaphore: make(chan struct{}, cloudResolveSigningConcurrency)}
	}
	gate := s.resolveSigningGate
	s.resolveMu.Unlock()

	select {
	case gate.semaphore <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return func() { <-gate.semaphore }, nil
}

func (s *StorageConfigService) completeResolve(key, typ string, call *cloudResolveCall, link *cloud.DirectLink, resolveErr error) {
	typ = strings.TrimSpace(typ)
	ttl, staleTTL := cloudResolveCacheDurations(typ, link)
	if resolveErr != nil && s.log != nil {
		s.log.Debug("resolve cloud direct link failed", zap.String("provider", typ), zap.Error(resolveErr))
	}

	s.resolveMu.Lock()
	if s.resolveGen == nil {
		s.resolveGen = make(map[string]uint64)
	}
	if s.resolveGen[typ] != call.generation {
		call.err = fmt.Errorf("%s storage config changed", typ)
	} else if resolveErr != nil {
		call.err = resolveErr
	} else if link == nil || strings.TrimSpace(link.URL) == "" {
		call.err = errors.New("cloud resolve returned empty link")
	} else {
		call.link = cloneDirectLink(link)
		s.storeResolvedLinkLocked(key, link, ttl, staleTTL)
	}
	if s.resolveGen[typ] == call.generation {
		s.recordResolveCircuitOutcomeLocked(typ, call.err, call.circuitProbe)
		if call.err == nil {
			delete(s.resolveErrors, key)
		} else if failureTTL := cloudResolveFailureTTLForError(call.err); failureTTL > 0 {
			s.storeResolveFailureLocked(key, call.err, failureTTL)
		}
	}
	if current := s.resolveFlight[key]; current == call {
		delete(s.resolveFlight, key)
	}
	close(call.done)
	s.resolveMu.Unlock()
}

func (s *StorageConfigService) prepareResolveCircuit(typ string, call *cloudResolveCall) error {
	if !cloudResolveCircuitEnabled(typ) {
		return nil
	}
	typ = strings.TrimSpace(typ)
	s.resolveMu.Lock()
	defer s.resolveMu.Unlock()
	if s.resolveCircuit == nil {
		s.resolveCircuit = make(map[string]cloudResolveCircuitState)
	}
	state := s.resolveCircuit[typ]
	if state.openUntil.IsZero() {
		return nil
	}
	now := time.Now()
	if now.Before(state.openUntil) {
		return &cloudResolveCircuitOpenError{provider: typ, retryAt: state.openUntil}
	}
	if state.probeInFlight {
		return &cloudResolveCircuitOpenError{provider: typ, retryAt: now.Add(cloudResolveFailureCacheTTL)}
	}
	state.probeInFlight = true
	s.resolveCircuit[typ] = state
	call.circuitProbe = true
	return nil
}

func (s *StorageConfigService) recordResolveCircuitOutcomeLocked(typ string, resolveErr error, probe bool) {
	if !cloudResolveCircuitEnabled(typ) {
		return
	}
	if s.resolveCircuit == nil {
		s.resolveCircuit = make(map[string]cloudResolveCircuitState)
	}
	typ = strings.TrimSpace(typ)
	if resolveErr == nil {
		delete(s.resolveCircuit, typ)
		return
	}
	var circuitErr *cloudResolveCircuitOpenError
	if errors.As(resolveErr, &circuitErr) {
		return
	}
	now := time.Now()
	state := s.resolveCircuit[typ]
	if cloudResolveRateLimited(resolveErr) {
		state.consecutiveFailures = 0
		state.lastFailure = time.Time{}
		state.openUntil = now.Add(cloudResolveCircuitCooldown)
		state.probeInFlight = false
		s.resolveCircuit[typ] = state
		s.logResolveCircuitOpened(typ, "provider_rate_limit", state.openUntil)
		return
	}
	if !cloudResolveTransportFailure(resolveErr) {
		if probe || !state.openUntil.After(now) {
			delete(s.resolveCircuit, typ)
		}
		return
	}
	if state.openUntil.After(now) && !probe {
		return
	}
	if probe {
		state.consecutiveFailures = 0
		state.lastFailure = now
		state.openUntil = now.Add(cloudResolveCircuitCooldown)
		state.probeInFlight = false
		s.resolveCircuit[typ] = state
		s.logResolveCircuitOpened(typ, "half_open_probe_failed", state.openUntil)
		return
	}
	if state.lastFailure.IsZero() || now.Sub(state.lastFailure) > cloudResolveCircuitFailureWindow {
		state.consecutiveFailures = 1
	} else {
		state.consecutiveFailures++
	}
	state.lastFailure = now
	if state.consecutiveFailures >= cloudResolveCircuitThreshold {
		state.consecutiveFailures = 0
		state.openUntil = now.Add(cloudResolveCircuitCooldown)
		state.probeInFlight = false
		s.logResolveCircuitOpened(typ, "consecutive_transport_failures", state.openUntil)
	}
	s.resolveCircuit[typ] = state
}

func (s *StorageConfigService) logResolveCircuitOpened(typ, reason string, retryAt time.Time) {
	if s.log == nil {
		return
	}
	s.log.Warn("cloud resolve circuit opened",
		zap.String("provider", typ),
		zap.String("reason", reason),
		zap.Time("retry_at", retryAt))
}

func (s *StorageConfigService) storeResolveFailureLocked(key string, resolveErr error, ttl time.Duration) {
	if resolveErr == nil || ttl <= 0 {
		return
	}
	if s.resolveErrors == nil {
		s.resolveErrors = make(map[string]cloudResolveFailureEntry)
	}
	now := time.Now()
	for existingKey, entry := range s.resolveErrors {
		if now.After(entry.expiresAt) {
			delete(s.resolveErrors, existingKey)
		}
	}
	if _, exists := s.resolveErrors[key]; !exists && len(s.resolveErrors) >= cloudResolveFailureMaxEntries {
		oldestKey := ""
		var oldestExpiry time.Time
		for existingKey, entry := range s.resolveErrors {
			if oldestKey == "" || entry.expiresAt.Before(oldestExpiry) {
				oldestKey = existingKey
				oldestExpiry = entry.expiresAt
			}
		}
		if oldestKey != "" {
			delete(s.resolveErrors, oldestKey)
		}
	}
	s.resolveErrors[key] = cloudResolveFailureEntry{err: resolveErr, expiresAt: now.Add(ttl)}
}

func cloudResolveFailureTTLForError(err error) time.Duration {
	if cloudResolveRateLimited(err) {
		return cloudResolveCircuitCooldown
	}
	if cloudResolveTransportFailure(err) {
		return cloudResolveFailureCacheTTL
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{"failed get link", "bad gateway", "returned http 5", "code=500", "status 502"} {
		if strings.Contains(msg, needle) {
			return cloudResolveFailureCacheTTL
		}
	}
	return 0
}

func cloudResolveCircuitEnabled(typ string) bool {
	return strings.TrimSpace(typ) == cloud.TypeOpenList
}

func cloudResolveRateLimited(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "40140117")
}

func cloudResolveTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"tls handshake timeout",
		"i/o timeout",
		"client.timeout exceeded",
		"context deadline exceeded",
		"connection reset",
		"connection refused",
		"no such host",
		"server misbehaving",
		"temporary failure",
		"unexpected eof",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func (s *StorageConfigService) refreshResolveInBackground(key, typ, fileRef, clientUA string) {
	if s == nil {
		return
	}
	call, owner := s.beginResolve(key, typ)
	if !owner {
		return
	}
	s.runResolve(key, typ, fileRef, clientUA, call, cloudResolveBackgroundRefreshMax)
}

func (s *StorageConfigService) storeResolvedLinkLocked(key string, link *cloud.DirectLink, ttl, staleTTL time.Duration) {
	if link == nil || strings.TrimSpace(link.URL) == "" || ttl <= 0 {
		return
	}
	if s.resolveCache == nil {
		s.resolveCache = make(map[string]cloudResolveCacheEntry)
	}
	now := time.Now()
	s.pruneResolveCacheLocked(now, key)
	hits := 0
	if existing, ok := s.resolveCache[key]; ok {
		hits = existing.hits
	}
	expiresAt := now.Add(ttl)
	staleUntil := now.Add(staleTTL)
	if staleUntil.Before(expiresAt) {
		staleUntil = expiresAt
	}
	s.resolveCache[key] = cloudResolveCacheEntry{link: cloneDirectLink(link), expiresAt: expiresAt, staleUntil: staleUntil, hits: hits, lastHit: now}
}

func (s *StorageConfigService) pruneResolveCacheLocked(now time.Time, incomingKey string) {
	if _, exists := s.resolveCache[incomingKey]; exists || len(s.resolveCache) < cloudResolveCacheMaxEntries {
		return
	}
	for key, entry := range s.resolveCache {
		if !entry.staleUntil.IsZero() && now.After(entry.staleUntil) {
			delete(s.resolveCache, key)
		}
	}
	for len(s.resolveCache) >= cloudResolveCacheMaxEntries {
		oldestKey := ""
		var oldest time.Time
		for key, entry := range s.resolveCache {
			seen := entry.lastHit
			if seen.IsZero() {
				seen = entry.expiresAt
			}
			if oldestKey == "" || seen.Before(oldest) {
				oldestKey = key
				oldest = seen
			}
		}
		if oldestKey == "" {
			return
		}
		delete(s.resolveCache, oldestKey)
	}
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

func cloudResolveCacheDurations(typ string, link *cloud.DirectLink) (time.Duration, time.Duration) {
	ttl := cloudResolveCacheTTL(typ)
	staleTTL := ttl
	if link == nil {
		return ttl, staleTTL
	}
	expiry, ok := cloudResolve115DirectLinkExpiry(link.URL)
	if !ok {
		return ttl, staleTTL
	}
	untilExpiry := time.Until(expiry)
	if strings.TrimSpace(typ) == cloud.TypeOpenList {
		usable := untilExpiry - time.Minute
		if usable <= 0 {
			return 0, 0
		}
		ttl = minDuration(usable, 30*time.Minute)
		return ttl, ttl
	}
	if untilExpiry <= time.Minute {
		return ttl, staleTTL
	}
	usable := untilExpiry - time.Minute
	if usable > staleTTL {
		staleTTL = minDuration(usable, 50*time.Minute)
	}
	if staleTTL > ttl {
		ttl = minDuration(staleTTL, 30*time.Minute)
	}
	return ttl, staleTTL
}

func cloudResolve115DirectLinkExpiry(raw string) (time.Time, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || !strings.Contains(strings.ToLower(u.Host), "115cdn.net") {
		return time.Time{}, false
	}
	rawT := strings.TrimSpace(u.Query().Get("t"))
	if rawT == "" {
		return time.Time{}, false
	}
	n, err := strconv.ParseInt(rawT, 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	if len(rawT) >= 13 {
		n /= 1000
	}
	expiry := time.Unix(n, 0)
	if !expiry.After(time.Now()) {
		return time.Time{}, false
	}
	return expiry, true
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
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
	if s.resolveGen == nil {
		s.resolveGen = make(map[string]uint64)
	}
	s.resolveGen[typ]++
	for key := range s.resolveCache {
		if strings.HasPrefix(key, prefix) {
			delete(s.resolveCache, key)
		}
	}
	for key := range s.resolveErrors {
		if strings.HasPrefix(key, prefix) {
			delete(s.resolveErrors, key)
		}
	}
	for key := range s.resolveFlight {
		if strings.HasPrefix(key, prefix) {
			delete(s.resolveFlight, key)
		}
	}
	delete(s.resolveCircuit, typ)
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
