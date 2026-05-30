// Package service — RSS subscriptions for automated downloads.
//
// SubscriptionService periodically polls every Subscription row, fetches
// the configured RSS / Atom feed, and queues new items into the
// DownloadService. Items are deduplicated by GUID stored as a Setting key
// "subscription.<id>.last_guid" so the same episode is never re-queued.
package service

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var (
	seriesPackRE = regexp.MustCompile(`(?i)(complete|batch|合集|全集|全\s*\d+\s*[集话話期]|整季|全季|s\d{1,2}\s*(?:complete|batch|pack)|season\s*\d{1,2}\s*(?:complete|batch|pack)|s\d{1,2}e\d{1,3}\s*[-~–—]\s*(?:e)?\d{1,3}|第\s*\d+\s*[-~–—]\s*\d+\s*[集话話期])`)
	seasonOnlyRE = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:s|season)\s*\d{1,2}(?:[\s._-]|$)|第\s*\d+\s*季`)
)

type siteSearchCandidate struct {
	Item     SearchResult
	Download string
	GUID     string
	Season   int
	Episode  int
	Pack     bool
	Score    int
}

// SubscriptionService runs the polling loop.
type SubscriptionService struct {
	cfg       *config.Config
	log       *zap.Logger
	repo      *repository.Container
	downloads *DownloadService
	site      *SiteService
	hub       *Hub
	stop      chan struct{}
}

// NewSubscriptionService is the constructor.
func NewSubscriptionService(cfg *config.Config, log *zap.Logger, repo *repository.Container, downloads *DownloadService, site *SiteService, hub *Hub) *SubscriptionService {
	return &SubscriptionService{
		cfg:       cfg,
		log:       log,
		repo:      repo,
		downloads: downloads,
		site:      site,
		hub:       hub,
		stop:      make(chan struct{}),
	}
}

// Start runs the polling loop in the background.
func (s *SubscriptionService) Start(ctx context.Context) {
	go s.loop(ctx)
}

// Stop shuts the loop down.
func (s *SubscriptionService) Stop() { close(s.stop) }

// rssFeed is the minimal RSS subset we need to decode.
type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Items []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			GUID        string `xml:"guid"`
			Description string `xml:"description"`
			Enclosure   struct {
				URL string `xml:"url,attr"`
			} `xml:"enclosure"`
		} `xml:"item"`
	} `xml:"channel"`
}

// Create persists a new subscription.
func (s *SubscriptionService) Create(ctx context.Context, sub *model.Subscription) error {
	if sub.Name == "" || sub.FeedURL == "" {
		return errors.New("name and feed_url required")
	}
	normalizeSubscriptionDefaults(sub)
	enabled := sub.Enabled
	if err := s.repo.Subscription.Create(ctx, sub); err != nil {
		return err
	}
	if !enabled {
		if err := s.repo.DB.WithContext(ctx).Model(sub).Update("enabled", false).Error; err != nil {
			return err
		}
		sub.Enabled = false
	}
	return nil
}

func normalizeSubscriptionDefaults(sub *model.Subscription) {
	if strings.TrimSpace(sub.SearchMode) == "" {
		sub.SearchMode = "keyword"
	}
	if strings.TrimSpace(sub.Resolution) == "" {
		sub.Resolution = "best"
	}
	if strings.TrimSpace(sub.WashPriority) == "" {
		sub.WashPriority = "balanced"
	}
	if sub.Priority == 0 {
		sub.Priority = 50
	}
}

// List returns every subscription rule.
func (s *SubscriptionService) List(ctx context.Context) ([]model.Subscription, error) {
	return s.repo.Subscription.List(ctx)
}

// Delete removes a subscription.
func (s *SubscriptionService) Delete(ctx context.Context, id string) error {
	return s.repo.DB.Where("id = ?", id).Delete(&model.Subscription{}).Error
}

// RunNow forces a poll for one subscription, ignoring its schedule. Used
// by the admin UI's "test now" button.
func (s *SubscriptionService) RunNow(ctx context.Context, id string) (int, error) {
	var sub model.Subscription
	if err := s.repo.DB.Where("id = ?", id).First(&sub).Error; err != nil {
		return 0, err
	}
	return s.runOne(ctx, &sub)
}

// loop polls every 10 minutes.
func (s *SubscriptionService) loop(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	// First run shortly after startup.
	first := time.NewTimer(30 * time.Second)
	defer first.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stop:
			return
		case <-first.C:
		case <-t.C:
		}
		s.runAll(ctx)
	}
}

func (s *SubscriptionService) runAll(ctx context.Context) {
	subs, err := s.repo.Subscription.List(ctx)
	if err != nil {
		s.log.Warn("subscription list failed", zap.Error(err))
		return
	}
	for i := range subs {
		if !subs[i].Enabled {
			continue
		}
		if n, err := s.runOne(ctx, &subs[i]); err != nil {
			s.log.Warn("subscription run failed",
				zap.String("name", subs[i].Name), zap.Error(err))
		} else if n > 0 {
			s.log.Info("subscription queued items",
				zap.String("name", subs[i].Name), zap.Int("count", n))
		}
	}
}

func (s *SubscriptionService) runOne(ctx context.Context, sub *model.Subscription) (int, error) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(sub.FeedURL)), "site-search://") {
		return s.runSiteSearch(ctx, sub)
	}

	feed, err := s.fetch(ctx, sub.FeedURL)
	if err != nil {
		return 0, err
	}

	filter := compileFilter(sub.Filter)
	guidKey := fmt.Sprintf("subscription.%s.seen", sub.ID)
	seenRaw, _ := s.repo.Setting.Get(ctx, guidKey)
	seen := splitNonEmpty(seenRaw)
	seenSet := make(map[string]struct{}, len(seen))
	for _, g := range seen {
		seenSet[g] = struct{}{}
	}

	// 非洗版订阅：成功下载一次即满足，预先算一次媒体库可用性用于跳过已入库的电影/剧集（对齐 MoviePilot）。
	washOff := !sub.WashEnabled
	var avail LocalAvailability
	if washOff {
		avail = SubscriptionLocalAvailability(ctx, s.repo, sub)
	}

	queued := 0
	for _, item := range feed.Channel.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		if _, ok := seenSet[guid]; ok {
			continue
		}
		if filter != nil && !filter.MatchString(item.Title) {
			continue
		}
		// 应用订阅高级规则（排除词/分辨率/质量/特效/发布组）—— 此前 RSS 路径完全跳过，导致排除不生效。
		if !matchesSubscriptionRules(sub, item.Title) {
			continue
		}
		if washOff && subscriptionItemAlreadyAvailable(sub, avail, item.Title) {
			seen = append(seen, guid)
			seenSet[guid] = struct{}{}
			continue
		}
		download := item.Enclosure.URL
		if download == "" {
			download = item.Link
		}
		if download == "" {
			continue
		}
		mediaType, mediaCategory := s.classifySubscriptionItem(ctx, sub, item.Title, "")
		savePath := s.resolveSubscriptionSavePath(ctx, sub, mediaType, mediaCategory)
		if s.downloadPathHasCandidate(ctx, sub, item.Title, savePath) {
			seen = append(seen, guid)
			seenSet[guid] = struct{}{}
			continue
		}
		if _, err := s.downloads.AddDownloadWithMeta(ctx, sub.UserID, download, savePath, DownloadTaskMeta{
			Title:       firstNonEmpty(item.Title, sub.Name),
			PosterURL:   sub.PosterURL,
			BackdropURL: sub.BackdropURL,
			Overview:    sub.Overview,
		}); err != nil {
			s.log.Warn("subscription enqueue failed",
				zap.String("title", item.Title),
				zap.String("media_type", mediaType),
				zap.String("media_category", mediaCategory),
				zap.String("save_path", savePath),
				zap.Error(err))
			continue
		}
		queued++
		seen = append(seen, guid)
	}
	// Remember the last 200 GUIDs so the seen set doesn't grow forever.
	if len(seen) > 200 {
		seen = seen[len(seen)-200:]
	}
	_ = s.repo.Setting.Set(ctx, guidKey, strings.Join(seen, "\n"))

	now := time.Now()
	_ = s.repo.DB.Model(sub).Updates(map[string]any{"last_run_at": &now}).Error
	if queued > 0 {
		s.hub.Publish("subscription", map[string]any{
			"id":     sub.ID,
			"name":   sub.Name,
			"queued": queued,
		})
	}
	return queued, nil
}

func (s *SubscriptionService) runSiteSearch(ctx context.Context, sub *model.Subscription) (int, error) {
	if s.site == nil {
		return 0, errors.New("site search service unavailable")
	}
	keyword := siteSearchKeyword(sub)
	if keyword == "" {
		return 0, errors.New("site-search subscription keyword required")
	}

	results, err := s.site.Search(ctx, keyword)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		now := time.Now()
		_ = s.repo.DB.Model(sub).Updates(map[string]any{"last_run_at": &now}).Error
		return 0, nil
	}

	guidKey := fmt.Sprintf("subscription.%s.seen", sub.ID)
	seenRaw, _ := s.repo.Setting.Get(ctx, guidKey)
	seen := splitNonEmpty(seenRaw)
	seenSet := make(map[string]struct{}, len(seen))
	for _, g := range seen {
		seenSet[g] = struct{}{}
	}

	availability := mergeLocalAvailability(
		SubscriptionLocalAvailability(ctx, s.repo, sub),
		s.pendingDownloadAvailability(ctx, sub),
	)
	candidates := selectSiteSearchCandidates(results, sub, seenSet, availability)
	var lastEnqueueErr error
	queued := 0
	var resources []string
	for _, candidate := range candidates {
		item := candidate.Item
		mediaType, mediaCategory := s.classifySubscriptionItem(ctx, sub, item.Title, item.Category)
		if s.shouldSkipExistingTorrent(ctx, mediaType, candidate) {
			seen = append(seen, candidate.GUID)
			seenSet[candidate.GUID] = struct{}{}
			continue
		}
		realURL := s.site.ResolveDownloadURL(ctx, candidate.Download)
		savePath := s.resolveSubscriptionSavePath(ctx, sub, mediaType, mediaCategory)
		if s.downloadPathHasCandidate(ctx, sub, candidate.Item.Title, savePath) {
			seen = append(seen, candidate.GUID)
			seenSet[candidate.GUID] = struct{}{}
			continue
		}
		if _, err := s.downloads.AddDownloadWithMeta(ctx, sub.UserID, realURL, savePath, DownloadTaskMeta{
			Title:       firstNonEmpty(item.Title, sub.Name),
			PosterURL:   sub.PosterURL,
			BackdropURL: sub.BackdropURL,
			Overview:    sub.Overview,
		}); err != nil {
			lastEnqueueErr = err
			s.log.Warn("site-search subscription enqueue failed",
				zap.String("subscription", sub.Name),
				zap.String("title", item.Title),
				zap.String("site_category", item.Category),
				zap.String("media_type", mediaType),
				zap.String("media_category", mediaCategory),
				zap.String("save_path", savePath),
				zap.Error(err))
			continue
		}
		queued++
		resources = append(resources, item.Title)
		seen = append(seen, candidate.GUID)
		seenSet[candidate.GUID] = struct{}{}
	}
	if len(seen) > 200 {
		seen = seen[len(seen)-200:]
	}
	_ = s.repo.Setting.Set(ctx, guidKey, strings.Join(seen, "\n"))
	now := time.Now()
	_ = s.repo.DB.Model(sub).Updates(map[string]any{"last_run_at": &now}).Error
	if queued > 0 {
		s.hub.Publish("subscription", map[string]any{
			"id":        sub.ID,
			"name":      sub.Name,
			"queued":    queued,
			"keyword":   keyword,
			"resources": resources,
		})
		return queued, nil
	}
	if lastEnqueueErr != nil {
		return 0, fmt.Errorf("找到 PT 资源但加入下载器失败: %w", lastEnqueueErr)
	}
	return 0, nil
}

func selectSiteSearchCandidates(results []SearchResult, sub *model.Subscription, seenSet map[string]struct{}, availability ...LocalAvailability) []siteSearchCandidate {
	candidates := make([]siteSearchCandidate, 0, len(results))
	for _, item := range results {
		if !matchesSubscriptionRules(sub, item.Title) {
			continue
		}
		download := strings.TrimSpace(item.DownloadURL)
		if download == "" {
			download = strings.TrimSpace(item.TorrentURL)
		}
		if download == "" {
			continue
		}
		guid := download
		if _, ok := seenSet[guid]; ok {
			continue
		}
		season, episode := ParseEpisode(item.Title)
		score := subscriptionCandidateScore(sub, item)
		candidates = append(candidates, siteSearchCandidate{
			Item:     item,
			Download: download,
			GUID:     guid,
			Season:   season,
			Episode:  episode,
			Pack:     isSeriesPackTitle(item.Title),
			Score:    score,
		})
	}
	if len(candidates) <= 1 {
		return candidates
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Item.Seeders != candidates[j].Item.Seeders {
			return candidates[i].Item.Seeders > candidates[j].Item.Seeders
		}
		return candidates[i].Item.Size > candidates[j].Item.Size
	})

	var local LocalAvailability
	if len(availability) > 0 {
		local = availability[0]
	}

	mediaType := normalizeMediaType(sub.MediaType, sub.Name+" "+sub.Filter, "")
	if !isSubscriptionSeriesType(mediaType) {
		// 对齐 MoviePilot：非洗版订阅成功下载一次即满足，媒体库/下载中已存在则不再重复下载。
		if (sub == nil || !sub.WashEnabled) && local.LocalMediaCount > 0 {
			return nil
		}
		return candidates[:1]
	}

	if local.LocalMediaCount > 0 {
		if local.TotalEpisodes > 0 && len(local.MissingEpisodes) == 0 {
			return nil
		}
		missingSet := missingEpisodeSet(local)
		onlyMissing := make([]siteSearchCandidate, 0, len(candidates))
		for _, candidate := range candidates {
			if candidate.Episode <= 0 {
				continue
			}
			season := candidate.Season
			if season <= 0 {
				season = 1
			}
			if _, exists := local.ExistingEpisodeKeys[episodeKey(season, candidate.Episode)]; exists {
				continue
			}
			if local.TotalEpisodes > 0 {
				if _, missing := missingSet[candidate.Episode]; !missing {
					continue
				}
			}
			onlyMissing = append(onlyMissing, candidate)
		}
		return sortedEpisodeCandidates(onlyMissing)
	}

	for _, candidate := range candidates {
		if candidate.Pack {
			return []siteSearchCandidate{candidate}
		}
	}

	selected := sortedEpisodeCandidates(candidates)
	if len(selected) == 0 {
		return candidates[:1]
	}
	return selected
}

// defaultExcludeWords 是参考 MoviePilot 默认过滤的「垃圾版本」排除清单，对所有订阅生效，
// 与用户自定义排除词合并。拉丁词在 containsAnyExcludeToken 里按词边界匹配以避免子串误伤。
const defaultExcludeWords = "cam,ts,tc,telesync,telecine,hdcam,hdts,枪版,抢先,抢鲜,预告,trailer,sample"

func matchesSubscriptionRules(sub *model.Subscription, title string) bool {
	titleFold := strings.ToLower(title)
	if containsAnyExcludeToken(titleFold, defaultExcludeWords) {
		return false
	}
	if sub == nil {
		return true
	}
	if sub.ExcludeWords != "" && containsAnyExcludeToken(titleFold, sub.ExcludeWords) {
		return false
	}
	if sub.ReleaseGroups != "" && !containsAnyToken(titleFold, sub.ReleaseGroups) {
		return false
	}
	if sub.Resolution != "" && sub.Resolution != "best" && !titleMatchesResolution(titleFold, sub.Resolution) {
		return false
	}
	if sub.Quality != "" && sub.Quality != "best" && !titleMatchesQuality(titleFold, sub.Quality) {
		return false
	}
	if sub.Effects != "" && !containsAnyEffect(titleFold, sub.Effects) {
		return false
	}
	return true
}

func subscriptionCandidateScore(sub *model.Subscription, item SearchResult) int {
	title := strings.ToLower(item.Title)
	score := item.Seeders
	if sub == nil || !sub.WashEnabled {
		if item.Free {
			score += 25
		}
		return score
	}
	resolutionScore := detectResolutionScore(title)
	qualityScore := detectQualityScore(title)
	effectScore := detectEffectScore(title)

	priority := "balanced"
	if sub != nil && strings.TrimSpace(sub.WashPriority) != "" {
		priority = strings.ToLower(strings.TrimSpace(sub.WashPriority))
	}
	switch priority {
	case "resolution":
		score += resolutionScore*1000 + qualityScore*100 + effectScore*50
	case "quality":
		score += qualityScore*1000 + resolutionScore*200 + effectScore*50
	case "effects":
		score += effectScore*1000 + resolutionScore*200 + qualityScore*100
	case "seeders":
		score += qualityScore*3 + resolutionScore*2 + effectScore
	default:
		score += resolutionScore*500 + qualityScore*300 + effectScore*150
	}
	if item.Free {
		score += 25
	}
	return score
}

func containsAnyToken(titleFold, csv string) bool {
	for _, token := range strings.FieldsFunc(strings.ToLower(csv), func(r rune) bool {
		return r == ',' || r == '/' || r == '|' || r == ';' || r == '，'
	}) {
		token = strings.TrimSpace(token)
		if token != "" && strings.Contains(titleFold, token) {
			return true
		}
	}
	return false
}

// containsAnyExcludeToken 用于排除词匹配：纯 ASCII 字母数字的词按词边界匹配（避免 "ts"
// 误伤 "tsukihime"、"cam" 误伤 "camp" 之类的子串误判），含 CJK/符号的词仍按子串匹配。
func containsAnyExcludeToken(titleFold, csv string) bool {
	for _, token := range strings.FieldsFunc(strings.ToLower(csv), func(r rune) bool {
		return r == ',' || r == '/' || r == '|' || r == ';' || r == '，'
	}) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if isASCIIWordToken(token) {
			if matchesWordBoundary(titleFold, token) {
				return true
			}
			continue
		}
		if strings.Contains(titleFold, token) {
			return true
		}
	}
	return false
}

func isASCIIWordToken(token string) bool {
	for _, r := range token {
		if r > unicode.MaxASCII || !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return token != ""
}

// matchesWordBoundary 判断 token 是否作为独立词出现在 title 中，词边界为「非字母数字」。
func matchesWordBoundary(titleFold, token string) bool {
	isWordRune := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	from := 0
	for {
		idx := strings.Index(titleFold[from:], token)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(token)
		leftOK := start == 0 || !isWordRune(rune(titleFold[start-1]))
		rightOK := end >= len(titleFold) || !isWordRune(rune(titleFold[end]))
		if leftOK && rightOK {
			return true
		}
		from = start + 1
		if from >= len(titleFold) {
			return false
		}
	}
}

func containsAnyEffect(titleFold, csv string) bool {
	for _, token := range strings.FieldsFunc(strings.ToLower(csv), func(r rune) bool {
		return r == ',' || r == '/' || r == '|' || r == ';' || r == '，'
	}) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		switch token {
		case "dolby-vision", "dolby vision", "dv":
			if strings.Contains(titleFold, "dolby vision") || strings.Contains(titleFold, "dovi") || regexp.MustCompile(`\bdv\b`).MatchString(titleFold) {
				return true
			}
		default:
			if strings.Contains(titleFold, token) {
				return true
			}
		}
	}
	return false
}

func titleMatchesResolution(titleFold, resolution string) bool {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "2160p", "4k", "uhd":
		return strings.Contains(titleFold, "2160p") || strings.Contains(titleFold, "4k") || strings.Contains(titleFold, "uhd")
	case "1080p":
		return strings.Contains(titleFold, "1080p") || strings.Contains(titleFold, "fhd")
	case "720p":
		return strings.Contains(titleFold, "720p")
	default:
		return strings.Contains(titleFold, strings.ToLower(strings.TrimSpace(resolution)))
	}
}

func titleMatchesQuality(titleFold, quality string) bool {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "webdl", "web-dl":
		return strings.Contains(titleFold, "web-dl") || strings.Contains(titleFold, "webdl")
	case "bluray", "blu-ray":
		return strings.Contains(titleFold, "bluray") || strings.Contains(titleFold, "blu-ray") || strings.Contains(titleFold, "bdrip")
	case "remux":
		return strings.Contains(titleFold, "remux")
	case "hdtv":
		return strings.Contains(titleFold, "hdtv")
	default:
		return strings.Contains(titleFold, strings.ToLower(strings.TrimSpace(quality)))
	}
}

func detectResolutionScore(titleFold string) int {
	switch {
	case titleMatchesResolution(titleFold, "2160p"):
		return 4
	case titleMatchesResolution(titleFold, "1080p"):
		return 3
	case titleMatchesResolution(titleFold, "720p"):
		return 2
	default:
		return 1
	}
}

func detectQualityScore(titleFold string) int {
	switch {
	case titleMatchesQuality(titleFold, "remux"):
		return 5
	case titleMatchesQuality(titleFold, "bluray"):
		return 4
	case titleMatchesQuality(titleFold, "web-dl"):
		return 3
	case titleMatchesQuality(titleFold, "hdtv"):
		return 2
	default:
		return 1
	}
}

func detectEffectScore(titleFold string) int {
	score := 0
	if containsAnyEffect(titleFold, "dolby-vision") {
		score += 4
	}
	if strings.Contains(titleFold, "hdr10+") {
		score += 3
	} else if strings.Contains(titleFold, "hdr") {
		score += 2
	}
	if strings.Contains(titleFold, "atmos") {
		score += 2
	}
	return score
}

func isSubscriptionSeriesType(mediaType string) bool {
	switch normalizeMediaType(mediaType, "", "") {
	case "tv", "anime", "variety":
		return true
	default:
		return false
	}
}

func isSeriesPackTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return false
	}
	if seriesPackRE.MatchString(title) {
		return true
	}
	_, episode := ParseEpisode(title)
	return episode == 0 && seasonOnlyRE.MatchString(title)
}

func (s *SubscriptionService) shouldSkipExistingTorrent(ctx context.Context, mediaType string, candidate siteSearchCandidate) bool {
	if s == nil || s.downloads == nil {
		return false
	}
	if isSubscriptionSeriesType(mediaType) && !candidate.Pack && candidate.Episode > 0 {
		return false
	}
	return s.downloads.TorrentExistsByName(ctx, candidate.Item.Title)
}

func (s *SubscriptionService) pendingDownloadAvailability(ctx context.Context, sub *model.Subscription) LocalAvailability {
	out := LocalAvailability{
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
	if sub != nil {
		out.TotalEpisodes = sub.TotalEpisodes
	}
	root := s.subscriptionBaseSavePath(ctx, sub)
	query := availabilityQuery(subscriptionName(sub), subscriptionFilter(sub))
	if root == "" || query == "" {
		return out
	}
	_ = scanDownloadPath(ctx, root, query, func(_ string, season, episode int) bool {
		out.LocalMediaCount++
		out.InLibrary = true
		if episode > 0 {
			out.ExistingEpisodeKeys[episodeKey(season, episode)] = struct{}{}
		}
		return true
	})
	mediaType := ""
	if sub != nil {
		mediaType = sub.MediaType
	}
	if isSubscriptionSeriesType(mediaType) || len(out.ExistingEpisodeKeys) > 0 {
		out.DownloadedEpisodes = len(out.ExistingEpisodeKeys)
		out.MissingEpisodes = missingEpisodes(out.ExistingEpisodeKeys, out.TotalEpisodes)
		for _, episode := range out.MissingEpisodes {
			out.MissingEpisodeKeys[episodeKey(1, episode)] = struct{}{}
		}
	} else if out.LocalMediaCount > 0 {
		out.DownloadedEpisodes = 1
		if out.TotalEpisodes == 0 {
			out.TotalEpisodes = 1
		}
	}
	return out
}

func (s *SubscriptionService) subscriptionBaseSavePath(ctx context.Context, sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	base := strings.TrimSpace(sub.SavePath)
	if base == "" && s != nil && s.repo != nil && s.repo.Setting != nil {
		base, _ = s.repo.Setting.Get(ctx, "qbittorrent.savepath")
	}
	return base
}

func subscriptionName(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	return sub.Name
}

func subscriptionFilter(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	return sub.Filter
}

func mergeLocalAvailability(values ...LocalAvailability) LocalAvailability {
	out := LocalAvailability{
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
	for _, value := range values {
		if out.TotalEpisodes == 0 {
			out.TotalEpisodes = value.TotalEpisodes
		}
		out.LocalMediaCount += value.LocalMediaCount
		out.InLibrary = out.InLibrary || value.InLibrary
		for key := range value.ExistingEpisodeKeys {
			out.ExistingEpisodeKeys[key] = struct{}{}
		}
	}
	out.DownloadedEpisodes = len(out.ExistingEpisodeKeys)
	if out.TotalEpisodes > 0 {
		out.MissingEpisodes = missingEpisodes(out.ExistingEpisodeKeys, out.TotalEpisodes)
		for _, episode := range out.MissingEpisodes {
			out.MissingEpisodeKeys[episodeKey(1, episode)] = struct{}{}
		}
	}
	if out.DownloadedEpisodes == 0 && out.LocalMediaCount > 0 {
		out.DownloadedEpisodes = out.LocalMediaCount
		if out.TotalEpisodes == 0 {
			out.TotalEpisodes = 1
		}
	}
	return out
}

// subscriptionItemAlreadyAvailable 判断某个订阅条目（按其标题解析出的季/集）是否已在媒体库存在。
// 电影/无集号条目：媒体库已有该片即视为已存在；剧集条目：对应季集已入库即视为已存在。
func subscriptionItemAlreadyAvailable(sub *model.Subscription, avail LocalAvailability, title string) bool {
	if avail.LocalMediaCount == 0 {
		return false
	}
	if !isSubscriptionSeriesType(subscriptionMediaType(sub)) {
		return true
	}
	wantSeason, wantEpisode := ParseEpisode(title)
	if wantEpisode <= 0 {
		// 整季合集 / 无法解析集号：库里已有内容时保守跳过，避免重复整季下载。
		return true
	}
	if wantSeason <= 0 {
		wantSeason = 1
	}
	_, exists := avail.ExistingEpisodeKeys[episodeKey(wantSeason, wantEpisode)]
	return exists
}

func subscriptionMediaType(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	return sub.MediaType
}

func (s *SubscriptionService) downloadPathHasCandidate(ctx context.Context, sub *model.Subscription, title, savePath string) bool {
	savePath = strings.TrimSpace(savePath)
	if savePath == "" {
		savePath = s.subscriptionBaseSavePath(ctx, sub)
	}
	query := availabilityQuery(title, subscriptionFilter(sub))
	if savePath == "" || query == "" {
		return false
	}
	wantSeason, wantEpisode := ParseEpisode(title)
	if wantSeason <= 0 {
		wantSeason = 1
	}
	found := false
	_ = scanDownloadPath(ctx, savePath, query, func(path string, season, episode int) bool {
		if wantEpisode <= 0 {
			found = true
			return false
		}
		if episode <= 0 {
			return true
		}
		if season <= 0 {
			season = 1
		}
		if episodeKey(season, episode) == episodeKey(wantSeason, wantEpisode) {
			found = true
			return false
		}
		return true
	})
	return found
}

func scanDownloadPath(ctx context.Context, root, query string, visit func(path string, season, episode int) bool) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	normalizedQuery := normalizeAvailabilityComparable(query)
	if normalizedQuery == "" {
		return nil
	}
	visited := 0
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(filepath.Base(path), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isDownloadMediaPath(path) {
			return nil
		}
		visited++
		if visited > 10000 {
			return filepath.SkipAll
		}
		if !strings.Contains(normalizeAvailabilityComparable(path), normalizedQuery) {
			return nil
		}
		season, episode := ParseEpisode(path)
		if !visit(path, season, episode) {
			return filepath.SkipAll
		}
		return nil
	})
}

func isDownloadMediaPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".!qb", ".part", ".aria2", ".crdownload":
		path = strings.TrimSuffix(path, filepath.Ext(path))
		ext = strings.ToLower(filepath.Ext(path))
	}
	_, ok := videoExtensions[ext]
	return ok
}

func normalizeAvailabilityComparable(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func siteSearchKeyword(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(sub.SearchMode), "imdb") && strings.TrimSpace(sub.IMDBID) != "" {
		return strings.TrimSpace(sub.IMDBID)
	}
	if u, err := url.Parse(sub.FeedURL); err == nil {
		if keyword := strings.TrimSpace(u.Query().Get("keyword")); keyword != "" {
			return keyword
		}
	}
	if keyword := strings.TrimSpace(sub.Filter); keyword != "" {
		return keyword
	}
	return strings.TrimSpace(sub.Name)
}

func (s *SubscriptionService) fetch(ctx context.Context, feedURL string) (*rssFeed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "MediaStationGo/0.1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("rss %s: %d", feedURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var f rssFeed
	if err := xml.Unmarshal(body, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func compileFilter(pat string) *regexp.Regexp {
	pat = strings.TrimSpace(pat)
	if pat == "" {
		return nil
	}
	if r, err := regexp.Compile("(?i)" + pat); err == nil {
		return r
	}
	return nil
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0)
	for _, p := range strings.Split(s, "\n") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
