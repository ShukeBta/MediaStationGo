package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// onTorrentComplete handles a torrent that just finished downloading.
// It organizes the completed torrent payload directly. Relying on existing
// Media rows is too late for freshly-downloaded files: they usually have not
// been scanned into the library yet.
func (d *DownloadService) onTorrentComplete(ctx context.Context, torrent QBitTorrent) {
	taskRow, hasTask := d.completedTorrentTask(ctx, torrent)
	d.notifyDownloadComplete(ctx, torrent, taskRow)
	if d.organizer == nil {
		return
	}
	// 仅当显式开启 organizer.auto_after_download / organize.auto 时才在下载完成后整理。
	// 之前的代码错误地把 organizer.smart_classify 也当成"自动整理"开关，
	// 让操作员只想启用"分类子目录"就被动触发了文件 move。
	autoOrganize := d.downloadAutoOrganizeEnabled(ctx)
	if !autoOrganize {
		d.log.Info("download completed, auto-organize disabled", zap.String("hash", torrent.Hash))
		return
	}
	source := d.completedTorrentSource(ctx, torrent)
	if source == "" {
		d.log.Warn("download completed but payload path is not accessible",
			zap.String("hash", torrent.Hash),
			zap.String("name", torrent.Name),
			zap.String("save_path", torrent.SavePath),
			zap.String("content_path", torrent.ContentPath))
		return
	}
	allowReplace := hasTask && taskRow.AllowExistingLibrary
	d.runCompletedTorrentOrganize(ctx, torrent, taskRow, source, allowReplace)
}

func (d *DownloadService) runCompletedTorrentOrganize(ctx context.Context, torrent QBitTorrent, task *model.DownloadTask, source string, allowReplace bool) {
	d.log.Info("download completed, triggering directory organize",
		zap.String("hash", torrent.Hash),
		zap.String("name", torrent.Name),
		zap.String("source", source),
		zap.Bool("allow_replace_existing", allowReplace))
	resWrap, err := d.ensureOrganizePipeline().Run(ctx, OrganizePipelineRequest{
		Scope:         OrganizeScopeDirectory,
		Trigger:       OrganizeTriggerDownload,
		TaskName:      d.downloadOrganizeTaskName(torrent, allowReplace),
		SourcePath:    source,
		MediaType:     downloadTaskMediaType(task),
		MediaCategory: firstNonEmpty(downloadTaskMediaCategory(task), torrent.Category),
		AllowReplace:  allowReplace,
	})
	if err != nil {
		if errors.Is(err, ErrUnsupportedOrganizeSource) {
			d.markCompletedTorrentCatchupRecorded(context.Background(), torrent)
			d.log.Warn("auto organize skipped unsupported completed torrent",
				zap.String("hash", torrent.Hash),
				zap.String("source", source),
				zap.Error(err))
			return
		}
		d.log.Error("auto organize completed torrent failed",
			zap.String("hash", torrent.Hash),
			zap.String("source", source),
			zap.Error(err))
		return
	}
	res := resWrap.Result
	if res == nil {
		res = &OrganizeResult{}
	}
	d.markCompletedTorrentCatchupRecorded(context.Background(), torrent)
	d.log.Info("auto organize completed torrent finished",
		zap.String("hash", torrent.Hash),
		zap.String("source", source),
		zap.String("dest", firstNonEmpty(res.DestPath, "")),
		zap.Int("organized", res.Organized),
		zap.Int("replaced", res.Replaced),
		zap.Int("skipped", res.Skipped),
		zap.Int("scrapes", len(res.Scrapes)),
		zap.Int("errors", len(res.Errors)))
}

func (d *DownloadService) downloadOrganizeTaskName(torrent QBitTorrent, allowReplace bool) string {
	name := strings.TrimSpace(torrent.Name)
	if name == "" {
		name = "下载完成自动整理"
	}
	if allowReplace {
		name += "（允许洗版）"
	}
	return name
}

func (d *DownloadService) ensureOrganizePipeline() *OrganizePipelineService {
	if d.organizePipeline != nil {
		return d.organizePipeline
	}
	return NewOrganizePipelineService(d.log, d.repo, d.organizer, d.scanner, d.tasks)
}

func (d *DownloadService) completedTorrentTask(ctx context.Context, torrent QBitTorrent) (*model.DownloadTask, bool) {
	if d == nil || d.repo == nil || d.repo.Download == nil {
		return nil, false
	}
	rows, err := d.repo.Download.List(ctx)
	if err != nil || len(rows) == 0 {
		return nil, false
	}
	taskByKey := tasksByTorrentIdentity(rows)
	if task, ok := findMatchingTaskByTorrentIdentity(torrent.Name, taskByKey); ok {
		return &task, true
	}
	if strings.TrimSpace(torrent.ContentPath) != "" {
		if task, ok := findMatchingTaskByTorrentIdentity(filepath.Base(torrent.ContentPath), taskByKey); ok {
			return &task, true
		}
	}
	return nil, false
}

func downloadTaskMediaType(task *model.DownloadTask) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.MediaType)
}

func downloadTaskMediaCategory(task *model.DownloadTask) string {
	if task == nil {
		return ""
	}
	return strings.TrimSpace(task.MediaCategory)
}

// DownloadPathMappingsSettingKey 允许用户自定义「下载器路径 → 本程序路径」
// 映射，每行一条，格式 `客户端路径=本地路径`（也接受 `=>` 或单个 `:` 分隔）。
// qBittorrent 与本程序常在不同容器/主机里，对同一份数据看到的路径不同；
// 此前映射表是写死的三条猜测，对不上时整理静默失败。
const DownloadPathMappingsSettingKey = "download.path_mappings"

func (d *DownloadService) completedTorrentSource(ctx context.Context, torrent QBitTorrent) string {
	mappings := d.downloadPathMappings(ctx)
	for _, candidate := range []string{
		torrent.ContentPath,
		filepath.Join(torrent.SavePath, torrent.Name),
	} {
		clean := strings.TrimSpace(candidate)
		if clean == "" || clean == "." {
			continue
		}
		// 尝试直接访问或路径映射
		if translated := translateClientPath(clean, mappings); translated != "" {
			return translated
		}
		// 复用 compose 注入的 MEDIASTATION_DOWNLOAD_DIR/MEDIA_DIR 宿主机↔容器
		// 映射（与媒体库路径换算同一套规则），覆盖「qB 跑在宿主机、
		// 本程序在容器里」的最常见部署形态。
		for _, mapped := range mappedPathCandidates(clean) {
			if mapped == clean {
				continue
			}
			if _, err := os.Stat(mapped); err == nil {
				return mapped
			}
		}
	}
	return ""
}

// userPathMappings 解析用户配置的下载器路径映射。
func (d *DownloadService) userPathMappings(ctx context.Context) map[string]string {
	out := map[string]string{}
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return out
	}
	raw, err := d.repo.Setting.Get(ctx, DownloadPathMappingsSettingKey)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var from, to string
		switch {
		case strings.Contains(line, "=>"):
			parts := strings.SplitN(line, "=>", 2)
			from, to = parts[0], parts[1]
		case strings.Contains(line, "="):
			parts := strings.SplitN(line, "=", 2)
			from, to = parts[0], parts[1]
		case strings.Count(line, ":") == 1:
			parts := strings.SplitN(line, ":", 2)
			from, to = parts[0], parts[1]
		default:
			continue
		}
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		if from != "" && to != "" {
			out[from] = to
		}
	}
	return out
}
