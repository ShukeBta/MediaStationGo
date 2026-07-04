package service

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// cmdStatus 处理 /status 命令。
func (s *TelegramBotService) cmdStatus(ctx context.Context) (telegramCommandReply, error) {
	libraryIDs, err := s.activeTelegramStatsLibraryIDs(ctx)
	if err != nil {
		return telegramCommandReply{}, err
	}
	var mediaCount int64
	s.mediaStatsQuery(libraryIDs).Count(&mediaCount)

	var totalSize int64
	if err := s.mediaStatsQuery(libraryIDs).Select("COALESCE(SUM(size_bytes), 0)").Row().Scan(&totalSize); err != nil {
		return telegramCommandReply{}, err
	}
	totalSizeGB := float64(totalSize) / 1024 / 1024 / 1024

	return telegramCommandReply{Text: fmt.Sprintf(
		"<b>系统运行状态</b>\n\n"+
			"🎬 媒体总数: <b>%d</b>\n"+
			"💾 存储占用: <b>%.1f GB</b>",
		mediaCount, totalSizeGB,
	)}, nil
}

// cmdSearch 处理 /search 命令。
func (s *TelegramBotService) cmdSearch(ctx context.Context, args []string) (telegramCommandReply, error) {
	if len(args) == 0 {
		return telegramCommandReply{Text: "请提供搜索关键词\n例: <code>/search 哥斯拉</code>"}, nil
	}

	keyword := strings.Join(args, " ")
	var results []model.Media
	err := s.repo.DB.Where("title LIKE ?", "%"+keyword+"%").
		Order("year DESC").Limit(8).
		Find(&results).Error
	if err != nil {
		return telegramCommandReply{}, err
	}

	if len(results) == 0 {
		return telegramCommandReply{Text: fmt.Sprintf("未找到与 <b>%s</b> 相关的媒体", keyword)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>搜索: %s</b>\n\n", keyword))
	for i, m := range results {
		year := ""
		if m.Year > 0 {
			year = fmt.Sprintf(" (%d)", m.Year)
		}
		ep := ""
		if m.SeasonNum > 0 && m.EpisodeNum > 0 {
			ep = fmt.Sprintf(" S%02dE%02d", m.SeasonNum, m.EpisodeNum)
		}
		sb.WriteString(fmt.Sprintf("%d. <b>%s</b>%s%s — %s\n", i+1, m.Title, year, ep, formatSize(m.SizeBytes)))
	}

	return telegramCommandReply{Text: sb.String()}, nil
}

// cmdDownloads 处理 /downloads 命令。
func (s *TelegramBotService) cmdDownloads(ctx context.Context) (telegramCommandReply, error) {
	type Row struct {
		Title  string
		Status string
	}
	var rows []Row
	if err := s.repo.DB.Raw(
		"SELECT COALESCE(NULLIF(title,''),'下载任务') as title, COALESCE(status,'unknown') as status FROM download_tasks ORDER BY created_at DESC LIMIT 8",
	).Scan(&rows).Error; err != nil {
		return telegramCommandReply{}, err
	}

	if len(rows) == 0 {
		return telegramCommandReply{Text: "当前没有下载任务。"}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<b>下载任务 (%d)</b>\n\n", len(rows)))
	for _, r := range rows {
		icon := "⏳"
		switch r.Status {
		case "completed":
			icon = "✅"
		case "downloading":
			icon = "📥"
		case "error":
			icon = "❌"
		}
		name := strings.TrimSpace(r.Title)
		if name == "" {
			name = "下载任务"
		}
		if len(name) > 60 {
			name = name[:57] + "..."
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", icon, name))
	}

	return telegramCommandReply{Text: sb.String()}, nil
}

// cmdStats 处理 /stats 命令。
func (s *TelegramBotService) cmdStats(ctx context.Context) (telegramCommandReply, error) {
	libs, err := s.activeTelegramStatsLibraries(ctx)
	if err != nil {
		return telegramCommandReply{}, err
	}
	libraryIDs := make([]string, 0, len(libs))
	for _, lib := range libs {
		libraryIDs = append(libraryIDs, lib.ID)
	}
	var totalMedia int64
	s.mediaStatsQuery(libraryIDs).Count(&totalMedia)

	var totalSize int64
	if err := s.mediaStatsQuery(libraryIDs).Select("COALESCE(SUM(size_bytes), 0)").Row().Scan(&totalSize); err != nil {
		return telegramCommandReply{}, err
	}

	type LibStat struct {
		Name  string
		Type  string
		Count int64
	}
	stats := make([]LibStat, 0, len(libs))
	for _, lib := range libs {
		var count int64
		if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("library_id = ?", lib.ID).Count(&count).Error; err != nil {
			return telegramCommandReply{}, err
		}
		stats = append(stats, LibStat{Name: lib.Name, Type: lib.Type, Count: count})
	}

	var sb strings.Builder
	sb.WriteString("<b>媒体库统计</b>\n\n")
	sb.WriteString(fmt.Sprintf("📚 总数: <b>%d</b>\n", totalMedia))
	sb.WriteString(fmt.Sprintf("💾 大小: <b>%s</b>\n", formatSize(totalSize)))

	if len(stats) > 0 {
		sb.WriteString("\n<b>各库分布:</b>\n")
		for _, l := range stats {
			icon := "🎬"
			switch l.Type {
			case "tv":
				icon = "📺"
			case "anime":
				icon = "🍥"
			case "music":
				icon = "🎵"
			}
			sb.WriteString(fmt.Sprintf("%s <b>%s</b>: %d\n", icon, l.Name, l.Count))
		}
	}

	return telegramCommandReply{Text: sb.String()}, nil
}

func (s *TelegramBotService) activeTelegramStatsLibraries(ctx context.Context) ([]model.Library, error) {
	if s == nil || s.repo == nil || s.repo.Library == nil {
		return nil, nil
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	libs = FilterDisplayCloudLibraries(ctx, s.repo, libs)
	out := libs[:0]
	for _, lib := range libs {
		if lib.Enabled {
			out = append(out, lib)
		}
	}
	return out, nil
}

func (s *TelegramBotService) activeTelegramStatsLibraryIDs(ctx context.Context) ([]string, error) {
	libs, err := s.activeTelegramStatsLibraries(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(libs))
	for _, lib := range libs {
		ids = append(ids, lib.ID)
	}
	return ids, nil
}

func (s *TelegramBotService) mediaStatsQuery(libraryIDs []string) *gorm.DB {
	q := s.repo.DB.Model(&model.Media{})
	if len(libraryIDs) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where("library_id IN ?", libraryIDs)
}

// formatSize 格式化字节数为可读字符串。
func formatSize(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := float64(bytes)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%.0f %s", v, units[i])
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
