// Package service — scraper orchestrator.
//
// ScraperService takes a Media row and tries to enrich it with metadata
// from one or more providers. Selection is driven by the library type:
//
//	library.type == "anime"   -> Bangumi  (fallback: TMDb)
//	library.type == "tv"      -> TheTVDB  (fallback: TMDb)
//	default                   -> TMDb
//
// After the primary match we optionally upgrade poster / backdrop with
// Fanart.tv when an API key is configured.
package service

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// ScraperService coordinates metadata enrichment across providers.
type ScraperService struct {
	cfg     *config.Config
	log     *zap.Logger
	repo    *repository.Container
	tmdb    *TMDbProvider
	bangumi *BangumiProvider
	thetvdb *TheTVDBProvider
	fanart  *FanartProvider
	adult   *AdultProvider
	hub     *Hub
}

// NewScraperService is the constructor.
func NewScraperService(
	cfg *config.Config,
	log *zap.Logger,
	repo *repository.Container,
	tmdb *TMDbProvider,
	bangumi *BangumiProvider,
	thetvdb *TheTVDBProvider,
	fanart *FanartProvider,
	hub *Hub,
	adult ...*AdultProvider,
) *ScraperService {
	var adultProvider *AdultProvider
	if len(adult) > 0 {
		adultProvider = adult[0]
	}
	return &ScraperService{
		cfg: cfg, log: log, repo: repo,
		tmdb: tmdb, bangumi: bangumi, thetvdb: thetvdb, fanart: fanart, adult: adultProvider, hub: hub,
	}
}

// yearPattern extracts a 4-digit year (1900-2099).
var yearPattern = regexp.MustCompile(`(?:^|[^\d])(19\d{2}|20\d{2})(?:[^\d]|$)`)

// noiseTokens are stripped before search.
var noiseTokens = []string{
	// 视频规格
	"1080p", "2160p", "4k", "720p", "480p", "uhd", "ds4k", "fhd",
	"bd", "bdrip", "brrip", "dvd", "dvdrip", "hdtv", "pdtv", "webdl",
	"hdrip", "bluray", "blu-ray", "webrip", "web-dl", "web",
	"x264", "x265", "h264", "h265", "hevc", "avc", "10bit", "8bit", "hi10p", "hi10",
	"hdr", "hdr10", "sdr", "dts", "ddp", "ddp5", "dd5", "dd2", "eac3", "truehd",
	"dovi", "atmos", "aac", "ac3", "flac",
	"remux", "extended", "uncut", "remastered", "repack", "proper", "internal",
	"limited", "imax", "directors-cut", "directors_cut",
	"hkfree", "yify", "rarbg", "ettv", "fgt", "tgx", "ctrlhd", "ntb", "flux",

	// 流媒体平台 / 字幕组 / 国家版本（动漫常见）
	"netflix", "nf", "amzn", "hulu", "disney", "max", "hbo",
	"linetv", "ourtv", "iqiyi", "youku", "bilibili", "qiyi", "krj",
	"crunchyroll", "funimation", "anidb", "horriblesubs", "subsplease",
	"erai-raws", "judas", "asw", "smcat", "leopard-raws", "ohys-raws",

	// 中文字幕标记
	"zm", "zw", "ch", "chs", "cht", "cn", "tc", "sc",
	"中字", "繁字", "简中", "繁中", "国语", "粤语", "日语",

	// 季数前缀残留 — ParseEpisode 已抽取过
	"season",
}

var noiseTokenSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(noiseTokens)+1)
	for _, token := range noiseTokens {
		set[token] = struct{}{}
	}
	set["dl"] = struct{}{}
	return set
}()

// bracketedTag matches "[anything]" or "(anything)" segments.
var bracketedTag = regexp.MustCompile(`[\[\(][^\]\)]*[\]\)]`)
var multiWordNoise = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bweb[\s._-]*dl\b`),
	regexp.MustCompile(`(?i)\bblu[\s._-]*ray\b`),
	regexp.MustCompile(`(?i)\bdirectors[\s._-]*cut\b`),
	regexp.MustCompile(`(?i)\berai[\s._-]*raws\b`),
	regexp.MustCompile(`(?i)\bohys[\s._-]*raws\b`),
}

// CleanQuery converts a filename like "Inception.2010.1080p.BluRay.x264.mkv"
// into a TMDb-friendly title plus an optional year hint.
func CleanQuery(raw string) (title string, year int) {
	name := strings.TrimSuffix(filepath.Base(raw), filepath.Ext(raw))
	lower := strings.ToLower(name)

	if m := yearPattern.FindStringSubmatch(lower); len(m) >= 2 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			year = v
			lower = strings.ReplaceAll(lower, m[1], " ")
		}
	}

	lower = bracketedTag.ReplaceAllString(lower, " ")

	lower = patSEnE.ReplaceAllString(lower, " ")
	lower = patNxE.ReplaceAllString(lower, " ")
	lower = patEP.ReplaceAllString(lower, " ")
	lower = patCN.ReplaceAllString(lower, " ")
	// 去掉中文季/部标记（如「第二季」「第2部」），避免残留在标题里既污染
	// 搜索查询又导致整理后的目录名重复季信息。
	lower = patCNSeason.ReplaceAllString(lower, " ")

	for _, pat := range multiWordNoise {
		lower = pat.ReplaceAllString(lower, " ")
	}
	for _, sep := range []string{".", "_", "-", "[", "]", "(", ")", "×"} {
		lower = strings.ReplaceAll(lower, sep, " ")
	}
	// 拆分后丢掉过短（≤1）且全为 ASCII 数字 / 字母的"碎片"，避免
	// 「2」「0」「v」之类残留干扰 TMDb 搜索。中文字符不算碎片。
	out := make([]string, 0, 8)
	for _, w := range strings.Fields(lower) {
		if _, ok := noiseTokenSet[w]; ok {
			continue
		}
		if len(w) <= 1 {
			r := []rune(w)
			if len(r) == 1 && r[0] < 128 {
				continue
			}
		}
		out = append(out, w)
	}
	title = strings.TrimSpace(strings.Join(out, " "))
	return title, year
}

// EnrichOne runs the provider chain for a single media row.
func (s *ScraperService) EnrichOne(ctx context.Context, m *model.Media) error {
	lib, err := s.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil {
		return err
	}

	seriesLike := mediaIsEpisodic(m, lib)
	var local *LocalMetadata
	if found, err := ReadLocalMetadata(m.Path, lib.Path, seriesLike); err == nil && found != nil {
		local = found
		applyLocalMetadata(m, local)
	} else if err != nil {
		s.log.Warn("read local metadata before scrape failed", zap.String("media_id", m.ID), zap.Error(err))
	}

	year := m.Year
	if year == 0 {
		_, year = CleanQuery(filepath.Base(m.Path))
	}

	if s.adult != nil && s.adult.Enabled() {
		if code := firstText(localAdultCode(local), AdultCodeFromMediaPath(m.Path), normalizeAdultCode(m.OriginalName), normalizeAdultCode(m.Title)); code != "" {
			if adultMatch, err := s.adult.Search(ctx, code); err == nil && adultMatch != nil {
				mergeLocalMetadataIntoMatch(adultMatch, local)
				return s.applyProviderMatch(ctx, m, lib, adultMatch)
			} else if err != nil {
				s.log.Debug("adult metadata search failed", zap.String("media_id", m.ID), zap.String("code", code), zap.Error(err))
			}
		}
	}

	candidates := scrapeQueryCandidates(m, lib)
	var query string
	match := (*Match)(nil)
	for _, candidate := range candidates {
		match = s.lookup(ctx, lib, candidate, year)
		query = candidate
		if match != nil {
			break
		}
	}
	if match == nil {
		if local != nil {
			return s.applyLocalMetadataMatch(ctx, m, local)
		}
		_ = s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).
			Update("scrape_status", "no_match").Error
		s.log.Info("metadata scrape no match",
			zap.String("media_id", m.ID),
			zap.String("query", query),
			zap.String("library_type", lib.Type))
		return nil
	}
	// Optional Fanart upgrade.
	if s.fanart != nil && s.fanart.Enabled() && match.TMDbID > 0 {
		if a, err := s.fanart.MovieArtwork(ctx, match.TMDbID); err == nil && a != nil {
			if a.Poster != "" {
				match.PosterURL = a.Poster
			}
			if a.Backdrop != "" {
				match.BackdropURL = a.Backdrop
			}
		}
	}
	mergeLocalMetadataIntoMatch(match, local)

	return s.applyProviderMatch(ctx, m, lib, match)
}

func localAdultCode(local *LocalMetadata) string {
	if local == nil {
		return ""
	}
	return local.AdultCode
}

func mergeLocalMetadataIntoMatch(match *Match, local *LocalMetadata) {
	if match == nil || local == nil {
		return
	}
	if local.Title != "" {
		match.Title = local.Title
	}
	if local.OriginalName != "" {
		match.OriginalName = local.OriginalName
	}
	if local.AdultCode != "" {
		match.OriginalName = local.AdultCode
		match.NSFW = true
	}
	if local.Overview != "" {
		match.Overview = local.Overview
	}
	if local.PosterURL != "" {
		match.PosterURL = local.PosterURL
	}
	if local.BackdropURL != "" {
		match.BackdropURL = local.BackdropURL
	}
	if local.Rating > 0 {
		match.Rating = local.Rating
	}
	if local.Year > 0 {
		match.Year = local.Year
	}
	if local.TMDbID > 0 {
		match.TMDbID = local.TMDbID
	}
	if local.Genres != "" {
		match.Genres = splitNFOList(local.Genres)
	}
	if local.Countries != "" {
		match.Countries = splitNFOList(local.Countries)
	}
	if local.Languages != "" {
		match.Languages = splitNFOList(local.Languages)
	}
	if local.NSFW {
		match.NSFW = true
	}
}

func (s *ScraperService) applyProviderMatch(ctx context.Context, m *model.Media, lib *model.Library, match *Match) error {
	updates := map[string]any{
		"title":         match.Title,
		"overview":      match.Overview,
		"poster_url":    match.PosterURL,
		"backdrop_url":  match.BackdropURL,
		"rating":        match.Rating,
		"year":          match.Year,
		"scrape_status": "matched",
	}
	if match.OriginalName != "" {
		updates["original_name"] = match.OriginalName
	}
	if match.TMDbID > 0 {
		updates["tm_db_id"] = match.TMDbID
	}
	if match.BangumiID > 0 {
		updates["bangumi_id"] = match.BangumiID
	}
	if match.NSFW {
		updates["nsfw"] = true
	}
	if len(match.Genres) > 0 {
		updates["genres"] = strings.Join(match.Genres, ",")
	}
	if len(match.Countries) > 0 {
		updates["countries"] = strings.Join(match.Countries, ",")
	}
	if len(match.Languages) > 0 {
		updates["languages"] = strings.Join(match.Languages, ",")
	}

	// Fetch extended metadata (languages, countries, genres) from TMDb
	if match.TMDbID > 0 && s.tmdb != nil && s.tmdb.Enabled() {
		mediaType := s.determineMediaType(lib, match)
		details, err := s.tmdb.GetDetails(ctx, match.TMDbID, mediaType)
		if err != nil {
			s.log.Warn("failed to get details from tmdb",
				zap.Int("tmdb_id", match.TMDbID),
				zap.String("type", mediaType),
				zap.Error(err))
		} else if details != nil {
			if len(details.Languages) > 0 {
				updates["languages"] = strings.Join(details.Languages, ",")
			}
			if len(details.Countries) > 0 {
				updates["countries"] = strings.Join(details.Countries, ",")
			}
			if len(details.Genres) > 0 {
				updates["genres"] = strings.Join(details.Genres, ",")
			}
			s.log.Debug("enrich: saved extended metadata",
				zap.String("media_id", m.ID),
				zap.Strings("languages", details.Languages),
				zap.Strings("countries", details.Countries),
				zap.Strings("genres", details.Genres))
		}
	}

	if err := s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).
		Updates(updates).Error; err != nil {
		return err
	}
	if refreshed, err := s.repo.Media.FindByID(ctx, m.ID); err == nil && refreshed != nil {
		if path, err := WriteMediaNFO(refreshed); err != nil {
			s.log.Warn("write nfo after scrape failed", zap.String("media_id", m.ID), zap.Error(err))
		} else {
			s.log.Debug("write nfo after scrape", zap.String("media_id", m.ID), zap.String("path", path))
		}
	}
	s.hub.Publish("scrape", map[string]any{
		"media_id":   m.ID,
		"title":      match.Title,
		"tmdb_id":    match.TMDbID,
		"bangumi_id": match.BangumiID,
		"source":     map[bool]string{true: "adult"}[match.NSFW],
	})
	return nil
}

func (s *ScraperService) applyLocalMetadataMatch(ctx context.Context, m *model.Media, local *LocalMetadata) error {
	next := *m
	applyLocalMetadata(&next, local)
	updates := map[string]any{
		"title":         next.Title,
		"scrape_status": "matched",
	}
	if next.OriginalName != "" {
		updates["original_name"] = next.OriginalName
	}
	if next.Overview != "" {
		updates["overview"] = next.Overview
	}
	if next.PosterURL != "" {
		updates["poster_url"] = next.PosterURL
	}
	if next.BackdropURL != "" {
		updates["backdrop_url"] = next.BackdropURL
	}
	if next.Rating > 0 {
		updates["rating"] = next.Rating
	}
	if next.Year > 0 {
		updates["year"] = next.Year
	}
	if next.TMDbID > 0 {
		updates["tm_db_id"] = next.TMDbID
	}
	if next.BangumiID > 0 {
		updates["bangumi_id"] = next.BangumiID
	}
	if next.SeasonNum > 0 {
		updates["season_num"] = next.SeasonNum
	}
	if next.EpisodeNum > 0 {
		updates["episode_num"] = next.EpisodeNum
	}
	if next.Genres != "" {
		updates["genres"] = next.Genres
	}
	if next.Countries != "" {
		updates["countries"] = next.Countries
	}
	if next.Languages != "" {
		updates["languages"] = next.Languages
	}
	if next.NSFW {
		updates["nsfw"] = true
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("id = ?", m.ID).Updates(updates).Error; err != nil {
		return err
	}
	s.hub.Publish("scrape", map[string]any{
		"media_id": m.ID,
		"title":    next.Title,
		"tmdb_id":  next.TMDbID,
		"source":   "local_nfo",
	})
	return nil
}

func scrapeQueryCandidates(m *model.Media, lib *model.Library) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		cleaned, _ := CleanQuery(raw)
		if cleaned == "" {
			cleaned = strings.TrimSpace(raw)
		}
		for _, candidate := range titleCandidates(cleaned) {
			key := strings.ToLower(candidate)
			if _, ok := seen[key]; ok || candidate == "" {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, candidate)
		}
	}
	if lib != nil && mediaIsEpisodic(m, lib) {
		add(seriesFolderTitle(m.Path, lib.Path))
	}
	add(m.Title)
	add(m.Path)
	if len(out) == 0 {
		out = append(out, strings.TrimSuffix(filepath.Base(m.Path), filepath.Ext(m.Path)))
	}
	return out
}

func seriesFolderTitle(mediaPath, libraryRoot string) string {
	dir := filepath.Dir(mediaPath)
	if seasonFromDir(filepath.Base(dir)) > 0 {
		dir = filepath.Dir(dir)
	}
	if libraryRoot != "" && samePath(dir, filepath.Clean(libraryRoot)) {
		return ""
	}
	base := filepath.Base(dir)
	if base == "." || base == string(filepath.Separator) {
		return ""
	}
	return base
}

func seasonFromDir(name string) int {
	if m := patSeasonFolder.FindStringSubmatch(name); len(m) >= 3 {
		for _, group := range m[1:] {
			if group != "" {
				return mustAtoi(group)
			}
		}
	}
	return 0
}

func titleCandidates(title string) []string {
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if title == "" {
		return nil
	}
	out := make([]string, 0, 2)
	if cjk := cjkTitleOnly(title); cjk != "" {
		out = append(out, cjk)
		if cjk != title {
			return out
		}
	}
	out = append(out, title)
	return out
}

func cjkTitleOnly(title string) string {
	parts := make([]string, 0, 4)
	for _, field := range strings.Fields(title) {
		if containsCJK(field) {
			parts = append(parts, field)
		}
	}
	return strings.Join(parts, " ")
}

func containsCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= '\u3400' && r <= '\u4dbf':
			return true
		case r >= '\u4e00' && r <= '\u9fff':
			return true
		case r >= '\uf900' && r <= '\ufaff':
			return true
		}
	}
	return false
}

func mediaIsEpisodic(m *model.Media, lib *model.Library) bool {
	if m != nil && (m.SeasonNum > 0 || m.EpisodeNum > 0) {
		return true
	}
	return librarySupportsSeasons(lib)
}

func librarySupportsSeasons(lib *model.Library) bool {
	if lib == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(lib.Type)) {
	case "tv", "anime", "variety", "show", "shows":
		return true
	default:
		return false
	}
}

// lookup runs the provider chain. When the library is missing we fall
// back to TMDb only.
//
// 库类型决定首选 provider：
//
//	anime  -> Bangumi  -> TMDb /search/tv  -> TMDb /search/movie
//	tv     -> TheTVDB  -> TMDb /search/tv  -> TMDb /search/movie
//	movie  -> TMDb /search/movie
//	(空)    -> TMDb /search/movie
//
// 任何 provider 错误都不会中止链式查询；只要返回 nil/err，就继续走下一个
// provider。这避免了 Bangumi token 未配置时 anime 库整体失败的问题。
func (s *ScraperService) lookup(ctx context.Context, lib *model.Library, query string, year int) *Match {
	kind := ""
	if lib != nil {
		kind = lib.Type
	}
	switch kind {
	case "anime":
		if s.bangumi != nil && s.bangumi.Enabled() {
			if m, err := s.bangumi.Search(ctx, query); err == nil && m != nil {
				return m
			} else if err != nil {
				s.log.Debug("bangumi search failed", zap.String("query", query), zap.Error(err))
			}
		}
	case "tv", "variety", "show", "shows":
		if s.thetvdb != nil && s.thetvdb.Enabled() {
			if m, err := s.thetvdb.SearchSeries(ctx, query); err == nil && m != nil {
				return m
			} else if err != nil {
				s.log.Debug("thetvdb search failed", zap.String("query", query), zap.Error(err))
			}
		}
	}
	if s.tmdb != nil && s.tmdb.Enabled() {
		// anime / tv 先用 TMDb /search/tv（剧名通常是 TV 类目）。
		if kind == "anime" || kind == "tv" || kind == "variety" || kind == "show" || kind == "shows" {
			if m, err := s.tmdb.SearchTV(ctx, query, year); err == nil && m != nil {
				return m
			} else if err != nil {
				s.log.Debug("tmdb tv search failed", zap.String("query", query), zap.Error(err))
			}
		}
		if m, err := s.tmdb.SearchMovie(ctx, query, year); err == nil && m != nil {
			return m
		} else if err != nil {
			s.log.Debug("tmdb movie search failed", zap.String("query", query), zap.Error(err))
		}
	}
	return nil
}

// EnrichLibrary runs the provider chain for every pending media in a library.
// When retryNoMatch is true it also retries rows previously marked no_match,
// which is the expected behaviour for a manual "重新刮削" action. Scanner-driven
// automatic enrichment keeps the default false path to avoid repeated scraping.
//
// Pending status includes both the canonical "pending" string and the
// empty / NULL values, because MediaRepository.Upsert can wipe the GORM
// default when re-running a scan over an already-existing row.
func (s *ScraperService) EnrichLibrary(ctx context.Context, libraryID string, retryNoMatch ...bool) (int, error) {
	var rows []model.Media
	statusFilter := "scrape_status IS NULL OR scrape_status = '' OR scrape_status = ?"
	statusArgs := []any{"pending"}
	if len(retryNoMatch) > 0 && retryNoMatch[0] {
		statusFilter += " OR scrape_status = ?"
		statusArgs = append(statusArgs, "no_match")
	}
	q := s.repo.DB.Where(statusFilter, statusArgs...)
	if libraryID != "" {
		q = q.Where("library_id = ?", libraryID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return 0, err
	}
	matched := 0
	processed := 0
	for i := range rows {
		select {
		case <-ctx.Done():
			return matched, ctx.Err()
		default:
		}
		if err := s.EnrichOne(ctx, &rows[i]); err != nil {
			s.log.Warn("enrich failed", zap.String("media", rows[i].ID), zap.Error(err))
			continue
		}
		processed++
		if s.mediaIsMatched(ctx, rows[i].ID) {
			matched++
		}
		time.Sleep(250 * time.Millisecond) // ~4 RPS
	}
	s.hub.Publish("scrape", map[string]any{
		"library_id": libraryID,
		"finished":   true,
		"matched":    matched,
		"processed":  processed,
	})
	return matched, nil
}

func (s *ScraperService) mediaIsMatched(ctx context.Context, mediaID string) bool {
	var status string
	err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("scrape_status").
		Where("id = ?", mediaID).
		Scan(&status).Error
	return err == nil && status == "matched"
}

// AnyEnabled reports whether at least one provider can run.
func (s *ScraperService) AnyEnabled() bool {
	if s.tmdb != nil && s.tmdb.Enabled() {
		return true
	}
	if s.bangumi != nil && s.bangumi.Enabled() {
		return true
	}
	if s.thetvdb != nil && s.thetvdb.Enabled() {
		return true
	}
	if s.adult != nil && s.adult.Enabled() {
		return true
	}
	return false
}

// determineMediaType returns "tv" for TV shows and "movie" for movies.
// It uses the library type as the primary signal.
func (s *ScraperService) determineMediaType(lib *model.Library, match *Match) string {
	if lib != nil {
		switch lib.Type {
		case "tv", "anime", "variety", "show", "shows":
			return "tv"
		}
	}
	// Fallback: if Bangumi ID is present, treat as TV/anime
	if match != nil && match.BangumiID > 0 {
		return "tv"
	}
	return "movie"
}
