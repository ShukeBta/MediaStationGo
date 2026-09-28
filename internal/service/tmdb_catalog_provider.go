package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var tmdbCatalogPath = regexp.MustCompile(`/(movie|tv)/([1-9][0-9]*)(?:/season/([0-9]+)(?:/episode/([1-9][0-9]*))?)?$`)

type tmdbCatalogPayload struct {
	ID            int               `json:"id"`
	Name          string            `json:"name"`
	Title         string            `json:"title"`
	Overview      string            `json:"overview"`
	AirDate       string            `json:"air_date"`
	FirstAirDate  string            `json:"first_air_date"`
	ReleaseDate   string            `json:"release_date"`
	PosterPath    string            `json:"poster_path"`
	StillPath     string            `json:"still_path"`
	SeasonNumber  *int              `json:"season_number"`
	EpisodeNumber *int              `json:"episode_number"`
	EpisodeCount  int               `json:"episode_count"`
	Seasons       []json.RawMessage `json:"seasons"`
	Episodes      []json.RawMessage `json:"episodes"`
}

func (t *TMDbProvider) SetCatalogRepository(repo *repository.TMDbCatalogRepository) { t.catalog = repo }

func (t *TMDbProvider) persistCatalogResponse(ctx context.Context, path string, raw json.RawMessage) error {
	if t.catalog == nil {
		return nil
	}
	match := tmdbCatalogPath.FindStringSubmatch(path)
	if match == nil {
		return nil
	}
	rootID, _ := strconv.Atoi(match[2])
	key := match[1] + "/" + match[2]
	kind, season, episode := "series", 0, 0
	if match[1] == "movie" {
		kind = "movie"
	}
	if match[3] != "" {
		kind = "season"
		season, _ = strconv.Atoi(match[3])
		key += "/season/" + match[3]
	}
	if match[4] != "" {
		kind = "episode"
		episode, _ = strconv.Atoi(match[4])
		key += "/episode/" + match[4]
	}
	item, data, err := t.catalogItem(key, kind, rootID, season, episode, raw, true)
	if err != nil {
		return err
	}
	if kind == "season" && data.Episodes == nil {
		return fmt.Errorf("TMDB season catalog is missing its episode list: %s", key)
	}
	items := []model.TMDbCatalogItem{item}
	children, childKind := data.Seasons, "season"
	if kind == "season" {
		children, childKind = data.Episodes, "episode"
	}
	if kind != "series" && kind != "season" {
		children = nil
	}
	for _, child := range children {
		var identity tmdbCatalogPayload
		if json.Unmarshal(child, &identity) != nil || identity.ID <= 0 {
			return fmt.Errorf("TMDB catalog contains an invalid child: %s", key)
		}
		childSeason, childEpisode := season, 0
		childKey := key
		if childKind == "season" {
			if identity.SeasonNumber == nil || *identity.SeasonNumber < 0 {
				continue
			}
			childSeason = *identity.SeasonNumber
			childKey += fmt.Sprintf("/season/%d", childSeason)
		} else {
			if identity.EpisodeNumber == nil || *identity.EpisodeNumber <= 0 {
				continue
			}
			childEpisode = *identity.EpisodeNumber
			childKey += fmt.Sprintf("/episode/%d", childEpisode)
		}
		entry, _, err := t.catalogItem(childKey, childKind, rootID, childSeason, childEpisode, child, false)
		if err != nil {
			return err
		}
		items = append(items, entry)
	}
	return t.catalog.Save(ctx, items)
}

func (t *TMDbProvider) catalogItem(key, kind string, rootID, season, episode int, raw json.RawMessage, complete bool) (model.TMDbCatalogItem, tmdbCatalogPayload, error) {
	var data tmdbCatalogPayload
	err := json.Unmarshal(raw, &data)
	if err == nil && (data.ID <= 0 || ((kind == "series" || kind == "movie") && data.ID != rootID) ||
		(kind == "season" && (data.SeasonNumber == nil || *data.SeasonNumber != season)) ||
		(kind == "episode" && (data.EpisodeNumber == nil || *data.EpisodeNumber != episode || (data.SeasonNumber != nil && *data.SeasonNumber != season)))) {
		err = fmt.Errorf("TMDB catalog identity mismatch: %s", key)
	}
	if err != nil {
		return model.TMDbCatalogItem{}, data, err
	}
	item := model.TMDbCatalogItem{Key: key, Kind: kind, RootID: rootID, TMDbID: data.ID, SeasonNum: season, EpisodeNum: episode,
		Title: data.Name, Overview: data.Overview, ReleaseDate: data.AirDate, Snapshot: string(raw), Complete: complete, FetchedAt: time.Now().UTC(), EpisodeCount: data.EpisodeCount}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, name := range []string{"credits", "translations", "external_ids", "videos"} {
		if _, exists := fields[name]; exists {
			item.Expanded = true
		}
	}
	if item.Title == "" {
		item.Title = data.Title
	}
	if item.ReleaseDate == "" {
		item.ReleaseDate = data.FirstAirDate
	}
	if item.ReleaseDate == "" {
		item.ReleaseDate = data.ReleaseDate
	}
	if data.PosterPath != "" {
		item.PosterURL = t.imgCDN + "/w500" + data.PosterPath
	}
	if data.StillPath != "" {
		item.StillURL = t.imgCDN + "/w500" + data.StillPath
	}
	if kind == "season" && complete {
		item.EpisodeCount = len(data.Episodes)
	}
	return item, data, nil
}

func (t *TMDbProvider) fetchCatalog(ctx context.Context, key string) error {
	if !tmdbCatalogPath.MatchString("/" + key) {
		return fmt.Errorf("invalid TMDB catalog key")
	}
	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return fmt.Errorf("TMDB API key is unavailable")
	}
	q := url.Values{"api_key": {apiKey}, "language": {"zh-CN"}, "append_to_response": {"credits,external_ids,translations"}}
	var raw json.RawMessage
	return t.getJSON(ctx, strings.TrimRight(t.resolveBaseURL(ctx), "/")+"/"+key+"?"+q.Encode(), &raw)
}
