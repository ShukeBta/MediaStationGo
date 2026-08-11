package service

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var ErrMediaProbeEmpty = errors.New("ffprobe returned no supported media metadata")
var ErrMediaProbeIncomplete = errors.New("ffprobe left required media metadata incomplete")

func mediaTrackMetadataMissing(media *model.Media) bool {
	if media == nil {
		return true
	}
	return media.MediaProbeVersion < mediaProbeMetadataVersion ||
		media.DurationSec <= 0 ||
		media.Width <= 0 ||
		media.Height <= 0 ||
		strings.TrimSpace(media.VideoCodec) == "" ||
		strings.TrimSpace(media.AudioCodec) == ""
}

func persistMediaProbeResult(
	ctx context.Context,
	repo *repository.Container,
	cache *RuntimeCacheService,
	generated *GeneratedArtworkService,
	log *zap.Logger,
	media *model.Media,
	probe *ProbeResult,
) error {
	if repo == nil || repo.DB == nil || media == nil {
		return ErrMediaNotFound
	}
	updates := probeResultUpdates(probe)
	if len(updates) == 0 {
		return ErrMediaProbeEmpty
	}
	durationSec := media.DurationSec
	storedBitRate := media.BitRate
	if probe.DurationSec > 0 {
		durationSec = probe.DurationSec
	}
	if probe.BitRate > 0 {
		storedBitRate = probe.BitRate
	}
	effectiveBitRate := effectiveMediaBitRate(storedBitRate, media.SizeBytes, durationSec)
	if effectiveBitRate > 0 {
		updates["bit_rate"] = effectiveBitRate
	}
	previousDuration := media.DurationSec
	if err := repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", media.ID).Updates(updates).Error; err != nil {
		return err
	}
	applyProbeResultToMediaValue(media, probe)
	if effectiveBitRate > 0 {
		media.BitRate = effectiveBitRate
	}
	if generated != nil && previousDuration != media.DurationSec {
		if _, err := generated.QueueRefreshForMedia(context.WithoutCancel(ctx), media.ID); err != nil && log != nil {
			log.Warn("queue generated artwork refresh after media probe failed", zap.String("media_id", media.ID), zap.Error(err))
		}
	}
	if cache != nil {
		cache.DeletePrefix(context.WithoutCancel(ctx), "media:")
	}
	return nil
}

func applyProbeResultToMediaValue(media *model.Media, probe *ProbeResult) {
	if media == nil || probe == nil {
		return
	}
	if probe.DurationSec > 0 {
		media.DurationSec = probe.DurationSec
	}
	if probe.Width > 0 {
		media.Width = probe.Width
	}
	if probe.Height > 0 {
		media.Height = probe.Height
	}
	if strings.TrimSpace(probe.VideoCodec) != "" {
		media.VideoCodec = probe.VideoCodec
	}
	if strings.TrimSpace(probe.AudioCodec) != "" {
		media.AudioCodec = probe.AudioCodec
	}
	if strings.TrimSpace(probe.Container) != "" {
		media.Container = probe.Container
	}
	if probe.BitRate > 0 {
		media.BitRate = probe.BitRate
	}
	if probe.VideoBitRate > 0 {
		media.VideoBitRate = probe.VideoBitRate
	}
	if probe.FrameRate > 0 {
		media.FrameRate = probe.FrameRate
	}
	if strings.TrimSpace(probe.VideoProfile) != "" {
		media.VideoProfile = probe.VideoProfile
	}
	if strings.TrimSpace(probe.VideoRange) != "" {
		media.VideoRange = probe.VideoRange
	}
	if probe.VideoBitDepth > 0 {
		media.VideoBitDepth = probe.VideoBitDepth
	}
	if probe.AudioBitRate > 0 {
		media.AudioBitRate = probe.AudioBitRate
	}
	if probe.AudioChannels > 0 {
		media.AudioChannels = probe.AudioChannels
	}
	if strings.TrimSpace(probe.AudioChannelLayout) != "" {
		media.AudioChannelLayout = probe.AudioChannelLayout
	}
	if probe.AudioSampleRate > 0 {
		media.AudioSampleRate = probe.AudioSampleRate
	}
	media.MediaProbeVersion = mediaProbeMetadataVersion
}

func probeResultUpdates(probe *ProbeResult) map[string]any {
	updates := map[string]any{}
	if probe == nil {
		return updates
	}
	if probe.DurationSec > 0 {
		updates["duration_sec"] = probe.DurationSec
	}
	if probe.Width > 0 {
		updates["width"] = probe.Width
	}
	if probe.Height > 0 {
		updates["height"] = probe.Height
	}
	if strings.TrimSpace(probe.VideoCodec) != "" {
		updates["video_codec"] = probe.VideoCodec
	}
	if strings.TrimSpace(probe.AudioCodec) != "" {
		updates["audio_codec"] = probe.AudioCodec
	}
	if probe.Container != "" {
		updates["container"] = probe.Container
	}
	if probe.BitRate > 0 {
		updates["bit_rate"] = probe.BitRate
	}
	if probe.VideoBitRate > 0 {
		updates["video_bit_rate"] = probe.VideoBitRate
	}
	if probe.FrameRate > 0 {
		updates["frame_rate"] = probe.FrameRate
	}
	if strings.TrimSpace(probe.VideoProfile) != "" {
		updates["video_profile"] = probe.VideoProfile
	}
	if strings.TrimSpace(probe.VideoRange) != "" {
		updates["video_range"] = probe.VideoRange
	}
	if probe.VideoBitDepth > 0 {
		updates["video_bit_depth"] = probe.VideoBitDepth
	}
	if probe.AudioBitRate > 0 {
		updates["audio_bit_rate"] = probe.AudioBitRate
	}
	if probe.AudioChannels > 0 {
		updates["audio_channels"] = probe.AudioChannels
	}
	if strings.TrimSpace(probe.AudioChannelLayout) != "" {
		updates["audio_channel_layout"] = probe.AudioChannelLayout
	}
	if probe.AudioSampleRate > 0 {
		updates["audio_sample_rate"] = probe.AudioSampleRate
	}
	if len(updates) > 0 {
		updates["media_probe_version"] = mediaProbeMetadataVersion
	}
	return updates
}
