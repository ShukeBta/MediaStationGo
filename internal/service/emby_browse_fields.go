package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (e *EmbyService) embyBrowseConfigKey() string {
	var categories map[string]string
	if e.cfg != nil {
		categories = e.cfg.Organizer.Categories
	}
	// A map of strings is always JSON encodable; map keys are serialized in
	// sorted order, so config changes invalidate derived classifications.
	body, _ := json.Marshal(categories)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (e *EmbyService) prepareEmbyBrowseFields(row *model.Media) {
	row.EmbySeriesKey = e.seriesIDForMedia(row)
	row.EmbyListKey = row.EmbySeriesKey
	if strings.TrimSpace(row.PartGroupKey) != "" {
		row.EmbyListKey = multipartSeriesID(row.LibraryID, row.PartGroupKey)
	}
	row.EmbySeriesName = fallbackEmbySeriesName(row)
	row.EmbyPremiereDate = ""
	if date, valid := embyPremiereDate(row.ReleaseDate); valid {
		row.EmbyPremiereDate = date.Format("2006-01-02")
	}
	// API-only library labels are not media facts and must not influence a
	// projection calculated during ingest versus one rebuilt from the database.
	classification := *row
	classification.LibraryName, classification.LibraryPath = "", ""
	classification.DisplayLibraryName, classification.DisplayLibraryPath = "", ""
	genres := e.embyGenresForMedia(&classification, "")
	if genres == nil {
		genres = []string{}
	}
	for i := range genres {
		genres[i] = strings.ToLower(genres[i])
	}
	body, _ := json.Marshal(genres)
	row.EmbyGenres = string(body)
	variants := make(map[string][]string)
	for _, mediaType := range []string{"", "movie", "tv", "anime", "variety", "adult", "nsfw"} {
		key := mediaType
		if key == "" {
			key = "auto"
		}
		values := e.embyGenresForMedia(&classification, mediaType)
		if values == nil {
			values = []string{}
		}
		variants[key] = values
	}
	body, _ = json.Marshal(variants)
	row.EmbyGenreVariants = string(body)
	row.EmbyConfigKey = e.embyBrowseConfigKey()
}
