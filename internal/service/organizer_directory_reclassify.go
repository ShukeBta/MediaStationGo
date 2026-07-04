package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

type organizeExistingReclassifyRequest struct {
	Source          string
	Target          string
	DestRoot        string
	TargetLibraryID string
	Existing        []string
	DryRun          bool
	MediaType       string
	Category        string
	Title           string
	Year            int
	Season          int
	Episode         int
	MetadataMatch   *Match
	Result          *OrganizeResult
}

func (o *OrganizerService) reclassifyExistingMedia(ctx context.Context, req organizeExistingReclassifyRequest) (bool, error) {
	if req.Result == nil || len(req.Existing) == 0 {
		return false, nil
	}
	if strings.TrimSpace(req.Category) == "" {
		return false, nil
	}
	target := filepath.Clean(strings.TrimSpace(req.Target))
	if target == "" || target == "." {
		return false, nil
	}
	candidates := reclassifyExistingCandidates(req.Existing, target, req.DestRoot)
	candidates = reclassifyMoveCandidates(candidates, target)
	if len(candidates) == 0 {
		return false, nil
	}
	if organizeFileExists(target) {
		cleaned, err := o.cleanupReclassifiedDuplicates(ctx, req, target, candidates)
		return cleaned > 0, err
	}
	if len(candidates) != 1 || o.mediaPathExists(ctx, target) {
		return false, nil
	}
	oldPath := candidates[0]
	req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
		Source: oldPath, Target: target, Action: "reclassify",
		MediaType: req.MediaType, Category: req.Category, Title: req.Title,
		Reason: "metadata category changed",
	})
	if req.DryRun {
		req.Result.Reclassified++
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { // #nosec G301 -- organized media directories must remain readable by NAS/player users.
		return false, err
	}
	if err := moveFile(oldPath, target); err != nil {
		return false, err
	}
	if err := moveSidecarNFO(oldPath, target); err != nil && o != nil && o.log != nil {
		o.log.Warn("organize reclassify sidecar nfo failed",
			zap.String("from", nfoPath(oldPath)),
			zap.String("to", nfoPath(target)),
			zap.Error(err))
	}
	if err := o.updateReclassifiedMediaRow(ctx, oldPath, target, req); err != nil {
		return false, err
	}
	cleanupEmptyMediaDirs(filepath.Dir(oldPath), req.DestRoot)
	if o != nil && o.log != nil {
		o.log.Info("organize reclassified existing media",
			zap.String("from", oldPath),
			zap.String("to", target),
			zap.String("category", req.Category),
			zap.String("media_type", req.MediaType))
	}
	req.Result.Reclassified++
	return true, nil
}

func reclassifyExistingCandidates(existing []string, target, destRoot string) []string {
	target = filepath.Clean(target)
	destRoot = filepath.Clean(strings.TrimSpace(destRoot))
	seen := map[string]struct{}{}
	out := make([]string, 0, len(existing))
	for _, path := range existing {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." || strings.EqualFold(path, target) {
			continue
		}
		if destRoot != "" && destRoot != "." && !pathWithin(path, destRoot) {
			continue
		}
		if !organizeFileExists(path) {
			continue
		}
		key := strings.ToLower(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, path)
	}
	return out
}

func reclassifyMoveCandidates(existing []string, target string) []string {
	targetDir := filepath.Clean(filepath.Dir(target))
	out := make([]string, 0, len(existing))
	for _, path := range existing {
		cleaned := filepath.Clean(strings.TrimSpace(path))
		if cleaned == "" || cleaned == "." {
			continue
		}
		if strings.EqualFold(filepath.Clean(filepath.Dir(cleaned)), targetDir) {
			continue
		}
		out = append(out, cleaned)
	}
	return out
}

func (o *OrganizerService) cleanupReclassifiedDuplicates(ctx context.Context, req organizeExistingReclassifyRequest, target string, candidates []string) (int, error) {
	cleaned := 0
	for _, oldPath := range candidates {
		if !safeToRemoveReclassifiedDuplicate(oldPath, target) {
			moved, err := o.moveReclassifiedConflict(ctx, req, oldPath, target)
			if err != nil {
				return cleaned, err
			}
			if moved {
				cleaned++
			}
			continue
		}
		req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
			Source: oldPath, Target: target, Action: "cleanup",
			MediaType: req.MediaType, Category: req.Category, Title: req.Title,
			Reason: "duplicate after metadata category changed",
		})
		if req.DryRun {
			cleaned++
			continue
		}
		if err := removeMediaAndNFO(oldPath); err != nil {
			return cleaned, err
		}
		o.deleteMediaRowForPath(ctx, oldPath)
		cleanupEmptyMediaDirs(filepath.Dir(oldPath), req.DestRoot)
		if o != nil && o.log != nil {
			o.log.Info("organize cleaned duplicate after reclassify",
				zap.String("path", oldPath),
				zap.String("target", target),
				zap.String("category", req.Category),
				zap.String("media_type", req.MediaType))
		}
		cleaned++
	}
	req.Result.Reclassified += cleaned
	return cleaned, nil
}

func (o *OrganizerService) moveReclassifiedConflict(ctx context.Context, req organizeExistingReclassifyRequest, oldPath, target string) (bool, error) {
	conflictTarget := o.nextReclassifyConflictTarget(ctx, target)
	if conflictTarget == "" {
		if o != nil && o.log != nil {
			o.log.Warn("organize kept duplicate with different size during reclassify",
				zap.String("path", oldPath),
				zap.String("target", target))
		}
		return false, nil
	}
	req.Result.Items = append(req.Result.Items, OrganizePreviewItem{
		Source: oldPath, Target: conflictTarget, Action: "reclassify",
		MediaType: req.MediaType, Category: req.Category, Title: req.Title,
		Reason: "metadata category changed; target occupied by different file",
	})
	if req.DryRun {
		return true, nil
	}
	if err := os.MkdirAll(filepath.Dir(conflictTarget), 0o755); err != nil { // #nosec G301 -- organized media directories must remain readable by NAS/player users.
		return false, err
	}
	if err := moveFile(oldPath, conflictTarget); err != nil {
		return false, err
	}
	if err := moveSidecarNFO(oldPath, conflictTarget); err != nil && o != nil && o.log != nil {
		o.log.Warn("organize reclassify conflict sidecar nfo failed",
			zap.String("from", nfoPath(oldPath)),
			zap.String("to", nfoPath(conflictTarget)),
			zap.Error(err))
	}
	if err := o.updateReclassifiedMediaRow(ctx, oldPath, conflictTarget, req); err != nil {
		return false, err
	}
	cleanupEmptyMediaDirs(filepath.Dir(oldPath), req.DestRoot)
	if o != nil && o.log != nil {
		o.log.Info("organize moved conflicting duplicate to correct category",
			zap.String("from", oldPath),
			zap.String("to", conflictTarget),
			zap.String("occupied_target", target),
			zap.String("category", req.Category),
			zap.String("media_type", req.MediaType))
	}
	return true, nil
}

func (o *OrganizerService) nextReclassifyConflictTarget(ctx context.Context, target string) string {
	target = filepath.Clean(strings.TrimSpace(target))
	if target == "" || target == "." {
		return ""
	}
	dir := filepath.Dir(target)
	ext := filepath.Ext(target)
	base := strings.TrimSuffix(filepath.Base(target), ext)
	for i := 2; i <= 999; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if organizeFileExists(candidate) || o.mediaPathExists(ctx, candidate) {
			continue
		}
		return candidate
	}
	return ""
}
