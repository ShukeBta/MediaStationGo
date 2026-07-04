package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// resolveBaseRoot picks the organize destination root (目的地目录): a
// per-request override wins, then the organize.target_dir setting, then the
// library's own path.
func (o *OrganizerService) resolveBaseRoot(ctx context.Context, lib *model.Library, override string) string {
	if r := strings.TrimSpace(override); r != "" {
		return r
	}
	if o.repo != nil && o.repo.Setting != nil {
		if v, err := o.repo.Setting.Get(ctx, "organize.target_dir"); err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return lib.Path
}

// resolveSourceRoot picks the organize source root (源目录，待整理文件所在目录):
// a per-request override wins, then the organize.source_dir setting, then the
// library's own path. Library organize only touches media located under this
// root, so operators can point at a specific download/staging folder.
func (o *OrganizerService) resolveSourceRoot(ctx context.Context, lib *model.Library, override string) string {
	if r := strings.TrimSpace(override); r != "" {
		return r
	}
	if o.repo != nil && o.repo.Setting != nil {
		if v, err := o.repo.Setting.Get(ctx, "organize.source_dir"); err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return lib.Path
}

// resolveTransferMode picks the transfer mode: a per-request override wins,
// otherwise the organize.transfer_mode setting (default move). When the
// effective mode is move and 做种保种 (organize.keep_seeding) is enabled, it is
// upgraded to hardlink so the source stays in place for the torrent client.
func (o *OrganizerService) resolveTransferMode(ctx context.Context, override TransferMode) TransferMode {
	mode := override
	if mode == "" {
		mode = TransferMove
		if o.repo != nil && o.repo.Setting != nil {
			if v, err := o.repo.Setting.Get(ctx, "organize.transfer_mode"); err == nil && strings.TrimSpace(v) != "" {
				mode = parseTransferMode(v)
			}
		}
	}
	if mode == TransferMove && o.keepSeedingEnabled(ctx) {
		// 移动会删除源文件导致 qBittorrent 停止做种；保种开启时改用硬链接
		// 既规范命名又保留源文件继续做种上传。硬链接失败时会报错，避免静默
		// 退化复制后占用双份磁盘空间。
		return TransferHardlink
	}
	return mode
}

// keepSeedingEnabled reports whether 做种保种 is on. Defaults to true so an
// unconfigured instance never silently breaks seeding on organize.
func (o *OrganizerService) keepSeedingEnabled(ctx context.Context) bool {
	if o.repo == nil || o.repo.Setting == nil {
		return true
	}
	v, err := o.repo.Setting.Get(ctx, "organize.keep_seeding")
	if err != nil || strings.TrimSpace(v) == "" {
		return true
	}
	return v == "true" || v == "1" || v == "on"
}

func (o *OrganizerService) autoAddLibraryEnabled(ctx context.Context) bool {
	if o == nil || o.repo == nil || o.repo.Setting == nil {
		return true
	}
	v, err := o.repo.Setting.Get(ctx, "organize.auto_add_library")
	if err != nil || strings.TrimSpace(v) == "" {
		return true
	}
	return parseBoolSetting(v, true)
}

func (o *OrganizerService) effectiveOrganizeOverrides(opts OrganizeOptions, explicitDest string) (string, string) {
	mediaType := normalizeOrganizeMediaType(opts.MediaType)
	category := sanitizeFilename(strings.TrimSpace(opts.MediaCategory))
	hasExplicitLayout := mediaType != "" || category != ""
	if category != "" {
		if impliedType, normalizedCategory := o.mediaTypeForDirectoryCategory(category); impliedType != "" {
			category = normalizedCategory
			if mediaType == "" {
				mediaType = impliedType
			}
		}
		return mediaType, category
	}
	if hasExplicitLayout {
		if layout := o.organizeLayoutFromDestPath(explicitDest); layout.Category != "" {
			category = sanitizeFilename(layout.Category)
			if mediaType == "" {
				mediaType = layout.MediaType
			}
		}
	}
	return mediaType, category
}

func (o *OrganizerService) organizeLayoutFromDestPath(dest string) organizeDirectoryLayout {
	dest = filepath.Clean(strings.TrimSpace(dest))
	if dest == "" || dest == "." {
		return organizeDirectoryLayout{}
	}
	if mediaType, category := o.mediaTypeForDirectoryCategory(filepath.Base(dest)); mediaType != "" && category != "" {
		return organizeDirectoryLayout{MediaType: mediaType, Category: category}
	}
	return organizeDirectoryLayout{}
}
