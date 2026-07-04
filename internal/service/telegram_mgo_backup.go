package service

import (
	"context"
	"fmt"
	"strings"
)

func (s *TelegramBotService) cmdMgoBackupDB(ctx context.Context) telegramCommandReply {
	if s.backup == nil {
		return telegramCommandReply{Text: "备份服务暂不可用。"}
	}
	info, err := s.backup.Create(ctx)
	if err != nil {
		return telegramCommandReply{Text: "数据库备份失败：" + err.Error()}
	}
	return telegramCommandReply{Text: fmt.Sprintf("数据库备份完成：<code>%s</code>\n大小：<b>%d</b> bytes", info.Filename, info.Size)}
}

func (s *TelegramBotService) cmdMgoRestoreDB(ctx context.Context, args []string) telegramCommandReply {
	if s.backup == nil {
		return telegramCommandReply{Text: "备份服务暂不可用。"}
	}
	if len(args) == 0 || strings.EqualFold(args[0], "list") {
		items, err := s.backup.List()
		if err != nil {
			return telegramCommandReply{Text: "读取备份列表失败：" + err.Error()}
		}
		if len(items) == 0 {
			return telegramCommandReply{Text: "暂无数据库备份。可先使用 <code>/backup_db</code> 创建。"}
		}
		lines := make([]string, 0, minInt(len(items), 10))
		for i, item := range items {
			if i >= 10 {
				break
			}
			lines = append(lines, fmt.Sprintf("%s（%d bytes）", item.Filename, item.Size))
		}
		return telegramCommandReply{Text: "可恢复备份：\n" + telegramInlineCodeList(lines) + "\n\n恢复需要确认：<code>/restore_from_db 文件名 confirm</code>"}
	}
	if len(args) < 2 || !strings.EqualFold(args[len(args)-1], "confirm") {
		return telegramCommandReply{Text: "恢复数据库会覆盖当前数据，需要确认：<code>/restore_from_db 文件名 confirm</code>"}
	}
	filename := strings.TrimSpace(args[0])
	if err := s.backup.Restore(ctx, filename); err != nil {
		return telegramCommandReply{Text: "恢复失败：" + err.Error()}
	}
	return telegramCommandReply{Text: "数据库已从备份恢复，请重启 MediaStationGo 后生效。"}
}
