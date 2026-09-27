package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type MediaProbeBatchResult struct {
	Total  int
	Probed int
	Failed int
	Errors []string
}

func (s *StreamService) ProbeMissingMedia(
	ctx context.Context,
	probe mediaTrackProber,
	maxConcurrent int,
	onProgress func(MediaProbeBatchResult),
) (MediaProbeBatchResult, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return MediaProbeBatchResult{}, fmt.Errorf("media repository unavailable")
	}
	if probe == nil {
		return MediaProbeBatchResult{}, fmt.Errorf("ffprobe service unavailable")
	}

	var all []model.Media
	if err := s.repo.DB.WithContext(ctx).Find(&all).Error; err != nil {
		return MediaProbeBatchResult{}, err
	}
	pending := make([]model.Media, 0, len(all))
	for _, media := range all {
		if mediaTrackMetadataMissing(&media) {
			pending = append(pending, media)
		}
	}

	result := MediaProbeBatchResult{Total: len(pending)}
	if onProgress != nil {
		onProgress(result)
	}
	if len(pending) == 0 {
		return result, nil
	}

	workers := normalizeFFprobeMaxConcurrent(maxConcurrent)
	if workers > len(pending) {
		workers = len(pending)
	}
	jobs := make(chan model.Media)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for media := range jobs {
				err := s.Probe(ctx, media.ID, probe)
				mu.Lock()
				if err != nil {
					result.Failed++
					result.Errors = append(result.Errors, fmt.Sprintf("%s (%s): %v", media.Title, media.ID, err))
				} else {
					result.Probed++
				}
				if onProgress != nil {
					onProgress(cloneMediaProbeBatchResult(result))
				}
				mu.Unlock()
			}
		}()
	}
	for _, media := range pending {
		select {
		case jobs <- media:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return cloneMediaProbeBatchResult(result), ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return cloneMediaProbeBatchResult(result), nil
}

func cloneMediaProbeBatchResult(result MediaProbeBatchResult) MediaProbeBatchResult {
	result.Errors = append([]string(nil), result.Errors...)
	return result
}
