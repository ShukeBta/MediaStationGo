package service

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const pipelineIngestMaxMediaIdentities = 1000

// PipelineIngestMediaIdentity carries an episode identity already established
// by the upstream ingest workflow. OpenListPath identifies one exact video file.
type PipelineIngestMediaIdentity struct {
	OpenListPath string `json:"openlist_path"`
	SeasonNum    int    `json:"season_num"`
	EpisodeNum   int    `json:"episode_num"`
}

func normalizePipelineIngestMediaIdentities(req *PipelineIngestRequest) error {
	if req == nil || len(req.TargetMediaIdentities) == 0 {
		return nil
	}
	if !req.Scan {
		return errors.New("target_media_identities require scan=true")
	}
	if len(req.TargetOpenListPaths) == 0 {
		return errors.New("target_media_identities require target_openlist_paths")
	}
	if len(req.TargetMediaIdentities) > pipelineIngestMaxMediaIdentities {
		return fmt.Errorf("target_media_identities exceed limit %d", pipelineIngestMaxMediaIdentities)
	}

	seen := make(map[string]struct{}, len(req.TargetMediaIdentities))
	normalized := make([]PipelineIngestMediaIdentity, 0, len(req.TargetMediaIdentities))
	for _, identity := range req.TargetMediaIdentities {
		identity.OpenListPath = pipelineNormalizeOpenListPath(identity.OpenListPath)
		if identity.OpenListPath == "" {
			return errors.New("target_media_identities openlist_path is required")
		}
		if _, ok := videoExtensions[strings.ToLower(path.Ext(identity.OpenListPath))]; !ok {
			return fmt.Errorf("target media identity is not a supported video file: %s", identity.OpenListPath)
		}
		if identity.SeasonNum < 0 || identity.SeasonNum > 99 {
			return fmt.Errorf("target media identity season_num must be between 0 and 99: %s", identity.OpenListPath)
		}
		if identity.EpisodeNum <= 0 || identity.EpisodeNum > 9999 {
			return fmt.Errorf("target media identity episode_num must be between 1 and 9999: %s", identity.OpenListPath)
		}
		if !pipelinePathIsSameOrChild(identity.OpenListPath, req.RootOpenListPath) {
			return fmt.Errorf("target media identity is outside library root: %s", identity.OpenListPath)
		}
		insideTarget := false
		for _, targetPath := range req.TargetOpenListPaths {
			if pipelinePathIsSameOrChild(identity.OpenListPath, targetPath) {
				insideTarget = true
				break
			}
		}
		if !insideTarget {
			return fmt.Errorf("target media identity is outside target_openlist_paths: %s", identity.OpenListPath)
		}
		if _, exists := seen[identity.OpenListPath]; exists {
			return fmt.Errorf("duplicate target media identity path: %s", identity.OpenListPath)
		}
		seen[identity.OpenListPath] = struct{}{}
		normalized = append(normalized, identity)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].OpenListPath < normalized[j].OpenListPath })
	req.TargetMediaIdentities = normalized
	return nil
}

func pipelineIngestMediaIdentityMap(values []PipelineIngestMediaIdentity) map[string]PipelineIngestMediaIdentity {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]PipelineIngestMediaIdentity, len(values))
	for _, identity := range values {
		out[pipelineOpenListPathToCloudPath(identity.OpenListPath)] = identity
	}
	return out
}

func validateCloudTargetMediaIdentities(candidates []cloudCandidate, identities map[string]PipelineIngestMediaIdentity) error {
	if len(identities) == 0 {
		return nil
	}
	found := make(map[string]struct{}, len(identities))
	for _, candidate := range candidates {
		identity, ok := identities[candidate.path]
		if !ok {
			continue
		}
		if _, duplicate := found[candidate.path]; duplicate {
			return fmt.Errorf("target media identity matched multiple scan candidates: %s", identity.OpenListPath)
		}
		if candidate.localMeta != nil && candidate.localMeta.EpisodeNum > 0 && candidate.localMeta.EpisodeNum != identity.EpisodeNum {
			return fmt.Errorf("target media identity conflicts with sidecar episode for %s: expected %d sidecar %d", identity.OpenListPath, identity.EpisodeNum, candidate.localMeta.EpisodeNum)
		}
		found[candidate.path] = struct{}{}
	}
	for cloudPath, identity := range identities {
		if _, ok := found[cloudPath]; !ok {
			return fmt.Errorf("target media identity file was not found in scan: %s", identity.OpenListPath)
		}
	}
	return nil
}

func (s *PipelineIngestService) verifyPipelineIngestMediaIdentities(ctx context.Context, expected []PipelineIngestMediaIdentity) ([]PipelineIngestMediaIdentity, error) {
	if len(expected) == 0 {
		return nil, nil
	}
	paths := make([]string, 0, len(expected))
	for _, identity := range expected {
		paths = append(paths, pipelineOpenListPathToCloudPath(identity.OpenListPath))
	}
	var rows []model.Media
	if err := s.repos.DB.WithContext(ctx).Where("deleted_at IS NULL AND path IN ?", paths).Find(&rows).Error; err != nil {
		return nil, err
	}
	byPath := make(map[string]model.Media, len(rows))
	for _, row := range rows {
		byPath[row.Path] = row
	}
	applied := make([]PipelineIngestMediaIdentity, 0, len(expected))
	for _, identity := range expected {
		cloudPath := pipelineOpenListPathToCloudPath(identity.OpenListPath)
		row, ok := byPath[cloudPath]
		if !ok {
			return nil, fmt.Errorf("target media identity was not persisted: %s", identity.OpenListPath)
		}
		if row.SeasonNum != identity.SeasonNum || row.EpisodeNum != identity.EpisodeNum {
			return nil, fmt.Errorf("target media identity persistence mismatch for %s: expected %d/%d actual %d/%d", identity.OpenListPath, identity.SeasonNum, identity.EpisodeNum, row.SeasonNum, row.EpisodeNum)
		}
		applied = append(applied, identity)
	}
	return applied, nil
}
