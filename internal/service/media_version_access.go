package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var (
	ErrMediaVersionNotFound  = errors.New("media version not found")
	ErrMediaVersionForbidden = errors.New("media version management forbidden")
	ErrMediaVersionKeyStale  = errors.New("media version key is stale")
)

var successfulResourceImportStatuses = []string{
	ResourceImportStatusCompleted,
	ResourceImportStatusCompletedWithWarning,
}

type MediaVersionItem struct {
	model.Media
	CanManage bool `json:"can_manage"`
	IsCurrent bool `json:"is_current"`
}

type MediaVersionList struct {
	Items             []MediaVersionItem `json:"items"`
	CanManageVersions bool               `json:"can_manage_versions"`
}

type DeleteMediaVersionResult struct {
	DeletedID   string `json:"deleted_id"`
	NextMediaID string `json:"next_media_id,omitempty"`
}

func (s *MediaService) CanManageVersion(ctx context.Context, userID string, isAdmin bool, mediaID string) (bool, error) {
	if s == nil {
		return false, errors.New("media service unavailable")
	}
	return userCanManageMediaVersion(ctx, s.repo, userID, isAdmin, mediaID)
}

func (s *MediaService) ListMediaVersions(ctx context.Context, mediaID, userID string, isAdmin bool) (MediaVersionList, error) {
	if s == nil || s.repo == nil || s.repo.Media == nil || s.repo.DB == nil {
		return MediaVersionList{}, errors.New("media service unavailable")
	}
	anchor, err := s.repo.Media.FindByID(ctx, strings.TrimSpace(mediaID))
	if err != nil {
		return MediaVersionList{}, err
	}
	if anchor == nil {
		return MediaVersionList{}, ErrMediaVersionNotFound
	}
	groupKey := mediaVersionGroupKey(*anchor)
	rows := []model.Media{*anchor}
	if groupKey != "" && strings.TrimSpace(anchor.PartGroupKey) == "" {
		persistedKey := mediaVersionPersistedKey(*anchor)
		candidates, err := s.repo.Media.ListMediaByVersionGroupKeys(
			ctx,
			[]string{persistedKey},
			[]string{anchor.LibraryID},
			repository.MediaQueryFilter{IncludeNSFW: true},
		)
		if err != nil {
			return MediaVersionList{}, err
		}
		rows = rows[:0]
		anchorFound := false
		for _, candidate := range candidates {
			if mediaVersionGroupKey(candidate) != groupKey {
				return MediaVersionList{}, fmt.Errorf("%w: candidate %s", ErrMediaVersionKeyStale, candidate.ID)
			}
			anchorFound = anchorFound || candidate.ID == anchor.ID
			rows = append(rows, candidate)
		}
		if !anchorFound {
			return MediaVersionList{}, fmt.Errorf("%w: anchor %s", ErrMediaVersionKeyStale, anchor.ID)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return betterMediaVersion(rows[i], rows[j]) })

	owned, err := mediaVersionsOwnedByUser(ctx, s.repo, userID, rows)
	if err != nil {
		return MediaVersionList{}, err
	}
	items := make([]MediaVersionItem, 0, len(rows))
	canManageVersions := false
	for _, row := range rows {
		canManage := isAdmin || owned[row.ID]
		if canManage {
			canManageVersions = true
		}
		items = append(items, MediaVersionItem{
			Media: row, CanManage: canManage, IsCurrent: row.ID == anchor.ID,
		})
	}
	return MediaVersionList{Items: items, CanManageVersions: canManageVersions}, nil
}

func (s *MediaService) DeleteMediaVersion(ctx context.Context, anchorID, versionID, userID string, isAdmin bool) (DeleteMediaVersionResult, error) {
	versions, err := s.ListMediaVersions(ctx, anchorID, userID, isAdmin)
	if err != nil {
		return DeleteMediaVersionResult{}, err
	}
	var target *MediaVersionItem
	for i := range versions.Items {
		if versions.Items[i].ID == strings.TrimSpace(versionID) {
			target = &versions.Items[i]
			break
		}
	}
	if target == nil {
		return DeleteMediaVersionResult{}, ErrMediaVersionNotFound
	}
	if !target.CanManage {
		return DeleteMediaVersionResult{}, ErrMediaVersionForbidden
	}
	nextMediaID := ""
	for _, item := range versions.Items {
		if item.ID != target.ID {
			nextMediaID = item.ID
			break
		}
	}
	if err := s.SoftDeleteBy(ctx, target.ID, userID, "version"); err != nil {
		return DeleteMediaVersionResult{}, err
	}
	return DeleteMediaVersionResult{DeletedID: target.ID, NextMediaID: nextMediaID}, nil
}

func userCanManageMediaVersion(ctx context.Context, repos *repository.Container, userID string, isAdmin bool, mediaID string) (bool, error) {
	if isAdmin {
		return true, nil
	}
	if repos == nil || repos.DB == nil {
		return false, errors.New("media ownership store unavailable")
	}
	if repos.User != nil {
		user, err := repos.User.FindByID(ctx, strings.TrimSpace(userID))
		if err != nil {
			return false, err
		}
		if user != nil && strings.EqualFold(strings.TrimSpace(user.Role), "admin") {
			return true, nil
		}
	}
	var count int64
	err := repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).
		Where("user_id = ? AND media_id = ? AND status IN ?", strings.TrimSpace(userID), strings.TrimSpace(mediaID), successfulResourceImportStatuses).
		Count(&count).Error
	return count > 0, err
}

func mediaVersionsOwnedByUser(ctx context.Context, repos *repository.Container, userID string, rows []model.Media) (map[string]bool, error) {
	owned := make(map[string]bool)
	if repos == nil || repos.DB == nil || strings.TrimSpace(userID) == "" || len(rows) == 0 {
		return owned, nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	var ownedIDs []string
	if err := repos.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).
		Distinct("media_id").
		Where("user_id = ? AND media_id IN ? AND status IN ?", strings.TrimSpace(userID), ids, successfulResourceImportStatuses).
		Pluck("media_id", &ownedIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range ownedIDs {
		owned[id] = true
	}
	return owned, nil
}
