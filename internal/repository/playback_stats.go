package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

type PlaybackStatsFilter struct {
	From, To, RankFrom, RankTo          time.Time
	UserID, LibraryID, MediaType, Grain string
	Page, PageSize                      int
}
type PlaybackStatsBucket struct {
	Period string `json:"period"`
	Count  int64  `json:"count"`
}
type PlaybackStatsDetail struct {
	model.PlaybackEvent
	UserName    string `json:"user_name"`
	LibraryName string `json:"library_name"`
}
type PlaybackRankItem struct {
	WorkKey   string `json:"work_key"`
	Title     string `json:"title"`
	PosterURL string `json:"poster_url"`
	MediaID   string `json:"media_id"`
	SeasonNum int    `json:"season_num"`
	Count     int64  `json:"count"`
}
type PlaybackStatsResult struct {
	Total    int64                 `json:"total"`
	Buckets  []PlaybackStatsBucket `json:"buckets"`
	Items    []PlaybackStatsDetail `json:"items"`
	Ranking  []PlaybackRankItem    `json:"ranking"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
	TimeZone string                `json:"time_zone"`
}

func playbackStatsQuery(db *gorm.DB, f PlaybackStatsFilter) *gorm.DB {
	q := db.Model(&model.PlaybackEvent{}).Where("played_at >= ? AND played_at < ?", f.From, f.To)
	if f.UserID != "" {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.LibraryID != "" {
		q = q.Where("library_id = ?", f.LibraryID)
	}
	if f.MediaType != "" {
		q = q.Where("media_type = ?", f.MediaType)
	}
	return q
}

func (r *Container) PlaybackStats(ctx context.Context, f PlaybackStatsFilter) (*PlaybackStatsResult, error) {
	if f.Page < 1 || f.Page > 1_000_000 || f.PageSize < 1 || f.PageSize > 100 || (f.Grain != "day" && f.Grain != "week" && f.Grain != "month") {
		return nil, errors.New("invalid playback statistics filter")
	}
	result := &PlaybackStatsResult{Items: []PlaybackStatsDetail{}, Buckets: []PlaybackStatsBucket{}, Ranking: []PlaybackRankItem{}, Page: f.Page, PageSize: f.PageSize, TimeZone: "UTC"}
	db := r.DB.WithContext(ctx)
	if err := playbackStatsQuery(db, f).Count(&result.Total).Error; err != nil {
		return nil, err
	}
	expression := "strftime('%Y-%m-%d', played_at)"
	if f.Grain == "month" {
		expression = "strftime('%Y-%m-01', played_at)"
	}
	if f.Grain == "week" {
		expression = "date(played_at, '-' || ((CAST(strftime('%w', played_at) AS integer) + 6) % 7) || ' days')"
	}
	if db.Dialector.Name() == "postgres" {
		expression = "to_char(date_trunc('" + f.Grain + "', played_at AT TIME ZONE 'UTC'), 'YYYY-MM-DD')"
	}
	if err := playbackStatsQuery(db, f).Select(expression + " AS period, COUNT(*) AS count").Group(expression).Order("period").Scan(&result.Buckets).Error; err != nil {
		return nil, err
	}
	var events []model.PlaybackEvent
	if err := playbackStatsQuery(db, f).Order("played_at DESC, id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&events).Error; err != nil {
		return nil, err
	}
	userIDs, libraryIDs := []string{}, []string{}
	for _, event := range events {
		userIDs = append(userIDs, event.UserID)
		libraryIDs = append(libraryIDs, event.LibraryID)
	}
	users, libraries := []model.User{}, []model.Library{}
	if len(events) > 0 {
		if err := db.Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, err
		}
		if err := db.Where("id IN ?", libraryIDs).Find(&libraries).Error; err != nil {
			return nil, err
		}
	}
	userNames, libraryNames := map[string]string{}, map[string]string{}
	for _, user := range users {
		userNames[user.ID] = user.Username
	}
	for _, library := range libraries {
		libraryNames[library.ID] = library.Name
	}
	for _, event := range events {
		result.Items = append(result.Items, PlaybackStatsDetail{PlaybackEvent: event, UserName: userNames[event.UserID], LibraryName: libraryNames[event.LibraryID]})
	}
	f.From, f.To = f.RankFrom, f.RankTo
	if err := playbackStatsQuery(db, f).Select("work_key, MIN(title) AS title, MIN(poster_url) AS poster_url, MIN(media_id) AS media_id, MAX(season_num) AS season_num, COUNT(*) AS count").Group("work_key").Order("count DESC, work_key ASC").Limit(10).Scan(&result.Ranking).Error; err != nil {
		return nil, err
	}
	return result, nil
}

type PlayerRequestLogFilter struct {
	From, To               time.Time
	Route, Method, UserID  string
	Status, Page, PageSize int
}

func (r *Container) ListPlayerRequestLogs(ctx context.Context, f PlayerRequestLogFilter) ([]model.PlayerRequestLog, int64, error) {
	if f.Page < 1 || f.Page > 1_000_000 || f.PageSize < 1 || f.PageSize > 100 {
		return nil, 0, errors.New("invalid request log page")
	}
	q := r.DB.WithContext(ctx).Model(&model.PlayerRequestLog{}).Where("requested_at >= ? AND requested_at < ?", f.From, f.To)
	if f.Route != "" {
		pattern := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.ToLower(f.Route))
		q = q.Where("LOWER(route) LIKE ? ESCAPE '\\'", "%"+pattern+"%")
	}
	if f.Method != "" {
		q = q.Where("method = ?", strings.ToUpper(f.Method))
	}
	if f.UserID != "" {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.Status != 0 {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []model.PlayerRequestLog{}
	err := q.Order("requested_at DESC, id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&items).Error
	return items, total, err
}
