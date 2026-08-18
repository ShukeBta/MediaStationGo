package service

import "github.com/ShukeBta/MediaStationGo/internal/model"

func applyLocalMetadata(m *model.Media, local *LocalMetadata) {
	applyLocalIdentityMetadata(m, local)
	applyLocalArtworkMetadata(m, local)
	applyLocalExternalIDMetadata(m, local)
	applyLocalEpisodeMetadata(m, local)
	applyLocalTaxonomyMetadata(m, local)
	applyLocalTechnicalMetadata(m, local.Technical)
	if local.NSFW {
		m.NSFW = true
	}
	if localMetadataMarksMatched(local) {
		m.ScrapeStatus = "matched"
	}
}

func applyLocalIdentityMetadata(m *model.Media, local *LocalMetadata) {
	if local.Title != "" {
		m.Title = local.Title
	}
	if local.OriginalName != "" {
		m.OriginalName = local.OriginalName
	}
	if local.AdultCode != "" {
		m.OriginalName = local.AdultCode
	}
	if local.Year > 0 {
		m.Year = local.Year
	}
	if local.ReleaseDate != "" {
		m.ReleaseDate = local.ReleaseDate
	}
	if local.Overview != "" {
		m.Overview = local.Overview
	}
	if local.Rating > 0 {
		m.Rating = local.Rating
	}
}

func applyLocalArtworkMetadata(m *model.Media, local *LocalMetadata) {
	if local.PosterURL != "" {
		m.PosterURL = local.PosterURL
	}
	if local.BackdropURL != "" {
		m.BackdropURL = local.BackdropURL
	}
}

func applyLocalExternalIDMetadata(m *model.Media, local *LocalMetadata) {
	if local.TMDbID > 0 {
		m.TMDbID = local.TMDbID
	}
	if local.BangumiID > 0 {
		m.BangumiID = local.BangumiID
	}
	if local.DoubanID != "" {
		m.DoubanID = local.DoubanID
	}
	if local.TheTVDBID != "" {
		m.TheTVDBID = local.TheTVDBID
	}
}

func applyLocalEpisodeMetadata(m *model.Media, local *LocalMetadata) {
	if local.EpisodeTitle != "" {
		m.EpisodeTitle = local.EpisodeTitle
	}
	if local.SeasonNum > 0 || local.EpisodeNum > 0 {
		m.SeasonNum = local.SeasonNum
	}
	if local.EpisodeNum > 0 {
		m.EpisodeNum = local.EpisodeNum
	}
}

func applyLocalTaxonomyMetadata(m *model.Media, local *LocalMetadata) {
	if local.Genres != "" {
		m.Genres = local.Genres
	}
	if local.Actors != "" {
		m.Actors = local.Actors
	}
	if local.Countries != "" {
		m.Countries = local.Countries
	}
	if local.Languages != "" {
		m.Languages = local.Languages
	}
}

func applyLocalTechnicalMetadata(m *model.Media, technical LocalTechnicalMetadata) {
	if technical.DurationSec > 0 {
		m.DurationSec = technical.DurationSec
	}
	if technical.Width > 0 {
		m.Width = technical.Width
	}
	if technical.Height > 0 {
		m.Height = technical.Height
	}
	if technical.VideoCodec != "" {
		m.VideoCodec = technical.VideoCodec
	}
	if technical.AudioCodec != "" {
		m.AudioCodec = technical.AudioCodec
	}
	if technical.VideoBitRate > 0 {
		m.VideoBitRate = technical.VideoBitRate
	}
	if technical.FrameRate > 0 {
		m.FrameRate = technical.FrameRate
	}
	if technical.VideoProfile != "" {
		m.VideoProfile = technical.VideoProfile
	}
	if technical.VideoRange != "" {
		m.VideoRange = technical.VideoRange
	}
	if technical.VideoBitDepth > 0 {
		m.VideoBitDepth = technical.VideoBitDepth
	}
	if technical.AudioBitRate > 0 {
		m.AudioBitRate = technical.AudioBitRate
	}
	if technical.AudioChannels > 0 {
		m.AudioChannels = technical.AudioChannels
	}
	if technical.AudioChannelLayout != "" {
		m.AudioChannelLayout = technical.AudioChannelLayout
	}
	if technical.AudioSampleRate > 0 {
		m.AudioSampleRate = technical.AudioSampleRate
	}
}

func fillMissingLocalTechnicalMetadata(m *model.Media, technical LocalTechnicalMetadata) {
	if m.DurationSec == 0 && technical.DurationSec > 0 {
		m.DurationSec = technical.DurationSec
	}
	if m.Width == 0 && technical.Width > 0 {
		m.Width = technical.Width
	}
	if m.Height == 0 && technical.Height > 0 {
		m.Height = technical.Height
	}
	if m.VideoCodec == "" && technical.VideoCodec != "" {
		m.VideoCodec = technical.VideoCodec
	}
	if m.AudioCodec == "" && technical.AudioCodec != "" {
		m.AudioCodec = technical.AudioCodec
	}
	if m.VideoBitRate == 0 && technical.VideoBitRate > 0 {
		m.VideoBitRate = technical.VideoBitRate
	}
	if m.FrameRate == 0 && technical.FrameRate > 0 {
		m.FrameRate = technical.FrameRate
	}
	if m.VideoProfile == "" && technical.VideoProfile != "" {
		m.VideoProfile = technical.VideoProfile
	}
	if m.VideoRange == "" && technical.VideoRange != "" {
		m.VideoRange = technical.VideoRange
	}
	if m.VideoBitDepth == 0 && technical.VideoBitDepth > 0 {
		m.VideoBitDepth = technical.VideoBitDepth
	}
	if m.AudioBitRate == 0 && technical.AudioBitRate > 0 {
		m.AudioBitRate = technical.AudioBitRate
	}
	if m.AudioChannels == 0 && technical.AudioChannels > 0 {
		m.AudioChannels = technical.AudioChannels
	}
	if m.AudioChannelLayout == "" && technical.AudioChannelLayout != "" {
		m.AudioChannelLayout = technical.AudioChannelLayout
	}
	if m.AudioSampleRate == 0 && technical.AudioSampleRate > 0 {
		m.AudioSampleRate = technical.AudioSampleRate
	}
}

func localMetadataMarksMatched(local *LocalMetadata) bool {
	return local != nil && (local.HasNFO || (!local.PathHint && localHasDescriptiveMetadata(local)))
}

func localHasDescriptiveMetadata(local *LocalMetadata) bool {
	if local == nil {
		return false
	}
	return local.Title != "" ||
		local.OriginalName != "" ||
		local.EpisodeTitle != "" ||
		local.AdultCode != "" ||
		local.Year > 0 ||
		local.ReleaseDate != "" ||
		local.Overview != "" ||
		local.Rating > 0 ||
		local.TMDbID > 0 ||
		local.BangumiID > 0 ||
		local.DoubanID != "" ||
		local.TheTVDBID != "" ||
		local.Genres != "" ||
		local.Actors != "" ||
		local.Countries != "" ||
		local.Languages != ""
}
