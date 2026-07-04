package service

import (
	"strconv"
	"strings"
)

func metadataFromDoc(doc *nfoDocument, baseDir string, seriesLike bool) *LocalMetadata {
	if doc == nil {
		return nil
	}
	meta := &LocalMetadata{
		Title:        cleanXMLText(doc.Title),
		OriginalName: cleanXMLText(doc.OriginalTitle),
		AdultCode:    normalizeAdultCode(doc.Num),
		Year:         int(doc.Year),
		ReleaseDate:  normalizeReleaseDate(firstText(doc.Premiered, doc.ReleaseDate, doc.Release, doc.Aired)),
		Overview:     firstText(doc.Plot, doc.Outline, doc.OriginalPlot),
		Rating:       float32(doc.Rating),
		PosterURL:    firstRemoteURL(baseDir, nfoPosterValues(doc)...),
		BackdropURL:  firstRemoteURL(baseDir, nfoBackdropValues(doc)...),
		TMDbID:       int(doc.TMDbID),
		BangumiID:    mustAtoi(externalIDFromUniqueIDs(doc.UniqueIDs, "bangumi", "bgm")),
		DoubanID:     externalIDFromUniqueIDs(doc.UniqueIDs, "douban"),
		TheTVDBID:    externalIDFromUniqueIDs(doc.UniqueIDs, "thetvdb", "tvdb"),
		SeasonNum:    int(doc.Season),
		EpisodeNum:   int(doc.Episode),
		Genres:       joinNFOValues(adultAwareGenres(doc)),
		Countries:    joinNFOValues(doc.Countries),
		Languages:    joinNFOValues(doc.Languages),
		HasNFO:       true,
	}
	if nfoIsEpisodeDetails(doc) {
		meta.EpisodeTitle = cleanXMLText(doc.Title)
	}
	if meta.AdultCode == "" {
		meta.AdultCode = normalizeAdultCode(firstText(doc.OriginalTitle, doc.SortTitle, doc.Title))
	}
	if meta.AdultCode != "" {
		meta.NSFW = true
		if meta.OriginalName == "" || strings.EqualFold(meta.OriginalName, meta.Title) {
			meta.OriginalName = meta.AdultCode
		}
	}
	if seriesLike && cleanXMLText(doc.ShowTitle) != "" {
		meta.Title = cleanXMLText(doc.ShowTitle)
		if cleanXMLText(doc.Title) != "" {
			meta.OriginalName = cleanXMLText(doc.Title)
		}
	}
	if meta.Year == 0 {
		meta.Year = yearFromDate(meta.ReleaseDate)
	}
	if meta.TMDbID == 0 {
		meta.TMDbID = tmdbIDFromUniqueIDs(doc.UniqueIDs)
	}
	return meta
}

func adultAwareGenres(doc *nfoDocument) []string {
	if doc == nil {
		return nil
	}
	values := make([]string, 0, len(doc.Genres)+len(doc.Tags)+len(doc.Actors)+4)
	values = append(values, doc.Genres...)
	values = append(values, doc.Tags...)
	for _, value := range []string{doc.Studio, doc.Maker, doc.Publisher, doc.Label} {
		if cleanXMLText(value) != "" {
			values = append(values, cleanXMLText(value))
		}
	}
	for _, value := range doc.Directors {
		if cleanXMLText(value) != "" {
			values = append(values, cleanXMLText(value))
		}
	}
	for _, actor := range doc.Actors {
		if cleanXMLText(actor.Name) != "" {
			values = append(values, cleanXMLText(actor.Name))
		} else if cleanXMLText(actor.Role) != "" {
			values = append(values, cleanXMLText(actor.Role))
		}
	}
	return values
}

// mergeEpisodeMetadata 把单集 sidecar NFO(<episodedetails>)合并进整剧元数据
// dst(通常来自 tvshow.nfo)。
//
// 关键约束:「整剧级」字段(Title/OriginalName/TMDbID/BangumiID/DoubanID/TheTVDBID)
// 是合集分组键的依据,必须保证「同一部剧的各集一致」。而单集 NFO 里的
// <uniqueid type="tmdb"> 是【单集 episode id】(如 4375419)、<title> 是【单集名】
// (如「九龙拉棺」),都是单集级数据 —— 一旦写进整剧字段,同剧每集的 id/原名互不
// 相同,会被前端 getSeriesKey / Emby seriesGroupsFromMedia 拆成多张卡(每集一卡)。
// 因此:
//   - 单集外部 id(tmdb/bangumi/douban/thetvdb)一律【不写入】整剧外部 id;
//     整剧 id 只认 tvshow.nfo(已在 dst);无则留空,交由路径剧名分组兜底。
//   - 单集名【不写入】OriginalName(整剧原名);整剧原名只来自 tvshow.nfo。
//   - 仅 overview/rating/剧照/季集号等【单集级】字段按集回填(不影响分组)。
func mergeEpisodeMetadata(dst, episode *LocalMetadata, doc *nfoDocument) {
	showTitle := cleanXMLText(doc.ShowTitle)
	// 整剧标题: 优先 <showtitle>(MoviePilot 在单集 NFO 里也会写整剧名);
	// 其次保留 dst 已有(来自 tvshow.nfo)。不要把单集 <title> 当整剧标题,
	// 否则会把"第 11 集/第几期"整理成整剧目录并污染后续 TMDb 查询。
	if showTitle != "" {
		dst.Title = showTitle
	}
	// 注意: 不要把单集名 / 单集 originaltitle 写进 OriginalName(整剧原名,分组键)。
	if episodeTitle := firstText(episode.EpisodeTitle, doc.Title); episodeTitle != "" && !strings.EqualFold(episodeTitle, showTitle) {
		dst.EpisodeTitle = episodeTitle
	}

	// 单集级展示字段: 每个媒体行本就对应一集,这些可安全按集回填。
	if dst.Year == 0 && episode.Year > 0 {
		dst.Year = episode.Year
	}
	if episode.Overview != "" {
		dst.Overview = episode.Overview
	}
	if episode.Rating > 0 {
		dst.Rating = episode.Rating
	}
	if episode.PosterURL != "" {
		dst.PosterURL = episode.PosterURL
	}
	if episode.BackdropURL != "" {
		dst.BackdropURL = episode.BackdropURL
	}
	// 整剧外部 id: 单集 NFO 的 id 都是单集级,绝不写入整剧字段(见上方说明)。
	if episode.SeasonNum > 0 {
		dst.SeasonNum = episode.SeasonNum
	}
	if episode.EpisodeNum > 0 {
		dst.EpisodeNum = episode.EpisodeNum
	}
	// 题材/地区/语言为整剧级,单集 NFO 偶尔携带时仅在整剧未提供时回填。
	if dst.Genres == "" && episode.Genres != "" {
		dst.Genres = episode.Genres
	}
	if dst.Countries == "" && episode.Countries != "" {
		dst.Countries = episode.Countries
	}
	if dst.Languages == "" && episode.Languages != "" {
		dst.Languages = episode.Languages
	}
}

func nfoIsEpisodeDetails(doc *nfoDocument) bool {
	if doc == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(doc.XMLName.Local), "episodedetails")
}

func tmdbIDFromUniqueIDs(ids []nfoUniqueID) int {
	value := externalIDFromUniqueIDs(ids, "tmdb")
	if value == "" {
		return 0
	}
	v, _ := strconv.Atoi(value)
	return v
}

func externalIDFromUniqueIDs(ids []nfoUniqueID, types ...string) string {
	for _, id := range ids {
		idType := strings.TrimSpace(id.Type)
		for _, typ := range types {
			if strings.EqualFold(idType, typ) {
				return strings.TrimSpace(id.Value)
			}
		}
	}
	return ""
}
