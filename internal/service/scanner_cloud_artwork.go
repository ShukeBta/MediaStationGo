package service

import (
	"context"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

type cloudImagePrefetchTask struct {
	typ       string
	ref       string
	stableKey string
}

func (s *ScannerService) cloudImagePrefetchWorker() {
	for task := range s.cloudImagePrefetchQueue {
		s.prefetchCloudImage(task)
	}
}

func (s *ScannerService) queueCloudArtworkPrefetch(raw string) {
	if s == nil || s.storage == nil || s.imageProxy == nil {
		return
	}
	typ, ref, ok := ParseCloudArtworkURL(raw)
	if !ok {
		return
	}
	stableKey := typ + ":" + ref
	if s.imageProxy.CloudImageCached(stableKey) {
		return
	}
	s.cloudImagePrefetchMu.Lock()
	if _, ok := s.cloudImagePrefetching[stableKey]; ok {
		s.cloudImagePrefetchMu.Unlock()
		return
	}
	s.cloudImagePrefetching[stableKey] = struct{}{}
	s.cloudImagePrefetchMu.Unlock()

	task := cloudImagePrefetchTask{typ: typ, ref: ref, stableKey: stableKey}
	select {
	case s.cloudImagePrefetchQueue <- task:
	default:
		s.cloudImagePrefetchMu.Lock()
		delete(s.cloudImagePrefetching, stableKey)
		s.cloudImagePrefetchMu.Unlock()
		if s.log != nil {
			s.log.Debug("cloud artwork prefetch queue full", zap.String("provider", typ), zap.String("ref", ref))
		}
	}
}

func (s *ScannerService) prefetchCloudImage(task cloudImagePrefetchTask) {
	defer func() {
		s.cloudImagePrefetchMu.Lock()
		delete(s.cloudImagePrefetching, task.stableKey)
		s.cloudImagePrefetchMu.Unlock()
	}()
	if s == nil || s.storage == nil || s.imageProxy == nil || s.imageProxy.CloudImageCached(task.stableKey) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	link, err := s.storage.CloudResolve(ctx, task.typ, task.ref, "")
	if err != nil {
		if s.log != nil {
			s.log.Debug("resolve cloud artwork for prefetch failed", zap.String("provider", task.typ), zap.String("ref", task.ref), zap.Error(err))
		}
		return
	}
	if err := s.imageProxy.PrefetchCloudResolved(ctx, task.stableKey, link); err != nil && s.log != nil {
		s.log.Debug("prefetch cloud artwork failed", zap.String("provider", task.typ), zap.String("ref", task.ref), zap.Error(err))
	}
}

func (s *ScannerService) cacheCloudArtworkNow(ctx context.Context, raw string) {
	if s == nil || s.storage == nil || s.imageProxy == nil {
		return
	}
	typ, ref, ok := ParseCloudArtworkURL(raw)
	if !ok {
		return
	}
	stableKey := typ + ":" + ref
	if s.imageProxy.CloudImageCached(stableKey) {
		return
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	link, err := s.storage.CloudResolve(cacheCtx, typ, ref, "")
	if err != nil {
		if s.log != nil {
			s.log.Debug("resolve cloud artwork for priority cache failed", zap.String("provider", typ), zap.String("ref", ref), zap.Error(err))
		}
		s.queueCloudArtworkPrefetch(raw)
		return
	}
	if err := s.imageProxy.PrefetchCloudResolved(cacheCtx, stableKey, link); err != nil {
		if s.log != nil {
			s.log.Debug("priority cache cloud artwork failed", zap.String("provider", typ), zap.String("ref", ref), zap.Error(err))
		}
		s.queueCloudArtworkPrefetch(raw)
	}
}

func (s *ScannerService) cacheCloudMetadataArtworkNow(ctx context.Context, meta *LocalMetadata) {
	if meta == nil {
		return
	}
	s.cacheCloudArtworkNow(ctx, meta.PosterURL)
	s.cacheCloudArtworkNow(ctx, meta.BackdropURL)
}

func ParseCloudArtworkURL(raw string) (string, string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", "", false
	}
	path := strings.Trim(u.Path, "/")
	typ := ""
	for _, prefix := range []string{"api/img/cloud/", "api/cloud/play/"} {
		if strings.HasPrefix(strings.ToLower(path), prefix) {
			typ = strings.TrimSpace(path[len(prefix):])
			break
		}
	}
	if typ == "" {
		return "", "", false
	}
	ref := strings.TrimSpace(u.Query().Get("ref"))
	if typ == "" || ref == "" || !isCloudArtworkRef(ref) {
		return "", "", false
	}
	return typ, ref, true
}

func CloudArtworkURL(typ, ref string) string {
	typ = strings.Trim(strings.ReplaceAll(strings.TrimSpace(typ), "\\", "/"), "/")
	ref = strings.TrimSpace(ref)
	if typ == "" || ref == "" {
		return ""
	}
	return "/api/img/cloud/" + url.PathEscape(typ) + "?ref=" + url.QueryEscape(ref)
}

func isCloudArtworkRef(ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	for _, suffix := range []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tbn"} {
		if strings.HasSuffix(ref, suffix) {
			return true
		}
	}
	return false
}
