package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type pipelineIngestNeedsAttentionError struct {
	reason string
}

func (e *pipelineIngestNeedsAttentionError) Error() string {
	if e == nil || e.reason == "" {
		return "target tree did not converge within 10 minutes"
	}
	return e.reason
}

func pipelineIngestStatusActive(status string) bool {
	switch status {
	case PipelineIngestStatusAccepted, PipelineIngestStatusConverging, PipelineIngestStatusRunning:
		return true
	default:
		return false
	}
}

func waitPipelineIngestDuration(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func pipelineTreeManifest(value cloudTreeManifest) PipelineIngestTreeManifest {
	return PipelineIngestTreeManifest{
		EntryCount:     value.EntryCount,
		DirectoryCount: value.DirectoryCount,
		FileCount:      value.FileCount,
		TotalFileSize:  value.TotalFileSize,
		Fingerprint:    value.Fingerprint,
	}
}

func cloudTreeManifestFromPipeline(value PipelineIngestTreeManifest) cloudTreeManifest {
	return cloudTreeManifest{
		EntryCount:     value.EntryCount,
		DirectoryCount: value.DirectoryCount,
		FileCount:      value.FileCount,
		TotalFileSize:  value.TotalFileSize,
		Fingerprint:    value.Fingerprint,
	}
}

func (s *PipelineIngestService) scanForPipelineIngestConverged(ctx context.Context, id string, target pipelineResolvedTarget, req PipelineIngestRequest, task *TaskHandle) (*ScanResult, []PipelineIngestIgnoredMedia, error) {
	job, err := s.Get(id)
	if err != nil {
		return nil, nil, err
	}
	stableWindow := s.stableWindow
	if stableWindow <= 0 {
		stableWindow = pipelineIngestStableWindow
	}
	maxConvergence := s.maxConvergence
	if maxConvergence <= 0 {
		maxConvergence = pipelineIngestMaxConvergence
	}
	startedAt := job.StartedAt
	deadline := startedAt.Add(maxConvergence)
	convergenceCtx, cancelConvergence := context.WithDeadline(ctx, deadline)
	defer cancelConvergence()
	previous := cloudTreeManifest{}
	var stableSince *time.Time
	attempt := 0
	if job.Result.Convergence != nil {
		attempt = job.Result.Convergence.Attempt
		previous = cloudTreeManifestFromPipeline(job.Result.Convergence.Manifest)
		stableSince = cloneTimePointer(job.Result.Convergence.StableSince)
	}
	accumulated := &ScanResult{LibraryID: target.LibraryID}
	var latestIgnored []PipelineIngestIgnoredMedia
	var latestErr error
	refreshParents := attempt == 0 && !req.TargetParentsVerified
	refreshDirs := map[string]struct{}{}

	for {
		attempt++
		now := s.currentTime()
		if !now.Before(deadline) {
			return accumulated, latestIgnored, s.finishPipelineIngestNeedsAttention(id, startedAt, now, attempt, previous, latestErr, maxConvergence)
		}
		if err := s.updateJob(id, PipelineIngestStatusConverging, "waiting for target tree to converge", func(job *PipelineIngestJob) {
			job.Status = PipelineIngestStatusConverging
		}); err != nil {
			return accumulated, latestIgnored, err
		}
		if task != nil {
			task.Update(TaskUpdate{Stage: PipelineIngestStatusConverging, Message: "waiting for target tree to converge"})
		}

		finish, ok := s.scanner.TryBeginLocalScan("pipeline-ingest:" + target.LibraryID + ":" + target.RootID)
		if !ok {
			latestErr = errors.New("library root scan already running")
			if err := s.persistPipelineIngestConvergence(id, startedAt, now, attempt, previous, stableSince, false, latestErr, deadline, stableWindow); err != nil {
				return accumulated, latestIgnored, err
			}
		} else {
			current, ignored, manifest, handled, scanErr := s.scanForPipelineIngestWithOptions(convergenceCtx, target, req, cloudTargetScanOptions{
				strictListErrors:     true,
				refreshDepth:         0,
				refreshDirs:          refreshDirs,
				refreshTargetParents: refreshParents,
			})
			finish()
			refreshParents = false
			if current != nil {
				mergePipelineIngestScanResult(accumulated, current)
			}
			latestIgnored = ignored
			if errors.Is(convergenceCtx.Err(), context.DeadlineExceeded) {
				if scanErr == nil {
					scanErr = context.DeadlineExceeded
				}
				observedAt := s.currentTime()
				if observedAt.Before(deadline) {
					observedAt = deadline
				}
				return accumulated, latestIgnored, s.finishPipelineIngestNeedsAttention(id, startedAt, observedAt, attempt, manifest, scanErr, maxConvergence)
			}
			if !handled && scanErr == nil {
				scanErr = errors.New("target_openlist_paths were provided but could not be handled by the OpenList target scanner")
			}
			now = s.currentTime()
			if scanErr != nil {
				latestErr = scanErr
				refreshDirs = failedCloudTreeDirs(scanErr)
				if err := s.persistPipelineIngestConvergence(id, startedAt, now, attempt, manifest, stableSince, false, scanErr, deadline, stableWindow); err != nil {
					return accumulated, latestIgnored, err
				}
				previous = manifest
				stableSince = nil
			} else {
				latestErr = nil
				refreshDirs = map[string]struct{}{}
				changed := !cloudTreeManifestsEqual(previous, manifest)
				if changed {
					stableAt := now
					stableSince = &stableAt
				} else if stableSince != nil && manifest.FileCount > 0 && now.Sub(*stableSince) >= stableWindow {
					completedAt := now
					convergence := PipelineIngestConvergenceResult{
						Status: PipelineIngestStatusCompleted, Attempt: attempt,
						StableForSeconds: int(now.Sub(*stableSince).Seconds()), MaxWaitSeconds: int(maxConvergence.Seconds()),
						Manifest: pipelineTreeManifest(manifest), Changed: false,
						StartedAt: startedAt, StableSince: cloneTimePointer(stableSince), ObservedAt: now, CompletedAt: &completedAt,
					}
					if err := s.updateJobResult(id, func(result *PipelineIngestResult) {
						result.Scan = ptrPipelineIngestScanSummary(accumulated)
						result.IgnoredMedia = latestIgnored
						result.Convergence = &convergence
					}); err != nil {
						return accumulated, latestIgnored, err
					}
					return accumulated, latestIgnored, nil
				}
				if err := s.persistPipelineIngestConvergence(id, startedAt, now, attempt, manifest, stableSince, changed, nil, deadline, stableWindow); err != nil {
					return accumulated, latestIgnored, err
				}
				previous = manifest
			}
		}

		now = s.currentTime()
		if !now.Before(deadline) {
			return accumulated, latestIgnored, s.finishPipelineIngestNeedsAttention(id, startedAt, now, attempt, previous, latestErr, maxConvergence)
		}
		waitFor := stableWindow
		if remaining := deadline.Sub(now); waitFor > remaining {
			waitFor = remaining
		}
		if err := s.wait(convergenceCtx, waitFor); err != nil {
			if errors.Is(convergenceCtx.Err(), context.DeadlineExceeded) {
				observedAt := s.currentTime()
				if observedAt.Before(deadline) {
					observedAt = deadline
				}
				return accumulated, latestIgnored, s.finishPipelineIngestNeedsAttention(id, startedAt, observedAt, attempt, previous, latestErr, maxConvergence)
			}
			return accumulated, latestIgnored, err
		}
	}
}

func failedCloudTreeDirs(err error) map[string]struct{} {
	dirs := map[string]struct{}{}
	var treeErr *cloudTreeWalkError
	if !errors.As(err, &treeErr) {
		return dirs
	}
	for _, dirID := range treeErr.FailedDirs() {
		dirs[normalizeCloudMountDir("", dirID)] = struct{}{}
	}
	return dirs
}

func (s *PipelineIngestService) persistPipelineIngestConvergence(id string, startedAt, observedAt time.Time, attempt int, manifest cloudTreeManifest, stableSince *time.Time, changed bool, scanErr error, deadline time.Time, stableWindow time.Duration) error {
	next := observedAt.Add(stableWindow)
	if next.After(deadline) {
		next = deadline
	}
	convergence := PipelineIngestConvergenceResult{
		Status: PipelineIngestStatusConverging, Attempt: attempt,
		MaxWaitSeconds: int(deadline.Sub(startedAt).Seconds()), Manifest: pipelineTreeManifest(manifest), Changed: changed,
		StartedAt: startedAt, StableSince: cloneTimePointer(stableSince), ObservedAt: observedAt, NextProbeAt: &next,
	}
	if stableSince != nil {
		convergence.StableForSeconds = int(observedAt.Sub(*stableSince).Seconds())
	}
	convergence.ErrorCount, convergence.Errors = pipelineIngestErrorDetails(scanErr)
	return s.updateJobResult(id, func(result *PipelineIngestResult) {
		result.Convergence = &convergence
	})
}

func (s *PipelineIngestService) finishPipelineIngestNeedsAttention(id string, startedAt, observedAt time.Time, attempt int, manifest cloudTreeManifest, scanErr error, maxConvergence time.Duration) error {
	convergence := PipelineIngestConvergenceResult{
		Status: PipelineIngestStatusNeedsAttention, Attempt: attempt,
		MaxWaitSeconds: int(maxConvergence.Seconds()), Manifest: pipelineTreeManifest(manifest),
		StartedAt: startedAt, ObservedAt: observedAt,
	}
	convergence.ErrorCount, convergence.Errors = pipelineIngestErrorDetails(scanErr)
	_ = s.updateJobResult(id, func(result *PipelineIngestResult) { result.Convergence = &convergence })
	return &pipelineIngestNeedsAttentionError{reason: fmt.Sprintf("target tree did not converge within %d minutes", int(maxConvergence.Minutes()))}
}

func pipelineIngestErrorDetails(err error) (int, []string) {
	if err == nil {
		return 0, nil
	}
	var treeErr *cloudTreeWalkError
	if errors.As(err, &treeErr) && len(treeErr.errors) > 0 {
		return len(treeErr.errors), append([]string(nil), treeErr.errors...)
	}
	return 1, []string{err.Error()}
}

func mergePipelineIngestScanResult(total, current *ScanResult) {
	if total == nil || current == nil {
		return
	}
	total.LibraryID = current.LibraryID
	total.Visited = current.Visited
	total.Added += current.Added
	total.Updated += current.Updated
	total.Skipped = current.Skipped
	total.Probed += current.Probed
	total.LocalMetadata += current.LocalMetadata
	total.Removed += current.Removed
	total.ErrorCount = current.ErrorCount
	total.Errors = append([]string(nil), current.Errors...)
}

func ptrPipelineIngestScanSummary(result *ScanResult) *PipelineIngestScanResult {
	summary := pipelineIngestScanSummary(result)
	return &summary
}
