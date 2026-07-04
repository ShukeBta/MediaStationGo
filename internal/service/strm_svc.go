// Package service — STRM 文件管理服务。
package service

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// STRM 错误定义。
var (
	ErrSTRMNotFound        = errors.New("strm record not found")
	ErrSTRMProtocolInvalid = errors.New("invalid strm protocol")
	ErrSTRMURLInvalid      = errors.New("invalid strm url")
)

// STRMService STRM 文件管理服务。
type STRMService struct {
	log  *zap.Logger
	repo *repository.Container
	cfg  *config.Config
}

// NewSTRMService 创建 STRM 服务。
func NewSTRMService(log *zap.Logger, repo *repository.Container, cfg *config.Config) *STRMService {
	return &STRMService{log: log, repo: repo, cfg: cfg}
}

// Create 创建 STRM 记录。
func (s *STRMService) Create(ctx context.Context, record *model.STRMRecord) (*model.STRMRecord, error) {
	if err := s.validateSTRM(record); err != nil {
		return nil, err
	}

	if err := s.repo.STRM.Create(ctx, record); err != nil {
		s.log.Error("create strm failed", zap.Error(err))
		return nil, err
	}

	return record, nil
}

// CreateBatch 批量创建 STRM 记录。
func (s *STRMService) CreateBatch(ctx context.Context, records []model.STRMRecord) (int, error) {
	created := 0
	for i := range records {
		if err := s.validateSTRM(&records[i]); err != nil {
			s.log.Warn("skip invalid strm record",
				zap.String("title", records[i].Title),
				zap.Error(err),
			)
			continue
		}
		created++
	}

	validRecords := make([]model.STRMRecord, 0, created)
	for _, r := range records {
		if model.IsAllowedProtocol(r.Protocol) && r.URL != "" {
			validRecords = append(validRecords, r)
		}
	}

	if len(validRecords) == 0 {
		return 0, nil
	}

	if err := s.repo.STRM.CreateBatch(ctx, validRecords); err != nil {
		s.log.Error("batch create strm failed", zap.Error(err))
		return 0, err
	}

	return len(validRecords), nil
}

// GetByID 获取 STRM 记录。
func (s *STRMService) GetByID(ctx context.Context, id string) (*model.STRMRecord, error) {
	record, err := s.repo.STRM.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, ErrSTRMNotFound
	}
	return record, nil
}

// List 列出 STRM 记录（支持筛选和分页）。
func (s *STRMService) List(ctx context.Context, filters map[string]string, page, pageSize int) ([]model.STRMRecord, int64, error) {
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	records, total, err := s.repo.STRM.List(ctx, filters, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// Update 更新 STRM 记录。
func (s *STRMService) Update(ctx context.Context, record *model.STRMRecord) (*model.STRMRecord, error) {
	existing, err := s.repo.STRM.FindByID(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrSTRMNotFound
	}

	if record.Protocol != "" {
		if !model.IsAllowedProtocol(record.Protocol) {
			return nil, ErrSTRMProtocolInvalid
		}
	}

	if err := s.repo.STRM.Update(ctx, record); err != nil {
		s.log.Error("update strm failed", zap.Error(err))
		return nil, err
	}

	return record, nil
}

// Delete 删除 STRM 记录。
func (s *STRMService) Delete(ctx context.Context, id string) error {
	existing, err := s.repo.STRM.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrSTRMNotFound
	}
	return s.repo.STRM.Delete(ctx, id)
}

// GetProtocols 获取支持的协议列表。
func (s *STRMService) GetProtocols() []string {
	return model.AllowedSTRMProtocols
}

// validateSTRM 验证 STRM 记录。
func (s *STRMService) validateSTRM(record *model.STRMRecord) error {
	if record.Title == "" {
		return errors.New("title is required")
	}
	if record.URL == "" {
		return ErrSTRMURLInvalid
	}
	if !model.IsAllowedProtocol(record.Protocol) {
		return ErrSTRMProtocolInvalid
	}

	// 标准化协议名
	record.Protocol = strings.ToLower(record.Protocol)

	return nil
}

// ListByMediaID 获取关联到指定媒体的 STRM 记录。
func (s *STRMService) ListByMediaID(ctx context.Context, mediaID string) ([]model.STRMRecord, error) {
	return s.repo.STRM.FindByMediaID(ctx, mediaID)
}
