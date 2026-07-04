package service

import (
	"context"
	"os"
	"time"

	"go.uber.org/zap"
)

// WaitReady blocks (with a deadline) until the playlist file shows up on
// disk. Returns true on success.
func (t *TranscoderService) WaitReady(ctx context.Context, mediaID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(t.PlaylistPath(mediaID)); err == nil {
			t.mu.Lock()
			if j, ok := t.jobs[mediaID]; ok {
				j.playlistOK = true
			}
			t.mu.Unlock()
			return true
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// StopJob cancels a running ffmpeg process for mediaID, if any.
func (t *TranscoderService) StopJob(mediaID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j, ok := t.jobs[mediaID]; ok {
		j.cancel()
		delete(t.jobs, mediaID)
	}
}

// TouchJob records client activity for the HLS playlist or segment. The idle
// watchdog uses it to stop ffmpeg soon after the player is closed or switches
// back to direct play.
func (t *TranscoderService) TouchJob(mediaID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.touchJobLocked(mediaID)
}

func (t *TranscoderService) touchJobLocked(mediaID string) {
	if j, ok := t.jobs[mediaID]; ok {
		j.lastAccess = time.Now()
	}
}

// StopAll terminates every running transcode (called on graceful shutdown).
func (t *TranscoderService) StopAll() {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, j := range t.jobs {
		j.cancel()
		delete(t.jobs, id)
	}
}

// ActiveJob is the JSON shape exposed to the React Tasks panel.
type ActiveJob struct {
	MediaID    string    `json:"media_id"`
	Encoder    string    `json:"encoder"`
	StartedAt  time.Time `json:"started_at"`
	PlaylistOK bool      `json:"playlist_ok"`
}

// Active returns a snapshot of the currently running transcode jobs.
func (t *TranscoderService) Active() []ActiveJob {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]ActiveJob, 0, len(t.jobs))
	for _, j := range t.jobs {
		out = append(out, ActiveJob{
			MediaID:    j.mediaID,
			Encoder:    j.encoder,
			StartedAt:  j.startedAt,
			PlaylistOK: j.playlistOK,
		})
	}
	return out
}

func (t *TranscoderService) maxConcurrent() int {
	if t.cfg.Transcoder.MaxConcurrent <= 0 {
		return 1
	}
	return t.cfg.Transcoder.MaxConcurrent
}

func (t *TranscoderService) idleTimeout() time.Duration {
	if t.cfg.Transcoder.IdleTimeoutSeconds <= 0 {
		return 120 * time.Second
	}
	return time.Duration(t.cfg.Transcoder.IdleTimeoutSeconds) * time.Second
}

func (t *TranscoderService) monitorIdle(ctx context.Context, job *hlsJob) {
	timeout := t.idleTimeout()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.mu.Lock()
			current, ok := t.jobs[job.mediaID]
			if !ok {
				t.mu.Unlock()
				return
			}
			idleFor := time.Since(current.lastAccess)
			t.mu.Unlock()
			if idleFor >= timeout {
				t.log.Info("transcode idle timeout",
					zap.String("media_id", job.mediaID),
					zap.Duration("idle_for", idleFor),
					zap.Duration("timeout", timeout),
				)
				t.StopJob(job.mediaID)
				return
			}
		}
	}
}
