package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"sort"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const (
	weeklyFeaturedMinRating = 7.5
	weeklyFeaturedPoolSize  = 20
)

type weeklyFeaturedCandidate struct {
	card   SeriesCard
	rating float64
	order  uint64
}

// WeeklyFeaturedCard returns one high-rated, non-adult work from the media
// visible to the current user. The candidate order is deterministic and the
// selected position advances once per calendar week, so refreshes stay stable
// while consecutive weeks rotate when more than one candidate is available.
func (s *MediaService) WeeklyFeaturedCard(
	ctx context.Context,
	now time.Time,
	visibility MediaVisibility,
) (*SeriesCard, string, error) {
	visibility.IncludeNSFW = false
	items, err := s.SearchMediaVisible(ctx, "", maxMediaSearchLimit, visibility)
	if err != nil {
		return nil, "", err
	}
	items, err = s.weeklyFeaturedSafeItems(ctx, items)
	if err != nil {
		return nil, "", err
	}
	candidates := weeklyFeaturedCandidates(items)
	weekKey := weeklyFeaturedWeekKey(now)
	if len(candidates) == 0 {
		return nil, weekKey, nil
	}
	selected := candidates[weeklyFeaturedWeekIndex(now, len(candidates))].card
	return &selected, weekKey, nil
}

func (s *MediaService) weeklyFeaturedSafeItems(ctx context.Context, items []model.Media) ([]model.Media, error) {
	if len(items) == 0 {
		return nil, nil
	}
	libraryIDs := make([]string, 0)
	seen := make(map[string]struct{})
	for i := range items {
		if items[i].LibraryID == "" || items[i].NSFW {
			continue
		}
		if _, ok := seen[items[i].LibraryID]; ok {
			continue
		}
		seen[items[i].LibraryID] = struct{}{}
		libraryIDs = append(libraryIDs, items[i].LibraryID)
	}
	var libraries []model.Library
	if len(libraryIDs) > 0 {
		if err := s.repo.DB.WithContext(ctx).Where("id IN ?", libraryIDs).Find(&libraries).Error; err != nil {
			return nil, err
		}
	}
	safeLibraries := make(map[string]struct{}, len(libraries))
	for i := range libraries {
		if libraries[i].Enabled && !LibraryIsAdult(libraries[i]) {
			safeLibraries[libraries[i].ID] = struct{}{}
		}
	}
	safe := make([]model.Media, 0, len(items))
	for i := range items {
		if items[i].NSFW {
			continue
		}
		if _, ok := safeLibraries[items[i].LibraryID]; ok {
			safe = append(safe, items[i])
		}
	}
	return safe, nil
}

func weeklyFeaturedCandidates(items []model.Media) []weeklyFeaturedCandidate {
	if len(items) == 0 {
		return nil
	}
	type ratingSummary struct {
		total float64
		count int
	}
	ratings := make(map[string]ratingSummary)
	for i := range items {
		if items[i].Rating <= 0 {
			continue
		}
		key := mediaSeriesKey(items[i])
		if key == "" {
			continue
		}
		summary := ratings[key]
		summary.total += float64(items[i].Rating)
		summary.count++
		ratings[key] = summary
	}

	cards := groupMediaSeriesCardsWithOrder(items, false)
	candidates := make([]weeklyFeaturedCandidate, 0, len(cards))
	for i := range cards {
		summary := ratings[cards[i].Key]
		if summary.count == 0 {
			continue
		}
		rating := summary.total / float64(summary.count)
		if rating < weeklyFeaturedMinRating {
			continue
		}
		candidates = append(candidates, weeklyFeaturedCandidate{
			card:   cards[i],
			rating: rating,
			order:  weeklyFeaturedOrder(cards[i].Key),
		})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].rating != candidates[j].rating {
			return candidates[i].rating > candidates[j].rating
		}
		return candidates[i].card.Key < candidates[j].card.Key
	})
	if len(candidates) > weeklyFeaturedPoolSize {
		candidates = candidates[:weeklyFeaturedPoolSize]
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].order != candidates[j].order {
			return candidates[i].order < candidates[j].order
		}
		return candidates[i].card.Key < candidates[j].card.Key
	})
	return candidates
}

func weeklyFeaturedOrder(key string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("mediastation-weekly-featured:" + key))
	return h.Sum64()
}

func weeklyFeaturedWeekKey(now time.Time) string {
	year, week := now.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func weeklyFeaturedWeekIndex(now time.Time, count int) int {
	if count <= 1 {
		return 0
	}
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(date.Weekday()) + 6) % 7
	monday := date.AddDate(0, 0, -offset)
	weekOrdinal := monday.Unix() / int64(7*24*time.Hour/time.Second)
	if weekOrdinal < 0 {
		weekOrdinal = -weekOrdinal
	}
	return int(weekOrdinal % int64(count))
}
