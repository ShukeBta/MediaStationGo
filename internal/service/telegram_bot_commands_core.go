package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// cmdStart 处理 /start 命令。
func (s *TelegramBotService) cmdStart(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	name := msg.From.FirstName
	if msg.From.Username != "" {
		name = "@" + msg.From.Username
	}
	if telegramIsGroupChat(msg.Chat.Type) && len(args) > 0 {
		return telegramCommandReply{Text: telegramGroupPrivateUserHint("绑定账号")}
	}
	if len(args) == 0 {
		if binding := s.telegramBinding(ctx, msg.From.ID); binding != nil {
			user, _ := s.repo.User.FindByID(ctx, binding.UserID)
			if user == nil {
				_ = s.repo.DB.WithContext(ctx).Unscoped().Delete(&model.TelegramBinding{}, "id = ?", binding.ID).Error
				return telegramCommandReply{Text: "之前绑定的媒体中心账号已不存在，请重新绑定：\n<code>/start 用户名 密码</code>"}
			}
			status := "未隐藏"
			if user.HideAdult {
				status = "已隐藏"
			}
			return telegramCommandReply{
				Text: fmt.Sprintf("<b>MediaStationGo 已绑定</b>\n\n你好 %s，当前账号：<b>%s</b>\n成人目录：<b>%s</b>", name, userNameOrFallback(user), status),
				Buttons: [][]telegramInlineButton{{{
					Text: map[bool]string{true: "显示成人目录", false: "隐藏成人目录"}[user.HideAdult],
					Data: "adult_toggle",
				}}},
			}
		}
		hint := "如果没有账号，请联系管理员注册。"
		if s.openRegEnabled(ctx) {
			hint = "如果还没有账号，可直接注册：\n<code>/register 用户名 密码</code>\n或：<code>/register 用户名-密码</code>"
		}
		return telegramCommandReply{Text: "<b>欢迎使用 MediaStationGo</b>\n\n普通用户请先绑定账号：\n<code>/start 用户名 密码</code>\n或：<code>/start 用户名-密码</code>\n\n" + hint}
	}
	channel := s.findChannelForMessage(ctx, msg)
	if dec := s.telegramUserBindDecision(ctx, channel, msg.From.ID); dec != bindAllowed {
		return telegramCommandReply{Text: telegramBindRejectText(dec, "绑定媒体中心账号")}
	}
	username, password := parseStartCredentials(args)
	if username == "" || password == "" {
		return telegramCommandReply{Text: "绑定格式不正确，请使用：\n<code>/start 用户名 密码</code>\n或：<code>/start 用户名-密码</code>"}
	}
	existingBinding := s.telegramBinding(ctx, msg.From.ID)
	user, err := s.repo.User.FindByUsername(ctx, username)
	if err != nil || user == nil {
		if existingBinding != nil {
			_ = s.unbindTelegramUser(ctx, msg.From.ID)
			return telegramCommandReply{Text: "当前绑定的媒体账号信息已失效，已自动解绑。请使用新的用户名和密码重新绑定。"}
		}
		return telegramCommandReply{Text: "未找到此用户，请联系管理员注册。"}
	}
	if !user.IsActive {
		return telegramCommandReply{Text: "此账号已被禁用，请联系管理员。"}
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		if existingBinding != nil && existingBinding.UserID == user.ID {
			_ = s.unbindTelegramUser(ctx, msg.From.ID)
			return telegramCommandReply{Text: "当前绑定账号的密码已失效，已自动解绑。请使用新密码重新绑定。"}
		}
		return telegramCommandReply{Text: "账号或密码错误。"}
	}
	if err := s.upsertTelegramBinding(ctx, msg, user.ID); err != nil {
		return telegramCommandReply{Text: "绑定失败：" + err.Error()}
	}
	return telegramCommandReply{
		Text: fmt.Sprintf("绑定成功：<b>%s</b>\n\n普通用户只能使用此 Bot 管理自己的成人目录隐藏状态；系统状态、搜索、下载和统计命令仅管理员可用。", user.Username),
		Buttons: [][]telegramInlineButton{{{
			Text: map[bool]string{true: "显示成人目录", false: "隐藏成人目录"}[user.HideAdult],
			Data: "adult_toggle",
		}}},
	}
}

// cmdRegister 处理 /register 命令：在管理员开启注册后，普通用户可通过 Bot
// 注册一个新的媒体中心账号，并自动绑定到当前 Telegram 账号。
func (s *TelegramBotService) cmdRegister(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, args []string) telegramCommandReply {
	if len(args) == 1 && looksLikeRedemptionCode(args[0]) {
		return s.redeemRegisterFlow(ctx, channel, msg, args[0])
	}
	if !s.openRegEnabled(ctx) {
		return telegramCommandReply{Text: "注册功能未开放，请联系管理员开启后再试。"}
	}
	// 开注名额已用尽则拦截（容量随凭证授权实时变化，名额单独计数）。
	if c := s.loadCapacity(ctx); c.Remaining() <= 0 {
		return telegramCommandReply{Text: "注册名额已满，请等待管理员重新开放或扩容授权。"}
	}
	if s.auth == nil {
		return telegramCommandReply{Text: "注册功能暂不可用，请联系管理员。"}
	}
	if channel == nil {
		channel = s.findChannelForMessage(ctx, msg)
	}
	if dec := s.telegramUserBindDecision(ctx, channel, msg.From.ID); dec != bindAllowed {
		return telegramCommandReply{Text: telegramBindRejectText(dec, "注册账号")}
	}
	if binding := s.telegramBinding(ctx, msg.From.ID); binding != nil {
		if user, _ := s.repo.User.FindByID(ctx, binding.UserID); user != nil {
			return telegramCommandReply{Text: fmt.Sprintf("当前 Telegram 已绑定账号：<b>%s</b>，无需重复注册。\n如需切换账号请使用 <code>/start 用户名 密码</code>。", userNameOrFallback(user))}
		}
	}
	username, password := parseStartCredentials(args)
	if username == "" || password == "" {
		return telegramCommandReply{Text: "注册格式不正确，请使用：\n<code>/register 用户名 密码</code>\n或：<code>/register 用户名-密码</code>"}
	}
	user, _, err := s.auth.Register(ctx, username, password)
	if err != nil {
		switch {
		case errors.Is(err, ErrUsernameTaken):
			return telegramCommandReply{Text: "该用户名已被占用，请换一个；如果是你本人的账号，请改用 <code>/start 用户名 密码</code> 绑定。"}
		case errors.Is(err, ErrUserLimitReached):
			return telegramCommandReply{Text: "注册失败：已达到用户数量上限，请联系管理员。"}
		default:
			return telegramCommandReply{Text: "注册失败：" + err.Error()}
		}
	}
	// 注册成功，扣减一个开注名额（名额用尽自动关闭注册）。
	s.consumeOpenRegSlot(ctx)
	if err := s.upsertTelegramBinding(ctx, msg, user.ID); err != nil {
		return telegramCommandReply{Text: fmt.Sprintf("账号 <b>%s</b> 注册成功，但自动绑定失败：%s\n请稍后使用 <code>/start %s 密码</code> 重新绑定。", user.Username, err.Error(), user.Username)}
	}
	return telegramCommandReply{
		Text: fmt.Sprintf("注册并绑定成功：<b>%s</b>\n\n你现在可以用此账号登录网页与第三方客户端。普通用户只能在此 Bot 管理成人目录显隐；其他功能仅管理员可用。", user.Username),
		Buttons: [][]telegramInlineButton{{{
			Text: map[bool]string{true: "显示成人目录", false: "隐藏成人目录"}[user.HideAdult],
			Data: "adult_toggle",
		}}},
	}
}

// cmdRegistrationToggle handles /registration and /openreg. It uses the same
// quota-aware open-registration state as the inline Bot menu.
func (s *TelegramBotService) cmdRegistrationToggle(ctx context.Context, args []string) telegramCommandReply {
	if len(args) == 0 || strings.EqualFold(strings.TrimSpace(args[0]), "status") {
		c := s.loadCapacity(ctx)
		state := "已关闭"
		if c.OpenRegOn {
			if c.OpenRegLimit > 0 {
				state = fmt.Sprintf("已开启（%d/%d 名额）", c.OpenRegUsed, c.OpenRegLimit)
			} else {
				state = "已开启（不限名额，受授权上限约束）"
			}
		}
		return telegramCommandReply{Text: fmt.Sprintf("普通用户 Bot 注册功能当前<b>%s</b>。\n剩余可注册：<b>%d</b> 人。\n\n开启：<code>/registration on 10</code>\n不限：<code>/registration on 0</code>\n关闭：<code>/registration off</code>", state, c.Remaining())}
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "on", "true", "1", "open", "enable", "enabled", "开启", "打开", "开":
		limit := 0
		if len(args) > 1 {
			n, err := strconv.Atoi(strings.TrimSpace(args[1]))
			if err != nil || n < 0 {
				return telegramCommandReply{Text: "名额必须是非负整数，0 表示不限名额。"}
			}
			limit = n
		}
		if err := s.openRegistration(ctx, limit); err != nil {
			return telegramCommandReply{Text: "开启失败：" + err.Error()}
		}
		label := "不限名额"
		if limit > 0 {
			label = fmt.Sprintf("%d 个名额", limit)
		}
		return telegramCommandReply{Text: "普通用户 Bot 注册功能已开启：" + label + "。"}
	case "off", "false", "0", "close", "disable", "disabled", "关闭", "关":
		if err := s.closeRegistration(ctx); err != nil {
			return telegramCommandReply{Text: "关闭失败：" + err.Error()}
		}
		return telegramCommandReply{Text: "普通用户 Bot 注册功能已关闭。"}
	default:
		return telegramCommandReply{Text: "参数无效，请使用 <code>/registration on [名额]</code> 或 <code>/registration off</code>。"}
	}
}

// cmdStatus 处理 /status 命令。
func (s *TelegramBotService) cmdHideAdult(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	channel := s.findChannelForMessage(ctx, msg)
	if dec := s.telegramUserBindDecision(ctx, channel, msg.From.ID); dec != bindAllowed {
		return telegramCommandReply{Text: telegramBindRejectText(dec, "使用成人目录隐藏开关")}
	}
	binding := s.telegramBinding(ctx, msg.From.ID)
	if binding == nil {
		return telegramCommandReply{Text: "请先绑定账号：<code>/start 用户名 密码</code>"}
	}
	user, err := s.repo.User.FindByID(ctx, binding.UserID)
	if err != nil || user == nil {
		return telegramCommandReply{Text: "绑定用户不存在，请重新 /start 绑定。"}
	}
	next := true
	if len(args) > 0 {
		switch strings.ToLower(strings.TrimSpace(args[0])) {
		case "off", "false", "0", "show", "显示", "关闭":
			next = false
		case "on", "true", "1", "hide", "隐藏", "开启":
			next = true
		default:
			next = !user.HideAdult
		}
	} else {
		next = !user.HideAdult
	}
	if err := s.repo.User.UpdateFields(ctx, user.ID, map[string]any{"hide_adult": next}); err != nil {
		return telegramCommandReply{Text: "更新失败：" + err.Error()}
	}
	status := map[bool]string{true: "已隐藏", false: "已显示"}[next]
	return telegramCommandReply{
		Text: "成人目录" + status + "。此设置会同步影响网页与第三方客户端。",
		Buttons: [][]telegramInlineButton{{{
			Text: map[bool]string{true: "显示成人目录", false: "隐藏成人目录"}[next],
			Data: "adult_toggle",
		}}},
	}
}
