package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) cmdKick(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号：<code>/start 用户名 密码</code>"}
	}
	if len(args) == 0 {
		return telegramCommandReply{Text: "请指定要踢下线的设备：<code>/kick all</code> 或 <code>/kick 设备编号</code>。先用 <code>/devices</code> 查看编号。"}
	}
	target := strings.TrimSpace(args[0])
	if strings.EqualFold(target, "all") || target == "全部" {
		if s.device != nil {
			if err := s.device.KickAllDevices(ctx, user.ID); err != nil {
				return telegramCommandReply{Text: "踢下线失败：" + err.Error()}
			}
		} else if err := s.repo.UserDevice.SetKickedByUser(ctx, user.ID, true); err != nil {
			return telegramCommandReply{Text: "踢下线失败：" + err.Error()}
		}
		return telegramCommandReply{Text: "已踢下线此账号的全部设备。"}
	}
	devices, _ := s.listUserDevices(ctx, user.ID)
	if len(devices) == 0 {
		return telegramCommandReply{Text: "当前没有记录到登录设备。"}
	}
	var chosen *model.UserDevice
	if n, err := strconv.Atoi(target); err == nil && n >= 1 && n <= len(devices) {
		chosen = &devices[n-1]
	} else {
		for i := range devices {
			if devices[i].ID == target || devices[i].DeviceID == target {
				chosen = &devices[i]
				break
			}
		}
	}
	if chosen == nil {
		return telegramCommandReply{Text: "未找到该设备。请用 <code>/devices</code> 查看设备编号后重试。"}
	}
	if s.device != nil {
		if err := s.device.KickDevice(ctx, user.ID, chosen.DeviceID); err != nil {
			return telegramCommandReply{Text: "踢下线失败：" + err.Error()}
		}
	} else if fp := strings.TrimSpace(chosen.Fingerprint); fp != "" {
		if err := s.repo.UserDevice.SetKickedByFingerprint(ctx, user.ID, fp, true); err != nil {
			return telegramCommandReply{Text: "踢下线失败：" + err.Error()}
		}
	} else if err := s.repo.UserDevice.SetKicked(ctx, chosen.ID, true); err != nil {
		return telegramCommandReply{Text: "踢下线失败：" + err.Error()}
	}
	return telegramCommandReply{Text: fmt.Sprintf("已踢下线：<b>%s</b>。", deviceLabel(chosen.DeviceName, chosen.Client))}
}

func (s *TelegramBotService) cmdSetName(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	if len(args) < 2 {
		return telegramCommandReply{Text: "请发送：<code>/setname 当前密码 新用户名</code>"}
	}
	return s.selfSetName(ctx, msg, strings.Join(args, " "))
}

func (s *TelegramBotService) cmdSetPass(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	if len(args) < 2 {
		return telegramCommandReply{Text: "请发送：<code>/setpass 当前密码 新密码</code>"}
	}
	return s.selfSetPass(ctx, msg, strings.Join(args, " "))
}

func (s *TelegramBotService) replyAccount(ctx context.Context, msg *TelegramMessage) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号：<code>/start 用户名 密码</code>"}
	}
	streak := 0
	if rec, _ := s.repo.SignIn.Get(ctx, user.ID); rec != nil {
		streak = rec.StreakDays
	}
	devices, _ := s.listUserDevices(ctx, user.ID)
	text := fmt.Sprintf("<b>我的账号</b>\n\n用户名：<b>%s</b>\n状态：<b>%s</b>\n到期：<b>%s</b>\n连续签到：<b>%d 天</b>\n登录设备：<b>%d 台</b>",
		user.Username,
		map[bool]string{true: "正常", false: "已禁用"}[user.IsActive],
		formatExpiry(user.ExpiredAt), streak, len(devices))
	return telegramCommandReply{Text: text, Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回菜单", Data: "menu_main"}}}}
}

func (s *TelegramBotService) replySignIn(ctx context.Context, msg *TelegramMessage) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号后再签到。"}
	}
	res, err := s.signIn(ctx, user.ID)
	if err != nil {
		return telegramCommandReply{Text: "签到失败：" + err.Error()}
	}
	if res.AlreadySigned {
		return telegramCommandReply{Text: fmt.Sprintf("今天已经签到过啦～\n连续签到 <b>%d</b> 天，累计 <b>%d</b> 天。", res.Streak, res.Total)}
	}
	return telegramCommandReply{Text: fmt.Sprintf("签到成功 ✅\n连续签到 <b>%d</b> 天，累计 <b>%d</b> 天。", res.Streak, res.Total)}
}

func (s *TelegramBotService) replyDevices(ctx context.Context, msg *TelegramMessage) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号。"}
	}
	devices, _ := s.listUserDevices(ctx, user.ID)
	if len(devices) == 0 {
		return telegramCommandReply{Text: "当前没有记录到登录设备。"}
	}
	var sb strings.Builder
	sb.WriteString("<b>我的登录设备</b>\n点击下方按钮可一键踢下线：\n")
	var rows [][]telegramInlineButton
	for i, d := range devices {
		status := ""
		if d.Kicked {
			status = "（已踢下线）"
		} else if d.Playing {
			status = "（播放中）"
		} else if d.Online {
			status = "（在线）"
		}
		sb.WriteString(fmt.Sprintf("\n%d. <b>%s</b>%s\n   最近活跃：%s", i+1, deviceLabel(d.DeviceName, d.Client), status, d.LastSeenAt.Format("01-02 15:04")))
		if !d.Kicked && !strings.HasPrefix(d.ID, "rt:") {
			rows = append(rows, []telegramInlineButton{{Text: "🚫 踢下线：" + deviceLabel(d.DeviceName, d.Client), Data: "kick:" + d.ID}})
		}
	}
	rows = append(rows, []telegramInlineButton{{Text: "⬅️ 返回菜单", Data: "menu_main"}})
	return telegramCommandReply{Text: sb.String(), Buttons: rows}
}

func (s *TelegramBotService) replyKick(ctx context.Context, msg *TelegramMessage, deviceRowID string) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号。"}
	}
	var d model.UserDevice
	if err := s.repo.DB.WithContext(ctx).Where("id = ? AND user_id = ?", deviceRowID, user.ID).First(&d).Error; err != nil {
		return telegramCommandReply{Text: "未找到该设备。"}
	}
	if s.device != nil {
		if err := s.device.KickDevice(ctx, user.ID, d.DeviceID); err != nil {
			return telegramCommandReply{Text: "操作失败：" + err.Error()}
		}
	} else if fp := strings.TrimSpace(d.Fingerprint); fp != "" {
		if err := s.repo.UserDevice.SetKickedByFingerprint(ctx, user.ID, fp, true); err != nil {
			return telegramCommandReply{Text: "操作失败：" + err.Error()}
		}
	} else if err := s.repo.UserDevice.SetKicked(ctx, d.ID, true); err != nil {
		return telegramCommandReply{Text: "操作失败：" + err.Error()}
	}
	return s.replyDevices(ctx, msg)
}

func (s *TelegramBotService) listUserDevices(ctx context.Context, userID string) ([]model.UserDevice, error) {
	if s.device != nil {
		return s.device.ListDevices(ctx, userID)
	}
	return s.repo.UserDevice.ListByUser(ctx, userID)
}

func (s *TelegramBotService) selfSetName(ctx context.Context, msg *TelegramMessage, input string) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号。"}
	}
	currentPassword, newName := splitCurrentPasswordAndValue(input)
	if currentPassword == "" || newName == "" {
		return telegramCommandReply{Text: "请发送：<code>当前密码 新用户名</code>。"}
	}
	newName = strings.TrimSpace(newName)
	if len(newName) < 2 || strings.ContainsAny(newName, " \t\n") {
		return telegramCommandReply{Text: "用户名至少 2 位且不能含空格，请重试。"}
	}
	if reply, ok := s.verifyTelegramSelfPassword(ctx, msg, user, currentPassword); !ok {
		return reply
	}
	if existing, _ := s.repo.User.FindByUsername(ctx, newName); existing != nil && existing.ID != user.ID {
		return telegramCommandReply{Text: "该用户名已被占用，请换一个。"}
	}
	if err := s.repo.User.UpdateFields(ctx, user.ID, map[string]any{"username": newName}); err != nil {
		return telegramCommandReply{Text: "修改失败：" + err.Error()}
	}
	return telegramCommandReply{Text: fmt.Sprintf("用户名已修改为 <b>%s</b>。请用新用户名登录。", newName)}
}

func (s *TelegramBotService) selfSetPass(ctx context.Context, msg *TelegramMessage, input string) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号。"}
	}
	currentPassword, newPass := splitCurrentPasswordAndValue(input)
	if currentPassword == "" || newPass == "" {
		return telegramCommandReply{Text: "请发送：<code>当前密码 新密码</code>。"}
	}
	newPass = strings.TrimSpace(newPass)
	if s.auth == nil {
		return telegramCommandReply{Text: "服务暂不可用。"}
	}
	if err := s.auth.ChangePassword(ctx, user.ID, currentPassword, newPass); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			_ = s.unbindTelegramUser(ctx, msg.From.ID)
			return telegramCommandReply{Text: "当前密码验证失败，绑定已自动解绑。请用新密码重新绑定账号。"}
		}
		return telegramCommandReply{Text: "修改失败：" + err.Error()}
	}
	if s.device != nil {
		_ = s.device.KickAllDevices(ctx, user.ID)
	}
	return telegramCommandReply{Text: "密码已修改，请用新密码重新登录第三方客户端。"}
}

func splitCurrentPasswordAndValue(input string) (string, string) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) < 2 {
		return "", ""
	}
	return fields[0], strings.TrimSpace(strings.Join(fields[1:], " "))
}

func (s *TelegramBotService) verifyTelegramSelfPassword(ctx context.Context, msg *TelegramMessage, user *model.User, currentPassword string) (telegramCommandReply, bool) {
	if s.auth == nil {
		return telegramCommandReply{Text: "服务暂不可用。"}, false
	}
	if err := s.auth.VerifyPassword(ctx, user.ID, currentPassword); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			_ = s.unbindTelegramUser(ctx, msg.From.ID)
			return telegramCommandReply{Text: "当前密码验证失败，绑定已自动解绑。请用新密码重新绑定账号。"}, false
		}
		return telegramCommandReply{Text: "验证失败：" + err.Error()}, false
	}
	return telegramCommandReply{}, true
}
