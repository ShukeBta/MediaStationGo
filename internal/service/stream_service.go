package service

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
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
	cfg        *config.Config
	log        *zap.Logger
	repo       *repository.Container
	transcoder *TranscoderService
}

// NewStreamService is the constructor.
func NewStreamService(cfg *config.Config, log *zap.Logger, repo *repository.Container, transcoder *TranscoderService) *StreamService {
	return &StreamService{
		cfg:        cfg,
		log:        log,
		repo:       repo,
		transcoder: transcoder,
	}
}

// ErrMediaNotFound is returned when the media row or its file is missing.
var ErrMediaNotFound = errors.New("media not found")

// ErrCloudPlaybackUnavailable 表示媒体行存在但属于云盘媒体、且当前无法
// 构造可用的播放重定向（通常是 STRMURL 缺失，需要重新扫描媒体库）。
// 调用方应把它与「媒体不存在」区分开，避免把配置类故障当成 404 返回给播放器。
var ErrCloudPlaybackUnavailable = errors.New("cloud media playback unavailable: media missing play url; re-scan the library")

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
func (s *StreamService) Probe(ctx context.Context, mediaID string, probe *FFprobeService) error {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return ErrMediaNotFound
	}
	res, err := probe.Probe(ctx, m.Path)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"duration_sec": res.DurationSec,
		"width":        res.Width,
		"height":       res.Height,
		"video_codec":  res.VideoCodec,
		"audio_codec":  res.AudioCodec,
	}
	if res.Container != "" {
		updates["container"] = res.Container
	}
	return s.repo.DB.Model(m).Updates(updates).Error
}
