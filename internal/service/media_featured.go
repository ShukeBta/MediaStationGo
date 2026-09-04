package service

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const (
	weeklyFeaturedMinRating     = 7.5
	weeklyFeaturedPoolSize      = 20
	weeklyFeaturedCooldownWeeks = 8
)

type weeklyFeaturedCandidate struct {
	card   SeriesCard
	rating float64
	order  uint64
}

// WeeklyFeaturedCard returns one high-rated, non-adult work from the media
// visible to the current user. Each user's weekly choice is persisted, and a
// work is held back for the following eight recommendation weeks whenever the
// permission-scoped candidate pool is large enough.
func (s *MediaService) WeeklyFeaturedCard(
	ctx context.Context,
	userID string,
	now time.Time,
	visibility MediaVisibility,
) (*SeriesCard, string, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, "", errors.New("weekly featured user id is required")
	}
	visibility.IncludeNSFW = false
	ctx, items, err := s.listVisibleSeriesCardCandidates(ctx, visibility)
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
	selected, err := s.selectWeeklyFeaturedCandidate(ctx, userID, now, weekKey, candidates)
	if err != nil {
		return nil, "", err
	}
	hydrated, err := s.hydrateSeriesCards(ctx, []SeriesCard{selected})
	if err != nil {
		return nil, "", err
	}
	return &hydrated[0], weekKey, nil
}

func (s *MediaService) selectWeeklyFeaturedCandidate(
	ctx context.Context,
	userID string,
	now time.Time,
	weekKey string,
	candidates []weeklyFeaturedCandidate,
) (SeriesCard, error) {
	var current model.WeeklyFeaturedSelection
	err := s.repo.DB.WithContext(ctx).
		Where("user_id = ? AND week_key = ?", userID, weekKey).
		First(&current).Error
	if err == nil {
		for i := range candidates {
			if candidates[i].card.Key == current.WorkKey {
				return candidates[i].card, nil
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return SeriesCard{}, err
	}

	weekStart := weeklyFeaturedWeekStart(now)
	cooldownStart := weekStart.AddDate(0, 0, -7*weeklyFeaturedCooldownWeeks)
	var recent []model.WeeklyFeaturedSelection
	if err := s.repo.DB.WithContext(ctx).
		Where("user_id = ? AND week_start < ? AND week_start >= ?", userID, weekStart, cooldownStart).
		Order("week_start DESC").
		Find(&recent).Error; err != nil {
		return SeriesCard{}, err
	}
	lastFeatured := make(map[string]time.Time, len(recent))
	for i := range recent {
		if seen, ok := lastFeatured[recent[i].WorkKey]; !ok || recent[i].WeekStart.After(seen) {
			lastFeatured[recent[i].WorkKey] = recent[i].WeekStart
		}
	}
	available := make([]weeklyFeaturedCandidate, 0, len(candidates))
	for i := range candidates {
		if _, coolingDown := lastFeatured[candidates[i].card.Key]; !coolingDown {
			available = append(available, candidates[i])
		}
	}
	var selected weeklyFeaturedCandidate
	if len(available) > 0 {
		selected = available[weeklyFeaturedWeekIndex(now, len(available))]
	} else {
		// A small permission-scoped library may have fewer works than the
		// cooldown window. Relax explicitly by choosing the least recently
		// featured work instead of returning an empty home-page hero.
		selected = candidates[0]
		for i := 1; i < len(candidates); i++ {
			selectedAt := lastFeatured[selected.card.Key]
			candidateAt := lastFeatured[candidates[i].card.Key]
			if candidateAt.Before(selectedAt) || (candidateAt.Equal(selectedAt) && candidates[i].order < selected.order) {
				selected = candidates[i]
			}
		}
	}

	record := model.WeeklyFeaturedSelection{
		UserID:    userID,
		WeekKey:   weekKey,
		WeekStart: weekStart,
		WorkKey:   selected.card.Key,
		MediaID:   selected.card.Rep.ID,
	}
	if err := s.repo.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "week_key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"week_start": weekStart,
			"work_key":   selected.card.Key,
			"media_id":   selected.card.Rep.ID,
			"updated_at": now,
		}),
	}).Create(&record).Error; err != nil {
		return SeriesCard{}, err
	}
	return selected.card, nil
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
	monday := weeklyFeaturedWeekStart(now)
	weekOrdinal := monday.Unix() / int64(7*24*time.Hour/time.Second)
	if weekOrdinal < 0 {
		weekOrdinal = -weekOrdinal
	}
	return int(weekOrdinal % int64(count))
}

func weeklyFeaturedWeekStart(now time.Time) time.Time {
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(date.Weekday()) + 6) % 7
	return date.AddDate(0, 0, -offset)
}
