package service

import (
	"net/url"
	"path"
	"strings"
)

func applyCloudNFOArtwork(typ string, sidecars cloudSidecarSet, meta *LocalMetadata, doc *nfoDocument) *LocalMetadata {
	if meta == nil {
		meta = &LocalMetadata{}
	}
	if doc == nil {
		return meta
	}
	if ref := cloudImageRefFromNFOValues(sidecars, nfoPosterValues(doc)...); ref != "" {
		meta.PosterURL = cloudPlaybackURL(typ, ref)
		meta.HasArtwork = true
	}
	if ref := cloudImageRefFromNFOValues(sidecars, nfoBackdropValues(doc)...); ref != "" {
		meta.BackdropURL = cloudPlaybackURL(typ, ref)
		meta.HasArtwork = true
	}
	return meta
}

func applyCloudDirectoryArtwork(typ, displayDir string, sidecars cloudSidecarSet, meta *LocalMetadata) *LocalMetadata {
	if meta == nil {
		meta = &LocalMetadata{}
	}
	if meta.PosterURL == "" {
		if ref := firstCloudImageRef(sidecars, cloudPosterNameCandidates(cloudDirectoryArtworkBases(displayDir), "poster", "folder", "cover", "show", "tvshow")...); ref != "" {
			meta.PosterURL = cloudPlaybackURL(typ, ref)
			meta.HasArtwork = true
		}
	}
	if meta.BackdropURL == "" {
		if ref := firstCloudImageRef(sidecars, cloudBackdropNameCandidates(cloudDirectoryArtworkBases(displayDir), "fanart", "backdrop", "background", "landscape")...); ref != "" {
			meta.BackdropURL = cloudPlaybackURL(typ, ref)
			meta.HasArtwork = true
		}
	}
	return meta
}

func applyCloudFileArtwork(typ string, sidecars cloudSidecarSet, displayPath, fileName, base string, meta *LocalMetadata) *LocalMetadata {
	if meta == nil {
		meta = &LocalMetadata{}
	}
	bases := cloudFileArtworkBases(displayPath, fileName, base)
	if meta.PosterURL == "" {
		if ref := firstCloudImageRef(sidecars, cloudPosterNameCandidates(bases, "poster", "folder", "cover", "movie", "show", "thumb")...); ref != "" {
			meta.PosterURL = cloudPlaybackURL(typ, ref)
			meta.HasArtwork = true
		}
	}
	if meta.BackdropURL == "" {
		if ref := firstCloudImageRef(sidecars, cloudBackdropNameCandidates(bases, "fanart", "backdrop", "background", "landscape")...); ref != "" {
			meta.BackdropURL = cloudPlaybackURL(typ, ref)
			meta.HasArtwork = true
		}
	}
	return meta
}

func firstCloudImageRef(sidecars cloudSidecarSet, names ...string) string {
	for _, name := range names {
		if ref := cloudImageRefByName(sidecars, name); ref != "" {
			return ref
		}
	}
	return ""
}

func cloudImageRefFromNFOValues(sidecars cloudSidecarSet, values ...string) string {
	for _, value := range values {
		if ref := cloudImageRefByName(sidecars, value); ref != "" {
			return ref
		}
	}
	return ""
}

func cloudImageRefByName(sidecars cloudSidecarSet, value string) string {
	name := normalizeCloudArtworkName(value)
	if name == "" || isHTTPURL(name) {
		return ""
	}
	if ref := sidecars.imageByName[strings.ToLower(name)]; ref != "" {
		return ref
	}
	base := strings.TrimSuffix(name, path.Ext(name))
	if ref := sidecars.imageByBase[strings.ToLower(base)]; ref != "" {
		return ref
	}
	return ""
}

func normalizeCloudArtworkName(value string) string {
	value = cleanXMLText(value)
	if value == "" {
		return ""
	}
	if isHTTPURL(value) {
		return value
	}
	if unescaped, err := url.QueryUnescape(value); err == nil {
		value = unescaped
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if idx := strings.IndexAny(value, "?#"); idx >= 0 {
		value = value[:idx]
	}
	value = strings.Trim(strings.TrimSpace(value), "/")
	if value == "" {
		return ""
	}
	return path.Base(value)
}
