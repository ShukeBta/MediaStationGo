package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type GenerateSTRMOptions struct {
	LibraryID        string `json:"library_id"`
	OutputDir        string `json:"output_dir"`
	BaseURL          string `json:"base_url,omitempty"`
	Enabled          bool   `json:"enabled"`
	Overwrite        bool   `json:"overwrite"`
	IncludeLocal     bool   `json:"include_local"`
	PreserveTree     bool   `json:"preserve_tree"`
	PlaybackToken    string `json:"-"`
	SkipSettingsSave bool   `json:"-"`
}

const strmTreeIgnoredItemSampleLimit = 20

type GenerateSTRMResult struct {
	LibraryID    string             `json:"library_id"`
	OutputDir    string             `json:"output_dir"`
	Generated    int                `json:"generated"`
	Updated      int                `json:"updated"`
	Skipped      int                `json:"skipped"`
	Cleaned      int                `json:"cleaned"`
	Total        int                `json:"total,omitempty"`
	Remaining    int                `json:"remaining,omitempty"`
	BatchLimited bool               `json:"batch_limited,omitempty"`
	Ignored      int                `json:"ignored,omitempty"`
	Previewed    int                `json:"previewed,omitempty"`
	Errors       []string           `json:"errors,omitempty"`
	IgnoredItems []string           `json:"ignored_items,omitempty"`
	Items        []GenerateSTRMItem `json:"items,omitempty"`
	Refresh      *STRMRefreshResult `json:"refresh,omitempty"`
}

type GenerateSTRMItem struct {
	MediaID  string `json:"media_id"`
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
	URL      string `json:"url,omitempty"`
	Action   string `json:"action"`
	Reason   string `json:"reason,omitempty"`
}

func (s *STRMService) GenerateForLibrary(ctx context.Context, opts GenerateSTRMOptions) (*GenerateSTRMResult, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return nil, errors.New("strm service unavailable")
	}
	libraryID := strings.TrimSpace(opts.LibraryID)
	if libraryID == "" {
		return nil, errors.New("library_id required")
	}
	lib, err := s.repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return nil, errors.New("library not found")
	}
	outputDir := s.resolveSTRMOutputDir(ctx, lib, opts)
	if outputDir == "" || outputDir == "." {
		return nil, errors.New("output_dir required")
	}
	s.saveSTRMGenerationSettings(ctx, outputDir, opts)
	if err := os.MkdirAll(outputDir, 0o755); err != nil { // #nosec G301 -- STRM output directories must stay readable by NAS/player users.
		return nil, err
	}

	rows, err := s.librarySTRMMedia(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	res := &GenerateSTRMResult{LibraryID: libraryID, OutputDir: outputDir}
	expectedFiles := map[string]struct{}{}
	for _, media := range rows {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		item := s.generateOne(ctx, *lib, media, outputDir, opts)
		res.addItem(item)
		if item.FilePath != "" && item.Action != "error" {
			expectedFiles[filepath.Clean(item.FilePath)] = struct{}{}
		}
	}
	if opts.Overwrite {
		cleaned, err := s.cleanupStaleGeneratedSTRM(ctx, outputDir, expectedFiles)
		if err != nil {
			res.Errors = append(res.Errors, err.Error())
		}
		res.Cleaned += cleaned
	}
	return res, nil
}

func (s *STRMService) GenerateForAllLibraries(ctx context.Context, opts GenerateSTRMOptions) (*GenerateSTRMResult, error) {
	if s == nil || s.repo == nil || s.repo.Library == nil {
		return nil, errors.New("strm service unavailable")
	}
	libraries, err := s.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	baseOutputDir := s.remapLegacySTRMOutputDir(resolveMappedDestinationPath(strings.TrimSpace(opts.OutputDir)))
	result := &GenerateSTRMResult{LibraryID: "*", OutputDir: baseOutputDir}
	for _, lib := range libraries {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		next := opts
		next.LibraryID = lib.ID
		next.SkipSettingsSave = true
		if baseOutputDir != "" && baseOutputDir != "." {
			next.OutputDir = filepath.Join(baseOutputDir, strmLibraryOutputSubdir(lib))
		}
		part, err := s.GenerateForLibrary(ctx, next)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", lib.Name, err))
			continue
		}
		result.merge(part)
	}
	if baseOutputDir != "" && baseOutputDir != "." && s.repo.Setting != nil {
		_ = s.repo.Setting.Set(ctx, "strm.output_dir", baseOutputDir)
		_ = s.repo.Setting.Set(ctx, "strm.output_scope", "all")
		_ = s.repo.Setting.Set(ctx, "strm.preserve_tree", strconv.FormatBool(opts.PreserveTree))
		result.OutputDir = baseOutputDir
	}
	return result, nil
}

func (s *STRMService) resolveSTRMOutputDir(ctx context.Context, lib *model.Library, opts GenerateSTRMOptions) string {
	outputDir := s.remapLegacySTRMOutputDir(resolveMappedDestinationPath(strings.TrimSpace(opts.OutputDir)))
	if (outputDir == "" || outputDir == ".") && s.repo.Setting != nil {
		if saved, err := s.repo.Setting.Get(ctx, "strm.output_dir"); err == nil {
			outputDir = s.remapLegacySTRMOutputDir(resolveMappedDestinationPath(strings.TrimSpace(saved)))
		}
	}
	if outputDir == "" || outputDir == "." {
		outputDir = s.defaultOutputDir(lib)
	}
	return strmLibrarySpecificOutputDir(outputDir, lib)
}

func (s *STRMService) remapLegacySTRMOutputDir(outputDir string) string {
	outputDir = filepath.Clean(strings.TrimSpace(outputDir))
	if outputDir == "" || outputDir == "." || s == nil || s.cfg == nil {
		return outputDir
	}
	dataDir := filepath.Clean(strings.TrimSpace(s.cfg.App.DataDir))
	if dataDir == "" || dataDir == "." || sameLibraryPath(dataDir, "/app/data") {
		return outputDir
	}
	legacy := "/app/data"
	normalized := strings.TrimRight(cleanPathForVolumeMapping(outputDir), "/")
	if sameLibraryPath(normalized, legacy) {
		return dataDir
	}
	if strings.HasPrefix(strings.ToLower(normalized), legacy+"/") {
		rel := strings.TrimPrefix(normalized[len(legacy):], "/")
		if rel != "" {
			return filepath.Join(dataDir, filepath.FromSlash(rel))
		}
	}
	return outputDir
}

func (s *STRMService) saveSTRMGenerationSettings(ctx context.Context, outputDir string, opts GenerateSTRMOptions) {
	if opts.SkipSettingsSave {
		return
	}
	if strings.TrimSpace(opts.BaseURL) != "" && s.repo.Setting != nil {
		baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
		_ = s.repo.Setting.Set(ctx, "app.server_url", baseURL)
		_ = s.repo.Setting.Set(ctx, "strm.base_url", baseURL)
	}
	if s.repo.Setting == nil {
		return
	}
	_ = s.repo.Setting.Set(ctx, "strm.auto_generate_enabled", strconv.FormatBool(opts.Enabled))
	_ = s.repo.Setting.Set(ctx, "strm.output_dir", outputDir)
	_ = s.repo.Setting.Set(ctx, "strm.output_scope", "library")
	_ = s.repo.Setting.Set(ctx, "strm.preserve_tree", strconv.FormatBool(opts.PreserveTree))
}

func (s *STRMService) librarySTRMMedia(ctx context.Context, libraryID string) ([]model.Media, error) {
	var rows []model.Media
	err := s.repo.DB.WithContext(ctx).
		Where("library_id = ?", libraryID).
		Order("title asc, season_num asc, episode_num asc, created_at asc").
		Find(&rows).Error
	return rows, err
}

func (s *STRMService) defaultOutputDir(lib *model.Library) string {
	subdir := strmLibraryOutputSubdir(*lib)
	if s != nil && s.cfg != nil && strings.TrimSpace(s.cfg.App.DataDir) != "" {
		return filepath.Join(s.cfg.App.DataDir, "strm", subdir)
	}
	return filepath.Join("data", "strm", subdir)
}

func (s *STRMService) generateOne(ctx context.Context, lib model.Library, media model.Media, outputDir string, opts GenerateSTRMOptions) GenerateSTRMItem {
	item := GenerateSTRMItem{MediaID: media.ID, Title: media.Title}
	playURL := s.strmPlaybackURL(ctx, media, opts.BaseURL, opts.PlaybackToken)
	if playURL == "" {
		item.Action = "skipped"
		item.Reason = "no playable strm target"
		return item
	}
	if strings.TrimSpace(media.STRMURL) == "" && !opts.IncludeLocal {
		item.Action = "skipped"
		item.Reason = "local media skipped"
		return item
	}
	rel := s.strmRelativePath(lib, media)
	if opts.PreserveTree {
		if treeRel := s.strmTreeRelativePath(media); treeRel != "" {
			rel = treeRel
		}
	}
	if rel == "" {
		item.Action = "skipped"
		item.Reason = "cannot build file name"
		return item
	}
	filePath := filepath.Join(outputDir, rel)
	item.FilePath = filePath
	item.URL = playURL
	if _, err := os.Stat(filePath); err == nil && !opts.Overwrite {
		item.Action = "skipped"
		item.Reason = "target exists"
		return item
	}
	action := "generated"
	if _, err := os.Stat(filePath); err == nil {
		action = "updated"
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil { // #nosec G301 -- STRM output directories must stay readable by NAS/player users.
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	if err := os.WriteFile(filePath, []byte(playURL+"\n"), 0o644); err != nil { // #nosec G306 -- STRM files are media sidecars intended to be readable by players.
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	if err := s.upsertGeneratedRecord(ctx, media, filePath, playURL, lib.Type); err != nil {
		item.Action = "error"
		item.Reason = err.Error()
		return item
	}
	item.Action = action
	return item
}

func (r *GenerateSTRMResult) addItem(item GenerateSTRMItem) {
	r.Items = append(r.Items, item)
	switch item.Action {
	case "generated":
		r.Generated++
	case "updated":
		r.Updated++
	case "skipped":
		r.Skipped++
	case "preview":
		r.Previewed++
	case "error":
		r.Errors = append(r.Errors, fmt.Sprintf("%s: %s", item.Title, item.Reason))
	}
}

func (r *GenerateSTRMResult) merge(part *GenerateSTRMResult) {
	if part == nil {
		return
	}
	if r.OutputDir == "" || r.OutputDir == "." {
		r.OutputDir = filepath.Dir(part.OutputDir)
	}
	r.Generated += part.Generated
	r.Updated += part.Updated
	r.Skipped += part.Skipped
	r.Cleaned += part.Cleaned
	r.Previewed += part.Previewed
	r.Ignored += part.Ignored
	r.Errors = append(r.Errors, part.Errors...)
	for _, item := range part.IgnoredItems {
		if len(r.IgnoredItems) >= strmTreeIgnoredItemSampleLimit {
			break
		}
		r.IgnoredItems = append(r.IgnoredItems, item)
	}
	r.Items = append(r.Items, part.Items...)
}
