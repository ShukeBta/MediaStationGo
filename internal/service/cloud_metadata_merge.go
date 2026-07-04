package service

import "strings"

func mergeCloudMetadata(dst, src *LocalMetadata) *LocalMetadata {
	if src == nil {
		return dst
	}
	if dst == nil {
		return cloneLocalMetadata(src)
	}
	if src.Title != "" {
		dst.Title = src.Title
	}
	if src.OriginalName != "" {
		dst.OriginalName = src.OriginalName
	}
	if src.EpisodeTitle != "" {
		dst.EpisodeTitle = src.EpisodeTitle
	}
	if src.AdultCode != "" {
		dst.AdultCode = src.AdultCode
	}
	if src.Year > 0 {
		dst.Year = src.Year
	}
	if src.ReleaseDate != "" {
		dst.ReleaseDate = src.ReleaseDate
	}
	if src.Overview != "" {
		dst.Overview = src.Overview
	}
	if src.Rating > 0 {
		dst.Rating = src.Rating
	}
	if src.PosterURL != "" {
		dst.PosterURL = src.PosterURL
	}
	if src.BackdropURL != "" {
		dst.BackdropURL = src.BackdropURL
	}
	if src.TMDbID > 0 {
		dst.TMDbID = src.TMDbID
	}
	if src.BangumiID > 0 {
		dst.BangumiID = src.BangumiID
	}
	if src.DoubanID != "" {
		dst.DoubanID = src.DoubanID
	}
	if src.TheTVDBID != "" {
		dst.TheTVDBID = src.TheTVDBID
	}
	if src.SeasonNum > 0 || src.EpisodeNum > 0 {
		dst.SeasonNum = src.SeasonNum
	}
	if src.EpisodeNum > 0 {
		dst.EpisodeNum = src.EpisodeNum
	}
	if src.Genres != "" {
		dst.Genres = src.Genres
	}
	if src.Countries != "" {
		dst.Countries = src.Countries
	}
	if src.Languages != "" {
		dst.Languages = src.Languages
	}
	dst.NSFW = dst.NSFW || src.NSFW
	dst.HasNFO = dst.HasNFO || src.HasNFO
	dst.HasArtwork = dst.HasArtwork || src.HasArtwork
	dst.PathHint = dst.PathHint || src.PathHint
	return dst
}

func mergeCloudPathHintMetadata(dst, hint *LocalMetadata) *LocalMetadata {
	if hint == nil {
		return dst
	}
	if dst == nil || !dst.HasNFO {
		return mergeCloudMetadata(dst, hint)
	}
	if dst.Title == "" {
		dst.Title = hint.Title
	}
	if dst.OriginalName == "" {
		dst.OriginalName = hint.OriginalName
	}
	if dst.Year == 0 {
		dst.Year = hint.Year
	}
	if dst.ReleaseDate == "" {
		dst.ReleaseDate = hint.ReleaseDate
	}
	if dst.TMDbID == 0 {
		dst.TMDbID = hint.TMDbID
	}
	if dst.BangumiID == 0 {
		dst.BangumiID = hint.BangumiID
	}
	if dst.DoubanID == "" {
		dst.DoubanID = hint.DoubanID
	}
	if dst.TheTVDBID == "" {
		dst.TheTVDBID = hint.TheTVDBID
	}
	dst.PathHint = dst.PathHint || hint.PathHint
	return dst
}

func cloneLocalMetadata(src *LocalMetadata) *LocalMetadata {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func cloudMetadataUseful(meta *LocalMetadata) bool {
	return meta != nil && (meta.HasNFO || meta.HasArtwork || localHasDescriptiveMetadata(meta))
}

func cloudPlaybackURL(typ, ref string) string {
	return CloudArtworkURL(typ, ref)
}

func joinCloudDisplayPath(parent, child string) string {
	parent = strings.Trim(strings.ReplaceAll(strings.TrimSpace(parent), "\\", "/"), "/")
	child = strings.Trim(strings.ReplaceAll(strings.TrimSpace(child), "\\", "/"), "/")
	switch {
	case parent == "":
		return child
	case child == "":
		return parent
	default:
		return parent + "/" + child
	}
}

func pathBaseSlash(value string) string {
	value = strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/")
	if value == "" {
		return ""
	}
	parts := strings.Split(value, "/")
	return parts[len(parts)-1]
}

func pathDirSlash(value string) string {
	value = strings.Trim(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "/")
	if value == "" {
		return ""
	}
	idx := strings.LastIndex(value, "/")
	if idx < 0 {
		return ""
	}
	return value[:idx]
}
