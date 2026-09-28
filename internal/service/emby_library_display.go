package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const EmbyLibraryDisplaySettingKey = "emby.library_display"

// EmbyLibraryDisplay 按数组顺序配置媒体库入口，不改变媒体访问权限。
type EmbyLibraryDisplay struct {
	ID     string `json:"id"`
	Hidden bool   `json:"hidden"`
}

// DecodeEmbyLibraryDisplay 校验设置格式；空值用于恢复默认展示。
func DecodeEmbyLibraryDisplay(value string) ([]EmbyLibraryDisplay, error) {
	if value == "" {
		return nil, nil
	}
	var input []struct {
		ID     string `json:"id"`
		Hidden *bool  `json:"hidden"`
	}
	if err := json.Unmarshal([]byte(value), &input); err != nil || input == nil {
		return nil, errors.New("媒体库展示设置必须是数组")
	}
	entries := make([]EmbyLibraryDisplay, 0, len(input))
	seen := make(map[string]bool, len(input))
	for _, entry := range input {
		entry.ID = strings.TrimSpace(entry.ID)
		if entry.Hidden == nil {
			return nil, errors.New("媒体库显隐状态必须是布尔值")
		}
		if strings.TrimSpace(entry.ID) == "" || seen[entry.ID] {
			return nil, errors.New("媒体库标识不能为空或重复")
		}
		seen[entry.ID] = true
		entries = append(entries, EmbyLibraryDisplay{ID: entry.ID, Hidden: *entry.Hidden})
	}
	return entries, nil
}

// DisplayLibraries 返回统一的 Emby 展示顺序；调用方仍需过滤用户权限。
func (e *EmbyService) DisplayLibraries(ctx context.Context) ([]model.Library, error) {
	libs, err := e.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	value, err := e.repo.Setting.Get(ctx, EmbyLibraryDisplaySettingKey)
	if err != nil {
		return nil, err
	}
	entries, err := DecodeEmbyLibraryDisplay(value)
	if err != nil {
		return nil, err
	}
	// Resolve the existing merged entrance before hiding it. Otherwise a
	// shadowed cloud library can reappear as soon as its local entrance is hidden.
	libs = FilterDisplayCloudLibraries(ctx, e.repo, libs)
	return applyEmbyLibraryDisplay(libs, entries), nil
}

func applyEmbyLibraryDisplay(libs []model.Library, entries []EmbyLibraryDisplay) []model.Library {
	byID := make(map[string]model.Library, len(libs))
	for _, lib := range libs {
		byID[lib.ID] = lib
	}
	out := make([]model.Library, 0, len(libs))
	for _, entry := range entries {
		if lib, ok := byID[entry.ID]; ok && !entry.Hidden {
			out = append(out, lib)
		}
		delete(byID, entry.ID)
	}
	for _, lib := range libs {
		if _, ok := byID[lib.ID]; ok {
			out = append(out, lib)
		}
	}
	return out
}
