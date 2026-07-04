package service

import (
	"context"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *STRMService) upsertGeneratedRecord(ctx context.Context, media model.Media, filePath, playURL, mediaType string) error {
	protocol := ""
	if u, err := url.Parse(playURL); err == nil {
		protocol = strings.ToLower(u.Scheme)
	}
	if protocol == "" {
		protocol = "http"
	}
	record := model.STRMRecord{
		Title:      media.Title,
		URL:        playURL,
		FilePath:   filePath,
		Protocol:   protocol,
		MediaID:    media.ID,
		MediaType:  mediaType,
		SeasonNum:  media.SeasonNum,
		EpisodeNum: media.EpisodeNum,
	}
	var existing model.STRMRecord
	err := s.repo.DB.WithContext(ctx).Where("media_id = ? AND file_path = ?", media.ID, filePath).First(&existing).Error
	if err == nil {
		existing.Title = record.Title
		existing.URL = record.URL
		existing.Protocol = record.Protocol
		existing.MediaType = record.MediaType
		existing.SeasonNum = record.SeasonNum
		existing.EpisodeNum = record.EpisodeNum
		return s.repo.DB.WithContext(ctx).Save(&existing).Error
	}
	return s.repo.DB.WithContext(ctx).Create(&record).Error
}

func (s *STRMService) cleanupStaleGeneratedSTRM(ctx context.Context, outputDir string, expected map[string]struct{}) (int, error) {
	outputDir = filepath.Clean(strings.TrimSpace(outputDir))
	if outputDir == "" || outputDir == "." {
		return 0, nil
	}
	cleaned, err := removeStaleSTRMFiles(outputDir, expected)
	if err != nil {
		return cleaned, err
	}
	recordsCleaned, err := s.removeStaleSTRMRecords(ctx, outputDir, expected)
	return cleaned + recordsCleaned, err
}

func removeStaleSTRMFiles(outputDir string, expected map[string]struct{}) (int, error) {
	cleaned := 0
	err := filepath.WalkDir(outputDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if entry.IsDir() || strings.ToLower(filepath.Ext(path)) != ".strm" {
			return nil
		}
		cleanPath := filepath.Clean(path)
		if _, ok := expected[cleanPath]; ok {
			return nil
		}
		if err := os.Remove(cleanPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		cleaned++
		return nil
	})
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return cleaned, err
	}
	return cleaned, nil
}

func (s *STRMService) removeStaleSTRMRecords(ctx context.Context, outputDir string, expected map[string]struct{}) (int, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return 0, nil
	}
	var records []model.STRMRecord
	if err := s.repo.DB.WithContext(ctx).Find(&records).Error; err != nil {
		return 0, err
	}
	rootAbs, err := filepath.Abs(outputDir)
	if err != nil {
		return 0, nil
	}
	cleaned := 0
	for i := range records {
		filePath := filepath.Clean(strings.TrimSpace(records[i].FilePath))
		if filePath == "" {
			continue
		}
		fileAbs, err := filepath.Abs(filePath)
		if err != nil || !pathWithin(fileAbs, rootAbs) {
			continue
		}
		if _, ok := expected[filePath]; ok {
			continue
		}
		if err := s.repo.DB.WithContext(ctx).Delete(&records[i]).Error; err != nil {
			return cleaned, err
		}
		cleaned++
	}
	return cleaned, nil
}
