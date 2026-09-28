package service

import "errors"

var ErrInvalidPlaybackProgress = errors.New("invalid playback progress")

// Unknown runtimes remain accepted for live/remote streams. Known runtimes
// guard malformed client positions before applying the recording threshold.
func validatePlaybackProgress(position, duration int64) error {
	if position < 0 || duration < 0 || (duration > 0 && position > duration) {
		return ErrInvalidPlaybackProgress
	}
	return nil
}

func shouldRecordPlaybackProgress(position, duration int64) bool {
	if duration > 10*60*1000 {
		return position >= 60_000
	}
	return position >= 20_000
}

func playbackCompleted(position, duration int64) bool {
	if duration <= 0 {
		return false
	}
	if duration < 10*60*1000 {
		return position >= max(int64(0), duration-30_000)
	}
	return position >= duration-duration/10
}
