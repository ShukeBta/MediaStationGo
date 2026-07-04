package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const bytesPerGiB = 1024 * 1024 * 1024

func matchesSubscriptionTorrentRules(sub *model.Subscription, item SearchResult) bool {
	if sub == nil {
		return true
	}
	if sub.MinSeeders > 0 && item.Seeders < sub.MinSeeders {
		return false
	}
	if sub.MaxSeeders > 0 && item.Seeders > sub.MaxSeeders {
		return false
	}
	if !subscriptionSizeInRange(item.Size, sub.MinSizeGB, sub.MaxSizeGB) {
		return false
	}
	if sub.FreeOnly && !subscriptionResultIsFree(item) {
		return false
	}
	return true
}

func subscriptionSizeInRange(sizeBytes int64, minGB, maxGB float64) bool {
	if minGB <= 0 && maxGB <= 0 {
		return true
	}
	if sizeBytes <= 0 {
		return false
	}
	sizeGB := float64(sizeBytes) / bytesPerGiB
	if minGB > 0 && sizeGB < minGB {
		return false
	}
	if maxGB > 0 && sizeGB > maxGB {
		return false
	}
	return true
}

func subscriptionResultIsFree(item SearchResult) bool {
	if item.Free {
		return true
	}
	text := strings.ToLower(subscriptionSearchResultText(item))
	return strings.Contains(text, "freeleech") ||
		strings.Contains(text, "2xfree") ||
		strings.Contains(text, "免费") ||
		matchesWordBoundary(text, "free")
}
