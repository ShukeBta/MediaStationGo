package service

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

const (
	STRMEnabledSettingKey                  = "strm.enabled"
	CloudPlaybackModeSettingKey            = "cloud.playback_mode"
	CloudPlaybackSTRMEnabledSettingKey     = "cloud.playback_strm_enabled"
	CloudPlaybackRedirectEnabledSettingKey = "cloud.playback_redirect_proxy_enabled"

	CloudPlaybackModeSTRM          = "strm"
	CloudPlaybackModeRedirectProxy = "redirect_proxy"
)

type CloudPlaybackOptions struct {
	STRMEnabled          bool
	RedirectProxyEnabled bool
	PreferredMode        string
}

// StreamService serves media files with proper Range support so browsers can
// seek into the stream.
type StreamService struct {
	cfg              *config.Config
	log              *zap.Logger
	repo             *repository.Container
	transcoder       *TranscoderService
	storage          cloudPlaybackResolver
	probe            cloudPlaybackProber
	playback         *PlaybackService
	cache            *RuntimeCacheService
	generatedArtwork *GeneratedArtworkService

	cloudTrackProbeOnce     sync.Once
	cloudTrackProbeQueue    chan playbackCloudProbeTask
	cloudTrackProbeMu       sync.Mutex
	cloudTrackProbePending  map[string]struct{}
	cloudTrackProbeBackoff  map[string]time.Time
	cloudTrackProbeWarnMu   sync.Mutex
	cloudTrackProbeLastWarn time.Time
}

type mediaTrackProber interface {
	Probe(ctx context.Context, path string) (*ProbeResult, error)
	ProbeHTTP(ctx context.Context, rawURL string, headers map[string]string) (*ProbeResult, error)
}

// NewStreamService is the constructor.
func NewStreamService(cfg *config.Config, log *zap.Logger, repo *repository.Container, transcoder *TranscoderService) *StreamService {
	workers := playbackCloudProbeWorkerCount(cfg)
	return &StreamService{
		cfg:                    cfg,
		log:                    log,
		repo:                   repo,
		transcoder:             transcoder,
		cloudTrackProbeQueue:   make(chan playbackCloudProbeTask, workers*4),
		cloudTrackProbePending: make(map[string]struct{}),
		cloudTrackProbeBackoff: make(map[string]time.Time),
	}
}

// prewarmCloudPlay 在响应 302 后，用请求自身的 UA 后台预热云盘直链解析，
// 使客户端 follow /api/cloud/play 时命中缓存，避免首次播放冷解析的等待。
func (s *StreamService) prewarmCloudPlay(r *http.Request, raw string) {
	if s == nil || s.storage == nil || r == nil {
		return
	}
	typ, ref, ok := parseCloudMediaPlaybackURL(raw)
	if !ok {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), cloudResolveColdMaxDuration)
		defer cancel()
		_, _ = s.storage.CloudResolve(ctx, typ, ref, r.UserAgent())
	}()
}

func (s *StreamService) SetCloudProbe(storage cloudPlaybackResolver) {
	if s != nil {
		s.storage = storage
	}
}

// SetCloudTrackProbe wires the HTTP media prober used by the playback-time
// metadata queue. Workers are started once and the queue itself stays bounded,
// so successful playback never waits for ffprobe or creates unbounded goroutines.
func (s *StreamService) SetCloudTrackProbe(probe cloudPlaybackProber) {
	if s == nil {
		return
	}
	s.cloudTrackProbeMu.Lock()
	s.probe = probe
	if s.cloudTrackProbeQueue == nil {
		workers := playbackCloudProbeWorkerCount(s.cfg)
		s.cloudTrackProbeQueue = make(chan playbackCloudProbeTask, workers*4)
	}
	if s.cloudTrackProbePending == nil {
		s.cloudTrackProbePending = make(map[string]struct{})
	}
	if s.cloudTrackProbeBackoff == nil {
		s.cloudTrackProbeBackoff = make(map[string]time.Time)
	}
	s.cloudTrackProbeMu.Unlock()
	if probe == nil {
		return
	}
	s.cloudTrackProbeOnce.Do(func() {
		workers := playbackCloudProbeWorkerCount(s.cfg)
		for i := 0; i < workers; i++ {
			go s.playbackCloudProbeWorker()
		}
	})
}

func (s *StreamService) SetPlaybackService(playback *PlaybackService) {
	if s != nil {
		s.playback = playback
	}
}

func (s *StreamService) SetRuntimeCache(cache *RuntimeCacheService) {
	if s != nil {
		s.cache = cache
	}
}

func (s *StreamService) SetGeneratedArtworkService(generated *GeneratedArtworkService) {
	if s != nil {
		s.generatedArtwork = generated
	}
}

// ErrMediaNotFound is returned when the media row or its file is missing.
var ErrMediaNotFound = errors.New("media not found")

// ErrCloudPlaybackUnavailable 表示媒体行存在但属于云盘媒体、且当前无法
// 构造可用的播放重定向（通常是 STRMURL 缺失，需要重新扫描媒体库）。
// 调用方应把它与「媒体不存在」区分开，避免把配置类故障当成 404 返回给播放器。
var ErrCloudPlaybackUnavailable = errors.New("cloud media playback unavailable: media missing play url; re-scan the library")

var ErrCloudPlaybackResolveFailed = errors.New("cloud media playback resolve failed")

var ErrCloudPlaybackDisabled = errors.New("cloud media playback disabled by admin settings")

// directPlayOnly reports whether the admin enabled「客户端直连解码」mode,
// in which the host never transcodes (HLS is refused) and all playback is
// handled by the client (direct play / 302 redirect).
func (s *StreamService) directPlayOnly(ctx context.Context) bool {
	if s.repo == nil || s.repo.Setting == nil {
		return false
	}
	v, err := s.repo.Setting.Get(ctx, PlaybackDirectOnlySettingKey)
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

// Probe re-runs ffprobe against an existing media row and refreshes the
// extracted metadata. Used by the admin UI's "rescan" button.
func (s *StreamService) Probe(ctx context.Context, mediaID string, probe mediaTrackProber) error {
	res, media, err := s.Inspect(ctx, mediaID, probe)
	if err != nil {
		return err
	}
	return persistMediaProbeResult(ctx, s.repo, s.cache, s.generatedArtwork, s.log, media, res)
}

// Inspect returns current stream metadata without persisting it. It reuses the
// same local/cloud source resolution as the normal probe path.
func (s *StreamService) Inspect(ctx context.Context, mediaID string, probe mediaTrackProber) (*ProbeResult, *model.Media, error) {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return nil, nil, ErrMediaNotFound
	}
	if probe == nil {
		return nil, nil, errors.New("ffprobe service unavailable")
	}
	res, err := s.probeMediaSource(ctx, m, probe)
	if err != nil {
		return nil, nil, err
	}
	return res, m, nil
}

func (s *StreamService) probeMediaSource(ctx context.Context, media *model.Media, probe mediaTrackProber) (*ProbeResult, error) {
	if media == nil {
		return nil, ErrMediaNotFound
	}
	if typ, ref, ok := parseCloudMediaPlaybackURL(media.STRMURL); ok {
		if s.storage == nil {
			return nil, errors.New("cloud media probe unavailable: storage service not configured")
		}
		return probeCloudFileMetadataWith(ctx, s.storage, probe, typ, ref)
	}
	if rawURL := probeHTTPMediaURL(media); rawURL != "" {
		return probe.ProbeHTTP(ctx, rawURL, cloudMediaInternalHeaders(nil))
	}
	path := strings.TrimSpace(media.Path)
	if path == "" {
		return nil, errors.New("media probe unavailable: empty media path")
	}
	if strings.HasPrefix(strings.ToLower(path), "cloud://") {
		return nil, errors.New("cloud media probe unavailable: missing resolvable playback reference; re-scan the library")
	}
	return probe.Probe(ctx, path)
}

func probeHTTPMediaURL(media *model.Media) string {
	if media == nil {
		return ""
	}
	for _, candidate := range []string{media.STRMURL, media.Path} {
		candidate = strings.TrimSpace(candidate)
		parsed, err := url.Parse(candidate)
		if err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
			return candidate
		}
	}
	return ""
}
