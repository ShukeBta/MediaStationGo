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
		return "strict target scan needs attention"
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

// scanForPipelineIngestConverged gives OpenList one bounded visibility delay,
// then performs exactly one strict target-tree scan. A list failure cancels the
// remaining walk and becomes needs_attention; this path never retries itself.
func (s *PipelineIngestService) scanForPipelineIngestConverged(ctx context.Context, id string, target pipelineResolvedTarget, req PipelineIngestRequest, task *TaskHandle) (*ScanResult, []PipelineIngestIgnoredMedia, error) {
	job, err := s.Get(id)
	if err != nil {
		return nil, nil, err
	}
	stableWindow := s.stableWindow
	if stableWindow <= 0 {
		stableWindow = pipelineIngestStableWindow
	}
	maxWait := s.maxConvergence
	if maxWait <= 0 {
		maxWait = pipelineIngestMaxConvergence
	}
	startedAt := job.StartedAt
	deadline := startedAt.Add(maxWait)
	strictCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	if convergence := job.Result.Convergence; convergence != nil && convergence.Attempt >= 1 {
		reason := "strict target scan was interrupted; manual retry is required"
		return nil, nil, s.finishPipelineIngestNeedsAttention(id, startedAt, s.currentTime(), convergence.Attempt, cloudTreeManifest{}, errors.New(reason), maxWait, reason, nil)
	}

	now := s.currentTime()
	if !now.Before(deadline) {
		reason := fmt.Sprintf("strict target scan exceeded the %d minute limit before it started", int(maxWait.Minutes()))
		return nil, nil, s.finishPipelineIngestNeedsAttention(id, startedAt, now, 0, cloudTreeManifest{}, context.DeadlineExceeded, maxWait, reason, nil)
	}

	settleUntil := startedAt.Add(stableWindow)
	if now.Before(settleUntil) {
		if settleUntil.After(deadline) {
			settleUntil = deadline
		}
		message := fmt.Sprintf("waiting %d seconds for OpenList visibility", int(settleUntil.Sub(now).Seconds()))
		if err := s.updateJob(id, PipelineIngestStatusConverging, message, func(job *PipelineIngestJob) {
			job.Status = PipelineIngestStatusConverging
		}); err != nil {
			return nil, nil, err
		}
		if task != nil {
			task.Update(TaskUpdate{Stage: PipelineIngestStatusConverging, Message: message})
		}
		if err := s.updateJobResult(id, func(result *PipelineIngestResult) {
			result.Convergence = &PipelineIngestConvergenceResult{
				Status: PipelineIngestStatusConverging, Attempt: 0,
				MaxWaitSeconds: int(maxWait.Seconds()), StartedAt: startedAt,
				ObservedAt: now, NextProbeAt: cloneTimePointer(&settleUntil),
			}
		}); err != nil {
			return nil, nil, err
		}
		if err := s.wait(strictCtx, settleUntil.Sub(now)); err != nil {
			if errors.Is(strictCtx.Err(), context.DeadlineExceeded) {
				observedAt := s.currentTime()
				reason := fmt.Sprintf("strict target scan exceeded the %d minute limit while waiting for OpenList visibility", int(maxWait.Minutes()))
				return nil, nil, s.finishPipelineIngestNeedsAttention(id, startedAt, observedAt, 0, cloudTreeManifest{}, err, maxWait, reason, nil)
			}
			return nil, nil, err
		}
	}

	const attempt = 1
	now = s.currentTime()
	if !now.Before(deadline) {
		reason := fmt.Sprintf("strict target scan exceeded the %d minute limit before it started", int(maxWait.Minutes()))
		return nil, nil, s.finishPipelineIngestNeedsAttention(id, startedAt, now, attempt, cloudTreeManifest{}, context.DeadlineExceeded, maxWait, reason, nil)
	}
	if err := s.updateJob(id, PipelineIngestStatusRunning, "running one strict target scan", func(job *PipelineIngestJob) {
		job.Status = PipelineIngestStatusRunning
	}); err != nil {
		return nil, nil, err
	}
	if task != nil {
		task.Update(TaskUpdate{Stage: PipelineIngestStatusRunning, Message: "running one strict target scan"})
	}
	if err := s.updateJobResult(id, func(result *PipelineIngestResult) {
		result.Convergence = &PipelineIngestConvergenceResult{
			Status: PipelineIngestStatusRunning, Attempt: attempt,
			MaxWaitSeconds: int(maxWait.Seconds()), StartedAt: startedAt, ObservedAt: now,
		}
	}); err != nil {
		return nil, nil, err
	}

	finish, ok := s.scanner.TryBeginLocalScan("pipeline-ingest:" + target.LibraryID + ":" + target.RootID)
	if !ok {
		reason := "library root scan is already running; strict target scan was not retried"
		return nil, nil, s.finishPipelineIngestNeedsAttention(id, startedAt, s.currentTime(), attempt, cloudTreeManifest{}, errors.New(reason), maxWait, reason, nil)
	}
	parentResolution := &cloudTargetResolutionDiagnostic{}
	scanResult, ignored, manifest, handled, scanErr := s.scanForPipelineIngestWithOptions(strictCtx, target, req, cloudTargetScanOptions{
		strictListErrors:           true,
		refreshDepth:               0,
		refreshTargetParents:       false,
		targetResolutionDiagnostic: parentResolution,
	})
	finish()

	if errors.Is(strictCtx.Err(), context.DeadlineExceeded) {
		if scanErr == nil {
			scanErr = context.DeadlineExceeded
		}
		reason := fmt.Sprintf("strict target scan exceeded the %d minute limit", int(maxWait.Minutes()))
		return scanResult, ignored, s.finishPipelineIngestNeedsAttention(id, startedAt, s.currentTime(), attempt, manifest, scanErr, maxWait, reason, parentResolution)
	}
	if !handled && scanErr == nil {
		scanErr = errors.New("target_openlist_paths were provided but could not be handled by the OpenList target scanner")
	}
	if scanErr != nil {
		reason := "strict target scan failed; no automatic retry was made"
		return scanResult, ignored, s.finishPipelineIngestNeedsAttention(id, startedAt, s.currentTime(), attempt, manifest, scanErr, maxWait, reason, parentResolution)
	}
	if manifest.FileCount == 0 {
		reason := "strict target scan found no media files; no automatic retry was made"
		return scanResult, ignored, s.finishPipelineIngestNeedsAttention(id, startedAt, s.currentTime(), attempt, manifest, errors.New(reason), maxWait, reason, parentResolution)
	}

	completedAt := s.currentTime()
	stableSince := startedAt.Add(stableWindow)
	convergence := PipelineIngestConvergenceResult{
		Status: PipelineIngestStatusCompleted, Attempt: attempt,
		StableForSeconds: int(stableWindow.Seconds()), MaxWaitSeconds: int(maxWait.Seconds()),
		Manifest: pipelineTreeManifest(manifest), Changed: false,
		StartedAt: startedAt, StableSince: &stableSince, ObservedAt: completedAt, CompletedAt: &completedAt,
	}
	applyCloudTargetResolutionDiagnostic(&convergence, parentResolution)
	if err := s.updateJobResult(id, func(result *PipelineIngestResult) {
		result.Scan = ptrPipelineIngestScanSummary(scanResult)
		result.IgnoredMedia = ignored
		result.Convergence = &convergence
	}); err != nil {
		return scanResult, ignored, err
	}
	return scanResult, ignored, nil
}

func (s *PipelineIngestService) finishPipelineIngestNeedsAttention(id string, startedAt, observedAt time.Time, attempt int, manifest cloudTreeManifest, scanErr error, maxWait time.Duration, reason string, parentResolution *cloudTargetResolutionDiagnostic) error {
	convergence := PipelineIngestConvergenceResult{
		Status: PipelineIngestStatusNeedsAttention, Attempt: attempt,
		MaxWaitSeconds: int(maxWait.Seconds()), Manifest: pipelineTreeManifest(manifest),
		StartedAt: startedAt, ObservedAt: observedAt,
	}
	applyCloudTargetResolutionDiagnostic(&convergence, parentResolution)
	convergence.ErrorCount, convergence.Errors = pipelineIngestErrorDetails(scanErr)
	if err := s.updateJobResult(id, func(result *PipelineIngestResult) { result.Convergence = &convergence }); err != nil {
		return err
	}
	return &pipelineIngestNeedsAttentionError{reason: reason}
}

func applyCloudTargetResolutionDiagnostic(convergence *PipelineIngestConvergenceResult, diagnostic *cloudTargetResolutionDiagnostic) {
	if convergence == nil || diagnostic == nil {
		return
	}
	convergence.ParentCacheMissCount = diagnostic.parentCacheMissCount
	convergence.ParentRefreshCount = diagnostic.parentRefreshCount
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

func ptrPipelineIngestScanSummary(result *ScanResult) *PipelineIngestScanResult {
	summary := pipelineIngestScanSummary(result)
	return &summary
}
