package service

import (
	"context"
	"encoding/base64"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const embyGenrePrefix = "msgo-genre-"

var embyTechnicalGenreKeys = map[string]struct{}{
	"adult":  {},
	"nsfw":   {},
	"javdb":  {},
	"javbus": {},
	"onejav": {},
	"fd2ppv": {},
}

type embyGenreCount struct {
	Name  string
	Count int
}

func embyGenreID(name string) string {
	return embyGenrePrefix + base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(name)))
}

func embyGenreName(id string) (string, bool) {
	if !strings.HasPrefix(id, embyGenrePrefix) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, embyGenrePrefix))
	if err != nil {
		return "", false
	}
	name := strings.TrimSpace(string(raw))
	return name, name != ""
}

// Genres exposes real metadata genres together with the existing smart
// category result. The query always applies the same user/library visibility
// policy as /Items, so hidden adult media cannot leak through names or counts.
func (e *EmbyService) Genres(ctx context.Context, p ItemsParams) (map[string]any, error) {
	return e.genresSQL(ctx, p)
}

func (e *EmbyService) embyGenresForMedia(media *model.Media, mediaType string) []string {
	if media == nil {
		return nil
	}
	var categories map[string]string
	if e != nil && e.cfg != nil {
		categories = e.cfg.Organizer.Categories
	}
	values := make([]string, 0, len(splitCSV(media.Genres))+1)
	if adultType := adultMediaDisplayType(media, mediaType); adultType != "" {
		values = append(values, adultType)
	} else if category := automaticMediaCategory(media, mediaType, categories); category != "" {
		values = append(values, category)
	}
	for _, genre := range splitCSV(media.Genres) {
		if _, technical := embyTechnicalGenreKeys[strings.ToLower(strings.TrimSpace(genre))]; technical {
			continue
		}
		values = append(values, genre)
	}
	return uniqueFoldedStrings(values)
}

func uniqueFoldedStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func embyGenreNames(p ItemsParams) ([]string, bool) {
	names := append([]string(nil), p.Genres...)
	for _, id := range p.GenreIDs {
		name, ok := embyGenreName(strings.TrimSpace(id))
		if !ok {
			return nil, false
		}
		names = append(names, name)
	}
	return uniqueFoldedStrings(names), true
}

func hasEmbyGenreFilter(p ItemsParams) bool {
	return len(p.GenreIDs) > 0 || len(p.Genres) > 0
}

func (e *EmbyService) mediaMatchesEmbyGenres(media *model.Media, p ItemsParams) bool {
	if !hasEmbyGenreFilter(p) {
		return true
	}
	names, valid := embyGenreNames(p)
	if !valid || len(names) == 0 {
		return false
	}
	values := e.embyGenresForMedia(media, "")
	for _, value := range values {
		for _, name := range names {
			if strings.EqualFold(value, name) {
				return true
			}
		}
	}
	return false
}

func (e *EmbyService) filterMediaRowsByEmbyGenres(rows []model.Media, p ItemsParams) []model.Media {
	if !hasEmbyGenreFilter(p) {
		return rows
	}
	out := rows[:0]
	for i := range rows {
		if e.mediaMatchesEmbyGenres(&rows[i], p) {
			out = append(out, rows[i])
		}
	}
	return out
}

func embyGenrePayload(genre embyGenreCount) map[string]any {
	return map[string]any{
		"Id":                 embyGenreID(genre.Name),
		"Name":               genre.Name,
		"ServerId":           embyServerID,
		"Type":               "Genre",
		"IsFolder":           true,
		"RecursiveItemCount": genre.Count,
	}
}

func embyGenreItems(names []string) []map[string]string {
	items := make([]map[string]string, 0, len(names))
	for _, name := range names {
		items = append(items, map[string]string{"Name": name, "Id": embyGenreID(name)})
	}
	return items
}

func (e *EmbyService) genreItem(ctx context.Context, userID, name string) (map[string]any, error) {
	out, err := e.Genres(ctx, ItemsParams{UserID: userID, SearchTerm: name, Limit: 500})
	if err != nil {
		return nil, err
	}
	for _, item := range out["Items"].([]map[string]any) {
		if value, _ := item["Name"].(string); strings.EqualFold(value, name) {
			return item, nil
		}
	}
	return nil, nil
}
