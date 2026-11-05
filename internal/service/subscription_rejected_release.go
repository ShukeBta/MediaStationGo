package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func rejectedReleaseIdentity(title, identityKey string) string {
	// Rejection applies to a specific release, so use the normalized full
	// title. DownloadTask identity keys intentionally collapse releases of the
	// same episode and are suitable only when no title text was stored.
	releaseKey := normalizedDownloadTitleKey(title)
	if strings.TrimSpace(title) == "" {
		releaseKey = strings.TrimSpace(identityKey)
	}
	if releaseKey == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToLower(releaseKey)))
	return hex.EncodeToString(sum[:])
}

func (s *SubscriptionService) rejectedReleaseIdentitySet(ctx context.Context, subscriptionID string) map[string]struct{} {
	out := map[string]struct{}{}
	if s == nil || s.repo == nil || s.repo.DB == nil || strings.TrimSpace(subscriptionID) == "" {
		return out
	}
	var rows []model.SubscriptionRejectedRelease
	if err := s.repo.DB.WithContext(ctx).
		Where("subscription_id = ?", strings.TrimSpace(subscriptionID)).
		Find(&rows).Error; err != nil {
		if s.log != nil {
			s.log.Warn("subscription rejected release list unavailable", zap.String("subscription_id", subscriptionID), zap.Error(err))
		}
		return out
	}
	for _, row := range rows {
		if row.ReleaseIdentity != "" {
			out[row.ReleaseIdentity] = struct{}{}
		}
	}
	return out
}

func (s *SubscriptionService) isRejectedSubscriptionRelease(ctx context.Context, subscriptionID, releaseIdentity string) bool {
	if s == nil || s.repo == nil || s.repo.DB == nil || strings.TrimSpace(subscriptionID) == "" || releaseIdentity == "" {
		return false
	}
	var count int64
	if err := s.repo.DB.WithContext(ctx).Model(&model.SubscriptionRejectedRelease{}).
		Where("subscription_id = ? AND release_identity = ?", strings.TrimSpace(subscriptionID), releaseIdentity).
		Count(&count).Error; err != nil {
		if s.log != nil {
			s.log.Warn("subscription rejected release lookup failed", zap.String("subscription_id", subscriptionID), zap.Error(err))
		}
		return false
	}
	return count > 0
}

func (s *SubscriptionService) recordRejectedSubscriptionEpisode(ctx context.Context, subID string, season, episode int, releaseIdentity, releaseTitle string) (int, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return 0, errRejectedReleaseStoreUnavailable
	}
	row := model.SubscriptionRejectedRelease{
		SubscriptionID: strings.TrimSpace(subID),
		SeasonNumber: season,
		EpisodeNumber: episode,
		ReleaseIdentity: releaseIdentity,
		ReleaseTitle: strings.TrimSpace(releaseTitle),
	}
	if err := s.repo.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "subscription_id"}, {Name: "season_number"}, {Name: "episode_number"}, {Name: "release_identity"}},
		DoNothing: true,
	}).Create(&row).Error; err != nil {
		return 0, err
	}
	var count int64
	if err := s.repo.DB.WithContext(ctx).Model(&model.SubscriptionRejectedRelease{}).
		Where("subscription_id = ? AND season_number = ? AND episode_number = ?", strings.TrimSpace(subID), season, episode).
		Distinct("release_identity").Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

var errRejectedReleaseStoreUnavailable = errors.New("subscription rejected release store unavailable")
