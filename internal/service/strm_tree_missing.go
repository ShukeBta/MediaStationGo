package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *STRMService) strmTreeExistingCloudRefs(ctx context.Context, opts GenerateSTRMTreeOptions) (map[string]struct{}, error) {
	if !opts.MissingOnly {
		return nil, nil
	}
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return nil, errors.New("media library unavailable")
	}
	var rows []model.Media
	if err := s.repo.DB.WithContext(ctx).
		Select("path", "strm_url").
		Where("strm_url <> '' OR path LIKE ?", "cloud://%").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	refs := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if typ, ref, ok := parseCloudMediaPlaybackURL(row.STRMURL); ok {
			refs[strmTreeCloudRefKey(typ, ref)] = struct{}{}
		}
		if typ, ref, ok := strmTreeCloudPathTarget(row.Path); ok {
			refs[strmTreeCloudRefKey(typ, ref)] = struct{}{}
		}
	}
	return refs, nil
}

func strmTreeSourceAlreadyInLibrary(source strmTreeSource, opts GenerateSTRMTreeOptions, existingRefs map[string]struct{}) bool {
	if len(existingRefs) == 0 || source.Provider == "" || source.Path == "" || source.Kind == strmTreeSourceKindSubtitle {
		return false
	}
	ref := strmTreeCloudRef(source.cloudRefPath(), opts.SourceRoot)
	_, ok := existingRefs[strmTreeCloudRefKey(source.Provider, ref)]
	return ok
}

func strmTreeCloudPathTarget(raw string) (string, string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "cloud://") {
		return "", "", false
	}
	rest := strings.TrimPrefix(raw, "cloud://")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	typ := strings.TrimSpace(parts[0])
	ref := strings.TrimSpace(parts[1])
	return typ, ref, typ != "" && ref != ""
}

func strmTreeCloudRefKey(provider, ref string) string {
	return strings.ToLower(normalizeSTRMTreeProvider(provider)) + "\x00" + strmTreeNormalizeCloudRef(ref)
}

func strmTreeNormalizeCloudRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if decoded, err := url.PathUnescape(ref); err == nil {
		ref = decoded
	}
	ref = strings.TrimSpace(strings.ReplaceAll(ref, "\\", "/"))
	return strings.ToLower(strings.TrimLeft(ref, "/"))
}
