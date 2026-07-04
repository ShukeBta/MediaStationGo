package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) cmdMgoCreateUser(ctx context.Context, args []string) telegramCommandReply {
	if len(args) < 2 {
		return telegramCommandReply{Text: "用法：<code>/ucr 用户名 密码 [天数]</code>，天数 0 表示永久。"}
	}
	if s.auth == nil {
		return telegramCommandReply{Text: "注册服务暂不可用。"}
	}
	user, _, err := s.auth.Register(ctx, args[0], args[1])
	if err != nil {
		return telegramCommandReply{Text: "创建失败：" + err.Error()}
	}
	days := 0
	if len(args) >= 3 {
		parsed, err := strconv.Atoi(args[2])
		if err != nil || parsed < 0 {
			return telegramCommandReply{Text: "账号已创建，但天数无效。请用 <code>/renew 用户名 天数</code> 调整。"}
		}
		days = parsed
		if err := s.applyRenewal(ctx, user.ID, days); err != nil {
			return telegramCommandReply{Text: "账号已创建，但续期失败：" + err.Error()}
		}
	}
	return telegramCommandReply{Text: fmt.Sprintf("已创建用户：<b>%s</b>\n到期：<b>%s</b>", user.Username, formatExpiry(s.userExpiry(ctx, user.ID)))}
}

func (s *TelegramBotService) cmdMgoUserInfo(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: "用法：<code>/uinfo 用户名</code>"}
	}
	user := s.findMgoBotUser(ctx, args[0])
	if user == nil {
		return telegramCommandReply{Text: "未找到用户。"}
	}
	s.applyRealtimeUserActivity(ctx, user)
	devices, _ := s.listUserDevices(ctx, user.ID)
	var historyCount int64
	_ = s.repo.DB.WithContext(ctx).Model(&model.PlaybackHistory{}).Where("user_id = ?", user.ID).Count(&historyCount).Error
	var binding model.TelegramBinding
	tg := "未绑定"
	if err := s.repo.DB.WithContext(ctx).Where("user_id = ?", user.ID).First(&binding).Error; err == nil {
		tg = fmt.Sprintf("tg:%d", binding.TelegramUserID)
		if binding.TelegramName != "" {
			tg += " " + binding.TelegramName
		}
	}
	return telegramCommandReply{Text: fmt.Sprintf(
		"<b>用户信息</b>\n\n用户名：<b>%s</b>\n角色：<b>%s</b>\n状态：<b>%s</b>\n到期：<b>%s</b>\nTelegram：<b>%s</b>\n设备：<b>%d</b>\n播放记录：<b>%d</b>\n最后登录：<b>%s</b>",
		user.Username, user.Role, activeLabel(user), formatExpiry(user.ExpiredAt), tg, len(devices), historyCount, formatOptionalTime(user.LastLoginAt),
	)}
}

func (s *TelegramBotService) applyRealtimeUserActivity(ctx context.Context, user *model.User) {
	if s == nil || user == nil || s.device == nil || s.device.sessions == nil {
		return
	}
	users := []model.User{*user}
	s.device.sessions.ApplyToUsers(ctx, users)
	*user = users[0]
}

func (s *TelegramBotService) cmdMgoDeleteUser(ctx context.Context, args []string) telegramCommandReply {
	if len(args) < 2 || !strings.EqualFold(args[len(args)-1], "confirm") {
		return telegramCommandReply{Text: "删除用户需要确认：<code>/rmemby 用户名 confirm</code> 或 <code>/urm 用户名 confirm</code>"}
	}
	user := s.findMgoBotUser(ctx, args[0])
	if user == nil {
		return telegramCommandReply{Text: "未找到用户。"}
	}
	if reason := s.protectReason(ctx, user.ID); reason != "" {
		return telegramCommandReply{Text: reason}
	}
	_ = s.repo.UserDevice.DeleteByUser(ctx, user.ID)
	if err := s.repo.User.Delete(ctx, user.ID); err != nil {
		return telegramCommandReply{Text: "删除失败：" + err.Error()}
	}
	return telegramCommandReply{Text: fmt.Sprintf("已删除用户 <b>%s</b>。", user.Username)}
}

func (s *TelegramBotService) cmdMgoOnlyRemoveRecord(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: "用法：<code>/only_rm_record tg:123456</code> 或 <code>/only_rm_record 用户名</code>，只删除 Telegram 绑定记录。"}
	}
	target := strings.TrimSpace(args[0])
	var removed int64
	if raw, ok := strings.CutPrefix(strings.ToLower(target), "tg:"); ok {
		tgID, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || tgID == 0 {
			return telegramCommandReply{Text: "Telegram ID 无效。"}
		}
		removed, err = s.deleteTelegramBindings(ctx, "telegram_user_id = ?", tgID)
		if err != nil {
			return telegramCommandReply{Text: "删除绑定失败：" + err.Error()}
		}
	} else {
		user := s.findMgoBotUser(ctx, target)
		if user == nil {
			return telegramCommandReply{Text: "未找到用户。"}
		}
		n, err := s.deleteTelegramBindings(ctx, "user_id = ?", user.ID)
		if err != nil {
			return telegramCommandReply{Text: "删除绑定失败：" + err.Error()}
		}
		removed = n
	}
	return telegramCommandReply{Text: fmt.Sprintf("已删除 Telegram 绑定记录：<b>%d</b> 条。", removed)}
}

func (s *TelegramBotService) cmdMgoUserIP(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: "用法：<code>/userip 用户名</code>"}
	}
	user := s.findMgoBotUser(ctx, args[0])
	if user == nil {
		return telegramCommandReply{Text: "未找到用户。"}
	}
	devices, err := s.listUserDevices(ctx, user.ID)
	if err != nil {
		return telegramCommandReply{Text: "查询失败：" + err.Error()}
	}
	if len(devices) == 0 {
		return telegramCommandReply{Text: "该用户暂无设备/IP记录。"}
	}
	var out []string
	for i, d := range devices {
		if i >= 20 {
			break
		}
		out = append(out, fmt.Sprintf("%d. %s / %s / %s / %s", i+1, blankDash(d.LastIP), blankDash(d.DeviceName), blankDash(d.Client), d.LastSeenAt.Format("2006-01-02 15:04")))
	}
	return telegramCommandReply{Text: "<b>" + user.Username + " 的设备/IP</b>\n\n<code>" + strings.Join(out, "\n") + "</code>"}
}

func (s *TelegramBotService) cmdMgoAuditDevices(ctx context.Context, mode string, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: fmt.Sprintf("用法：<code>/%s 关键词</code>", mode)}
	}
	keyword := strings.TrimSpace(strings.Join(args, " "))
	var rows []struct {
		Username   string
		DeviceID   string
		DeviceName string
		Client     string
		LastIP     string
		LastSeenAt time.Time
	}
	q := s.repo.DB.WithContext(ctx).Table("user_devices").
		Select("users.username, user_devices.device_id, user_devices.device_name, user_devices.client, user_devices.last_ip, user_devices.last_seen_at").
		Joins("JOIN users ON users.id = user_devices.user_id").
		Order("user_devices.last_seen_at desc").
		Limit(20)
	switch mode {
	case "auditip":
		q = q.Where("user_devices.last_ip LIKE ?", "%"+keyword+"%")
	case "auditdevice":
		q = q.Where("user_devices.device_name LIKE ? OR user_devices.device_id LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	case "auditclient":
		q = q.Where("user_devices.client LIKE ?", "%"+keyword+"%")
	case "udeviceid":
		q = q.Where("user_devices.device_id LIKE ?", "%"+keyword+"%")
	}
	if err := q.Scan(&rows).Error; err != nil {
		return telegramCommandReply{Text: "查询失败：" + err.Error()}
	}
	if len(rows) == 0 {
		return telegramCommandReply{Text: "没有匹配记录。"}
	}
	var out []string
	for i, r := range rows {
		out = append(out, fmt.Sprintf("%d. %s / %s / %s / %s / %s", i+1, r.Username, blankDash(r.LastIP), blankDash(r.DeviceName), blankDash(r.Client), r.LastSeenAt.Format("2006-01-02 15:04")))
	}
	return telegramCommandReply{Text: "<b>审计结果</b>\n\n<code>" + strings.Join(out, "\n") + "</code>"}
}
