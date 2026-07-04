package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *SubscriptionService) classifySubscriptionItem(ctx context.Context, sub *model.Subscription, title, sourceCategory string) (string, string) {
	mediaType := normalizeMediaType(sub.MediaType, title+" "+sub.Name+" "+sub.Filter, sourceCategory)
	category := strings.TrimSpace(sub.MediaCategory)
	if category == "" {
		if match := s.lookupSubscriptionMetadata(ctx, mediaType, title, sub); match != nil {
			category = classifyMediaCategory(mediaClassifyInput{
				MediaType: mediaType,
				Title:     match.Title + " " + match.OriginalName,
				Languages: match.Languages,
				Countries: match.Countries,
				Genres:    match.Genres,
				Category:  sourceCategory,
			}, s.categoryMap())
			if s != nil && s.log != nil && category != "" {
				s.log.Info("subscription metadata classified",
					zap.String("title", title),
					zap.String("matched_title", match.Title),
					zap.String("media_type", mediaType),
					zap.String("media_category", category),
					zap.Int("tmdb_id", match.TMDbID),
					zap.Int("bangumi_id", match.BangumiID),
					zap.String("douban_id", match.DoubanID),
					zap.String("thetvdb_id", match.TheTVDBID))
			}
		}
	}
	if category == "" {
		category = classifyMediaCategory(mediaClassifyInput{
			MediaType: mediaType,
			Title:     title + " " + sub.Name + " " + sub.Filter,
			Category:  sourceCategory,
		}, s.categoryMap())
	}
	return mediaType, category
}

func (s *SubscriptionService) lookupSubscriptionMetadata(ctx context.Context, mediaType, title string, sub *model.Subscription) *Match {
	if s == nil || s.scraper == nil || !s.scraper.AnyEnabled() {
		return nil
	}
	queries := subscriptionMetadataQueries(title, sub)
	if len(queries) == 0 {
		return nil
	}
	for _, libType := range subscriptionMetadataLibraryTypes(mediaType, title) {
		lib := &model.Library{Type: libType, Enabled: true}
		for _, query := range queries {
			cleaned, year := CleanQueryWithRecognition(ctx, s.repo, query)
			if cleaned == "" {
				cleaned = strings.TrimSpace(query)
			}
			for _, candidate := range titleCandidates(cleaned) {
				if candidate == "" {
					continue
				}
				match := s.scraper.lookup(ctx, lib, nil, candidate, year)
				if match == nil || strings.TrimSpace(match.Title) == "" {
					continue
				}
				if !organizeMetadataMatchTrusted(candidate, year, match) {
					continue
				}
				return match
			}
		}
	}
	return nil
}

func subscriptionMetadataQueries(title string, sub *model.Subscription) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	add(title)
	if sub != nil {
		add(sub.Filter)
		add(sub.Name)
	}
	return out
}

func subscriptionMetadataLibraryTypes(mediaType, title string) []string {
	if strings.TrimSpace(mediaType) == "" {
		text := strings.ToLower(title)
		switch {
		case classifierEpisodeRE.MatchString(text) || classifierSeasonRE.MatchString(text):
			return []string{"tv", "anime", "movie"}
		case containsAnyText(text, "动漫", "动画", "anime", "bangumi"):
			return []string{"anime", "tv", "movie"}
		case containsAnyText(text, "电影", "movie", "film"):
			return []string{"movie", "tv", "anime"}
		default:
			return []string{"tv", "movie", "anime"}
		}
	}
	switch normalizeMediaType(mediaType, title, "") {
	case "movie":
		return []string{"movie"}
	case "anime":
		return []string{"anime", "tv"}
	case "tv", "variety":
		return []string{"tv", "anime"}
	default:
		if classifierEpisodeRE.MatchString(title) || classifierSeasonRE.MatchString(title) {
			return []string{"tv", "anime"}
		}
		return []string{"movie", "tv", "anime"}
	}
}

func (s *SubscriptionService) categoryMap() map[string]string {
	if s == nil || s.cfg == nil || s.cfg.Organizer.Categories == nil {
		return nil
	}
	return s.cfg.Organizer.Categories
}

func (s *SubscriptionService) resolveSubscriptionSavePath(ctx context.Context, sub *model.Subscription, mediaType, category string) string {
	if sub == nil {
		return ""
	}
	base := strings.TrimSpace(sub.SavePath)
	if base == "" {
		base = downloadDefaultSaveRoot(ctx, s.repo)
	}
	if base == "" {
		return ""
	}
	if !s.isSmartClassifyEnabled(ctx) || category == "" {
		return base
	}
	return downloadSavePathCategoryRoot(base, sanitizeFilename(category))
}

func (s *SubscriptionService) isSmartClassifyEnabled(ctx context.Context) bool {
	if s != nil && s.repo != nil && s.repo.Setting != nil {
		val, err := s.repo.Setting.Get(ctx, DownloadSmartClassifySettingKey)
		if err == nil && val != "" {
			return parseBoolSetting(val, true)
		}
		val, err = s.repo.Setting.Get(ctx, "organizer.smart_classify")
		if err == nil && parseBoolSetting(val, false) {
			return true
		}
	}
	if s != nil && s.cfg != nil && s.cfg.Organizer.SmartClassify {
		return true
	}
	return true
}
