package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// saveMediaProbeDocument 覆盖保存媒体的完整轨道文档。size 用作来源指纹:
// 文件被替换(大小变化)后旧文档在读取时自动失效。
func saveMediaProbeDocument(ctx context.Context, repo *repository.Container, media *model.Media, doc *ProbeDocument) error {
	if repo == nil || repo.DB == nil || media == nil || strings.TrimSpace(media.ID) == "" || !probeDocumentHasTracks(doc) {
		return nil
	}
	raw, err := MarshalProbeDocument(doc)
	if err != nil {
		return err
	}
	row := model.MediaProbeMetadata{
		MediaID:       media.ID,
		ProbeJSON:     raw,
		SchemaVersion: ProbeDocumentSchemaVersion,
		SizeBytes:     media.SizeBytes,
		SourceKey:     mediaProbeSourceKey(media),
		ProbedAt:      time.Now().UTC(),
	}
	return repo.DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "media_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"probe_json", "schema_version", "size_bytes", "source_key", "probed_at"}),
	}).Create(&row).Error
}

// loadMediaProbeDocument 读取与当前媒体文件匹配的轨道文档;不存在、版本过旧
// 或文件大小已变化时返回 nil。
func loadMediaProbeDocument(ctx context.Context, repo *repository.Container, media *model.Media) *ProbeDocument {
	if repo == nil || repo.DB == nil || media == nil || strings.TrimSpace(media.ID) == "" {
		return nil
	}
	var row model.MediaProbeMetadata
	err := repo.DB.WithContext(ctx).Where("media_id = ?", media.ID).Take(&row).Error
	if err != nil {
		return nil
	}
	return decodeStoredProbeDocument(media, row)
}

// loadMediaProbeDocuments 批量读取轨道文档,键为媒体 ID。
func loadMediaProbeDocuments(ctx context.Context, repo *repository.Container, media []model.Media) map[string]*ProbeDocument {
	out := map[string]*ProbeDocument{}
	if repo == nil || repo.DB == nil || len(media) == 0 {
		return out
	}
	byID := make(map[string]*model.Media, len(media))
	ids := make([]string, 0, len(media))
	for i := range media {
		id := strings.TrimSpace(media[i].ID)
		if id == "" {
			continue
		}
		if _, ok := byID[id]; !ok {
			ids = append(ids, id)
		}
		byID[id] = &media[i]
	}
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		var rows []model.MediaProbeMetadata
		if err := repo.DB.WithContext(ctx).Where("media_id IN ?", ids[start:end]).Find(&rows).Error; err != nil {
			return out
		}
		for _, row := range rows {
			if doc := decodeStoredProbeDocument(byID[row.MediaID], row); doc != nil {
				out[row.MediaID] = doc
			}
		}
	}
	return out
}

func decodeStoredProbeDocument(media *model.Media, row model.MediaProbeMetadata) *ProbeDocument {
	if media == nil || row.SourceKey != mediaProbeSourceKey(media) {
		return nil
	}
	if media.SizeBytes > 0 && row.SizeBytes > 0 && media.SizeBytes != row.SizeBytes {
		return nil
	}
	doc, err := UnmarshalProbeDocument(row.ProbeJSON, row.SchemaVersion)
	if err != nil {
		return nil
	}
	return doc
}

func mediaProbeSourceKey(media *model.Media) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", media.Path, media.STRMURL, media.SizeBytes))))
}

// AttachMediaTracks 为单媒体详情响应附加持久化的轨道列表。
func (s *MediaService) AttachMediaTracks(ctx context.Context, media *model.Media) {
	if s == nil || media == nil {
		return
	}
	if doc := loadMediaProbeDocument(ctx, s.repo, media); doc != nil {
		media.Tracks = projectProbeTracks(doc)
	}
}
