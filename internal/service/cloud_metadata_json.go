package service

import (
	"encoding/json"
	"strconv"
	"strings"
)

type cloudJSONArtwork struct {
	posterValues   []string
	backdropValues []string
}

func metadataFromCloudJSON(body []byte) (*LocalMetadata, cloudJSONArtwork) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, cloudJSONArtwork{}
	}
	obj := firstMetadataJSONObject(raw)
	if len(obj) == 0 {
		return nil, cloudJSONArtwork{}
	}
	meta := &LocalMetadata{
		Title:        firstJSONString(obj, "title", "name", "showtitle", "show_title"),
		OriginalName: firstJSONString(obj, "original_title", "originaltitle", "original_name", "originalname", "sorttitle"),
		EpisodeTitle: firstJSONString(obj, "episode_title", "episodetitle", "episode_name", "episodename"),
		Year:         firstJSONInt(obj, "year"),
		ReleaseDate:  normalizeReleaseDate(firstJSONString(obj, "release_date", "releasedate", "premiered", "aired", "date")),
		Overview:     firstJSONString(obj, "overview", "plot", "outline", "summary", "description"),
		Rating:       firstJSONFloat(obj, "rating", "vote_average", "score"),
		TMDbID:       firstJSONInt(obj, "tmdb_id", "tmdbid", "tmdb"),
		BangumiID:    firstJSONInt(obj, "bangumi_id", "bangumiid", "bgm_id"),
		DoubanID:     firstJSONString(obj, "douban_id", "doubanid"),
		TheTVDBID:    firstJSONString(obj, "thetvdb_id", "tvdb_id", "thetvdbid", "tvdbid"),
		SeasonNum:    firstJSONInt(obj, "season", "season_num", "season_number"),
		EpisodeNum:   firstJSONInt(obj, "episode", "episode_num", "episode_number"),
		Genres:       firstJSONList(obj, "genres", "genre", "tags"),
		Countries:    firstJSONList(obj, "countries", "country", "production_countries"),
		Languages:    firstJSONList(obj, "languages", "language", "spoken_languages"),
	}
	if showTitle := firstJSONString(obj, "showtitle", "show_title", "series_title", "series_name"); showTitle != "" {
		if meta.EpisodeTitle == "" && meta.Title != "" && !strings.EqualFold(strings.TrimSpace(meta.Title), strings.TrimSpace(showTitle)) {
			meta.EpisodeTitle = meta.Title
		}
		meta.Title = showTitle
	}
	if meta.Year == 0 {
		meta.Year = yearFromDate(meta.ReleaseDate)
	}
	artwork := cloudJSONArtwork{
		posterValues: firstJSONStrings(obj,
			"poster_url", "poster", "poster_path", "cover", "cover_url", "thumb", "thumbnail", "image"),
		backdropValues: firstJSONStrings(obj,
			"backdrop_url", "backdrop", "backdrop_path", "fanart", "fanart_url", "background", "landscape"),
	}
	if images, ok := jsonObject(obj["images"]); ok {
		artwork.posterValues = append(artwork.posterValues, firstJSONStrings(images, "poster", "large", "common", "medium", "small", "cover")...)
		artwork.backdropValues = append(artwork.backdropValues, firstJSONStrings(images, "backdrop", "fanart", "background", "landscape")...)
	}
	if art, ok := jsonObject(obj["art"]); ok {
		artwork.posterValues = append(artwork.posterValues, firstJSONStrings(art, "poster", "thumb", "cover")...)
		artwork.backdropValues = append(artwork.backdropValues, firstJSONStrings(art, "fanart", "backdrop", "background", "landscape")...)
	}
	if len(artwork.posterValues) > 0 {
		meta.PosterURL = firstHTTPJSONValue(artwork.posterValues)
	}
	if len(artwork.backdropValues) > 0 {
		meta.BackdropURL = firstHTTPJSONValue(artwork.backdropValues)
	}
	if meta.PosterURL != "" || meta.BackdropURL != "" {
		meta.HasArtwork = true
	}
	if localHasDescriptiveMetadata(meta) || meta.HasArtwork || len(artwork.posterValues) > 0 || len(artwork.backdropValues) > 0 {
		meta.HasNFO = true
		return meta, artwork
	}
	return nil, cloudJSONArtwork{}
}

func applyCloudJSONArtwork(typ string, sidecars cloudSidecarSet, meta *LocalMetadata, artwork cloudJSONArtwork) *LocalMetadata {
	if meta == nil {
		meta = &LocalMetadata{}
	}
	if meta.PosterURL == "" {
		if ref := cloudImageRefFromNFOValues(sidecars, artwork.posterValues...); ref != "" {
			meta.PosterURL = cloudPlaybackURL(typ, ref)
			meta.HasArtwork = true
		}
	}
	if meta.BackdropURL == "" {
		if ref := cloudImageRefFromNFOValues(sidecars, artwork.backdropValues...); ref != "" {
			meta.BackdropURL = cloudPlaybackURL(typ, ref)
			meta.HasArtwork = true
		}
	}
	return meta
}

func firstMetadataJSONObject(raw any) map[string]any {
	obj, ok := jsonObject(raw)
	if !ok {
		return nil
	}
	for _, key := range []string{"movie", "media", "metadata", "item", "data"} {
		if nested, ok := jsonObject(obj[key]); ok && jsonObjectLooksLikeMetadata(nested) {
			return nested
		}
	}
	return obj
}

func jsonObjectLooksLikeMetadata(obj map[string]any) bool {
	for _, key := range []string{"title", "name", "overview", "plot", "tmdb_id", "tmdbid", "poster", "poster_url", "poster_path", "backdrop", "backdrop_path"} {
		if _, ok := obj[key]; ok {
			return true
		}
	}
	return false
}

func jsonObject(raw any) (map[string]any, bool) {
	obj, ok := raw.(map[string]any)
	return obj, ok
}

func firstJSONString(obj map[string]any, keys ...string) string {
	values := firstJSONStrings(obj, keys...)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstJSONStrings(obj map[string]any, keys ...string) []string {
	out := []string{}
	for _, key := range keys {
		value, ok := lookupJSONKey(obj, key)
		if !ok {
			continue
		}
		out = append(out, jsonStrings(value)...)
		if len(out) > 0 {
			return out
		}
	}
	return out
}

func firstJSONInt(obj map[string]any, keys ...string) int {
	for _, key := range keys {
		value, ok := lookupJSONKey(obj, key)
		if !ok {
			continue
		}
		if i := jsonInt(value); i > 0 {
			return i
		}
	}
	return 0
}

func firstJSONFloat(obj map[string]any, keys ...string) float32 {
	for _, key := range keys {
		value, ok := lookupJSONKey(obj, key)
		if !ok {
			continue
		}
		if f := jsonFloat(value); f > 0 {
			return f
		}
	}
	return 0
}

func firstJSONList(obj map[string]any, keys ...string) string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, key := range keys {
		value, ok := lookupJSONKey(obj, key)
		if !ok {
			continue
		}
		for _, part := range jsonStrings(value) {
			for _, item := range strings.Split(part, ",") {
				item = strings.TrimSpace(item)
				if item == "" {
					continue
				}
				dedupeKey := strings.ToLower(item)
				if _, exists := seen[dedupeKey]; exists {
					continue
				}
				seen[dedupeKey] = struct{}{}
				out = append(out, item)
			}
		}
		if len(out) > 0 {
			return strings.Join(out, ",")
		}
	}
	return ""
}

func lookupJSONKey(obj map[string]any, key string) (any, bool) {
	for existing, value := range obj {
		if strings.EqualFold(strings.TrimSpace(existing), key) {
			return value, true
		}
	}
	return nil, false
}

func jsonStrings(value any) []string {
	switch v := value.(type) {
	case string:
		if text := strings.TrimSpace(v); text != "" {
			return []string{text}
		}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, jsonStrings(item)...)
		}
		return out
	case map[string]any:
		return firstJSONStrings(v, "name", "title", "value", "iso_3166_1", "iso_639_1")
	case float64:
		if v > 0 {
			return []string{strconv.Itoa(int(v))}
		}
	}
	return nil
}

func jsonInt(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(v))
		return i
	}
	return 0
}

func jsonFloat(value any) float32 {
	switch v := value.(type) {
	case float64:
		return float32(v)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 32)
		return float32(f)
	}
	return 0
}

func firstHTTPJSONValue(values []string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if isHTTPURL(value) {
			return value
		}
	}
	return ""
}
