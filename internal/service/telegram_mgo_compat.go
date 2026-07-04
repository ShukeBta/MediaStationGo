package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) cmdMgoUnsupported(name, replacement string) telegramCommandReply {
	text := fmt.Sprintf("<b>%s</b> 已识别，但当前 Telegram Bot API 无法完整复刻该行为。", name)
	if replacement != "" {
		text += "\n请使用：" + replacement
	}
	return telegramCommandReply{Text: text}
}

func (s *TelegramBotService) findMgoBotUser(ctx context.Context, target string) *model.User {
	target = strings.TrimSpace(strings.TrimPrefix(target, "@"))
	if target == "" {
		return nil
	}
	if user, _ := s.repo.User.FindByUsername(ctx, target); user != nil {
		return user
	}
	if user, _ := s.repo.User.FindByID(ctx, target); user != nil {
		return user
	}
	if tgRaw, ok := strings.CutPrefix(strings.ToLower(target), "tg:"); ok {
		if tgID, err := strconv.ParseInt(tgRaw, 10, 64); err == nil {
			var binding model.TelegramBinding
			if err := s.repo.DB.WithContext(ctx).Where("telegram_user_id = ?", tgID).First(&binding).Error; err == nil {
				user, _ := s.repo.User.FindByID(ctx, binding.UserID)
				return user
			}
		}
	}
	return nil
}

func activeLabel(user *model.User) string {
	if user == nil {
		return "未知"
	}
	if !user.IsActive {
		return "已禁用"
	}
	if user.ExpiredAt != nil && time.Now().After(*user.ExpiredAt) {
		return "已过期"
	}
	return "正常"
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format("2006-01-02 15:04")
}

func blankDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func telegramInlineCodeList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return "<code>" + strings.Join(items, "</code>、<code>") + "</code>"
}

func parseOnOff(raw string) *bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "on", "true", "1", "yes", "enable", "enabled", "开启", "开":
		v := true
		return &v
	case "off", "false", "0", "no", "disable", "disabled", "关闭", "关":
		v := false
		return &v
	default:
		return nil
	}
}

func humanDurationFromMillis(ms int64) string {
	if ms <= 0 {
		return "0 分钟"
	}
	totalMinutes := ms / 1000 / 60
	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	if hours == 0 {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	return fmt.Sprintf("%d 小时 %d 分钟", hours, minutes)
}
