package service

import "math"

func effectiveMediaBitRate(storedBitRate, sizeBytes int64, durationSec int) int64 {
	if storedBitRate > 0 {
		return storedBitRate
	}
	if sizeBytes <= 0 || durationSec <= 0 || sizeBytes > math.MaxInt64/8 {
		return 0
	}
	return sizeBytes * 8 / int64(durationSec)
}
