package repository

import (
	"context"
	"errors"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
	"time"
)

type TaskHistoryFilter struct {
	From, To             time.Time
	Kind, TaskID, Status string
	Page, PageSize       int
}
type TaskLogDay struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

func taskPageValid(page, size int) error {
	if page < 1 || page > 1_000_000 || size < 1 || size > 100 {
		return errors.New("invalid pagination")
	}
	return nil
}

func taskLogQuery(db *gorm.DB, f TaskHistoryFilter) *gorm.DB {
	q := db.Model(&model.TaskLogEntry{}).Where("logged_at >= ? AND logged_at < ?", f.From, f.To)
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.TaskID != "" {
		q = q.Where("task_id = ?", f.TaskID)
	}
	return q
}

func (r *Container) TaskLogDays(ctx context.Context, f TaskHistoryFilter) ([]TaskLogDay, error) {
	expression := "strftime('%Y-%m-%d', logged_at)"
	if r.DB.Dialector.Name() == "postgres" {
		expression = "to_char(logged_at AT TIME ZONE 'UTC', 'YYYY-MM-DD')"
	}
	items := []TaskLogDay{}
	err := taskLogQuery(r.DB.WithContext(ctx), f).Select(expression + " AS day, COUNT(*) AS count").Group(expression).Order("day DESC").Scan(&items).Error
	return items, err
}

func (r *Container) TaskLogs(ctx context.Context, f TaskHistoryFilter) ([]model.TaskLogEntry, int64, error) {
	if err := taskPageValid(f.Page, f.PageSize); err != nil {
		return nil, 0, err
	}
	q := taskLogQuery(r.DB.WithContext(ctx), f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []model.TaskLogEntry{}
	err := q.Order("logged_at DESC, id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&items).Error
	return items, total, err
}

func (r *Container) TaskExecutions(ctx context.Context, f TaskHistoryFilter) ([]model.TaskExecution, int64, error) {
	if err := taskPageValid(f.Page, f.PageSize); err != nil {
		return nil, 0, err
	}
	q := r.DB.WithContext(ctx).Model(&model.TaskExecution{}).Where("started_at >= ? AND started_at < ?", f.From, f.To)
	if f.Kind != "" {
		q = q.Where("kind = ?", f.Kind)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.TaskID != "" {
		q = q.Where("id = ?", f.TaskID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []model.TaskExecution{}
	err := q.Order("started_at DESC, id DESC").Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).Find(&items).Error
	return items, total, err
}

type PendingScrapeItem struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	LibraryID    string `json:"library_id"`
	ScrapeStatus string `json:"scrape_status"`
	IsSTRM       bool   `json:"is_strm"`
}

func (r *Container) PendingScrape(ctx context.Context, libraryID string, page, size int) ([]PendingScrapeItem, int64, error) {
	if err := taskPageValid(page, size); err != nil {
		return nil, 0, err
	}
	q := r.DB.WithContext(ctx).Model(&model.Media{}).Where("scrape_status IS NULL OR scrape_status IN ?", []string{"", "pending", "failed", "unmatched"})
	if libraryID != "" {
		q = q.Where("library_id = ?", libraryID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	items := []PendingScrapeItem{}
	err := q.Select("id,title,library_id,scrape_status,CASE WHEN COALESCE(strm_url, '') <> '' OR LOWER(path) LIKE '%.strm' THEN TRUE ELSE FALSE END AS is_strm").Order("updated_at DESC, id DESC").Offset((page - 1) * size).Limit(size).Scan(&items).Error
	return items, total, err
}
