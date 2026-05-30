package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// pendingTTL bounds how long a button-initiated text prompt stays valid.
const pendingTTL = 5 * time.Minute

func (s *TelegramBotService) setPending(userID int64, kind string) {
	s.pendingMu.Lock()
	s.pending[userID] = pendingInput{Kind: kind, CreatedAt: time.Now()}
	s.pendingMu.Unlock()
}

func (s *TelegramBotService) takePending(userID int64) (pendingInput, bool) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	p, ok := s.pending[userID]
	if ok {
		delete(s.pending, userID)
	}
	if ok && time.Since(p.CreatedAt) > pendingTTL {
		return pendingInput{}, false
	}
	return p, ok
}

// boundUser resolves the local user bound to a Telegram account, or nil.
func (s *TelegramBotService) boundUser(ctx context.Context, telegramUserID int) *model.User {
	binding := s.telegramBinding(ctx, telegramUserID)
	if binding == nil {
		return nil
	}
	u, _ := s.repo.User.FindByID(ctx, binding.UserID)
	return u
}

// mainMenu builds the button-based menu, tailored to the user's binding and
// admin status. Ordinary users only see self-service actions; admins get an
// extra management section.
func (s *TelegramBotService) mainMenu(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage) telegramCommandReply {
	isAdmin := s.telegramUserIsAdmin(ctx, channel, msg.From.ID)
	user := s.boundUser(ctx, msg.From.ID)

	var rows [][]telegramInlineButton
	var header string

	if user == nil {
		header = "<b>MediaStationGo</b>\n\n你还没有绑定媒体中心账号。"
		rows = append(rows, []telegramInlineButton{{Text: "🔗 绑定账号", Data: "act_bind"}})
		if s.openRegEnabled(ctx) {
			rows = append(rows, []telegramInlineButton{{Text: "📝 注册新账号", Data: "act_register"}})
		}
		rows = append(rows, []telegramInlineButton{{Text: "🎟 兑换码注册", Data: "act_redeem_register"}})
	} else {
		adult := map[bool]string{true: "已隐藏", false: "已显示"}[user.HideAdult]
		header = fmt.Sprintf("<b>MediaStationGo</b>\n\n账号：<b>%s</b>\n到期：<b>%s</b>\n成人目录：<b>%s</b>",
			user.Username, formatExpiry(user.ExpiredAt), adult)
		rows = append(rows,
			[]telegramInlineButton{
				{Text: "👤 我的账号", Data: "act_account"},
				{Text: "📅 签到", Data: "act_signin"},
			},
			[]telegramInlineButton{
				{Text: "📱 我的设备", Data: "act_devices"},
				{Text: map[bool]string{true: "🔞 显示成人目录", false: "🔞 隐藏成人目录"}[user.HideAdult], Data: "adult_toggle"},
			},
			[]telegramInlineButton{
				{Text: "✏️ 改用户名", Data: "act_setname"},
				{Text: "🔑 改密码", Data: "act_setpass"},
			},
			[]telegramInlineButton{{Text: "🎟 兑换码续期", Data: "act_redeem_renew"}},
		)
	}

	if isAdmin {
		rows = append(rows,
			[]telegramInlineButton{{Text: "—— 管理员 ——", Data: "noop"}},
			[]telegramInlineButton{
				{Text: "📊 容量/状态", Data: "adm_capacity"},
				{Text: "👥 用户管理", Data: "adm_users"},
			},
			[]telegramInlineButton{
				{Text: "🔓 开注设置", Data: "adm_openreg"},
				{Text: "🎟 生成兑换码", Data: "adm_gencode"},
			},
			[]telegramInlineButton{{Text: "⚙️ 设备策略", Data: "adm_devicepolicy"}},
		)
	}

	return telegramCommandReply{Text: header, Buttons: rows}
}

// handleMenuCallback routes inline-button taps. Returns (reply, handled).
func (s *TelegramBotService) handleMenuCallback(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, data string) (telegramCommandReply, bool) {
	isAdmin := s.telegramUserIsAdmin(ctx, channel, msg.From.ID)

	switch {
	case data == "noop":
		return telegramCommandReply{}, true
	case data == "menu_main":
		return s.mainMenu(ctx, channel, msg), true
	case data == "act_bind":
		return telegramCommandReply{Text: "请发送：<code>/start 用户名 密码</code> 绑定已有账号。"}, true
	case data == "act_register":
		if !s.openRegEnabled(ctx) {
			return telegramCommandReply{Text: "注册功能未开放，请联系管理员。"}, true
		}
		s.setPending(int64(msg.From.ID), "register")
		return telegramCommandReply{Text: "请发送新账号的 <b>用户名 密码</b>（空格分隔），例如：<code>alice mypass123</code>"}, true
	case data == "act_redeem_register":
		s.setPending(int64(msg.From.ID), "redeem_register")
		return telegramCommandReply{Text: "请发送你的<b>注册兑换码</b>，例如：<code>ABCD2345EFGH</code>\n（兑换后会要求设置用户名密码）"}, true
	case data == "act_redeem_renew":
		s.setPending(int64(msg.From.ID), "redeem_renew")
		return telegramCommandReply{Text: "请发送你的<b>续期兑换码</b>，将为当前绑定账号续期。"}, true
	case data == "act_account":
		return s.replyAccount(ctx, msg), true
	case data == "act_signin":
		return s.replySignIn(ctx, msg), true
	case data == "act_devices":
		return s.replyDevices(ctx, msg), true
	case data == "act_setname":
		s.setPending(int64(msg.From.ID), "setname")
		return telegramCommandReply{Text: "请发送新的<b>用户名</b>。"}, true
	case data == "act_setpass":
		s.setPending(int64(msg.From.ID), "setpass")
		return telegramCommandReply{Text: "请发送新的<b>密码</b>（至少 6 位）。"}, true
	case strings.HasPrefix(data, "kick:"):
		return s.replyKick(ctx, msg, strings.TrimPrefix(data, "kick:")), true
	}

	// ── 管理员专属 ──
	if !isAdmin {
		return telegramCommandReply{Text: "此功能仅管理员可用。"}, true
	}
	switch {
	case data == "adm_capacity":
		return s.replyCapacity(ctx), true
	case data == "adm_openreg":
		return s.replyOpenRegMenu(ctx), true
	case data == "adm_openreg_close":
		_ = s.closeRegistration(ctx)
		return telegramCommandReply{Text: "已关闭注册。"}, true
	case strings.HasPrefix(data, "adm_openreg_set:"):
		n, _ := strconv.Atoi(strings.TrimPrefix(data, "adm_openreg_set:"))
		if err := s.openRegistration(ctx, n); err != nil {
			return telegramCommandReply{Text: "开注失败：" + err.Error()}, true
		}
		label := "不限"
		if n > 0 {
			label = fmt.Sprintf("%d 个名额", n)
		}
		return telegramCommandReply{Text: "已开放注册：" + label + "。"}, true
	case data == "adm_gencode":
		return s.replyGenCodeMenu(), true
	case strings.HasPrefix(data, "gc:"):
		return s.replyGenCode(ctx, msg, data), true
	case data == "adm_users":
		return s.replyUserList(ctx), true
	case strings.HasPrefix(data, "usr:"):
		return s.replyUserActions(ctx, strings.TrimPrefix(data, "usr:")), true
	case strings.HasPrefix(data, "uban:"):
		return s.replyUserBan(ctx, strings.TrimPrefix(data, "uban:"), false), true
	case strings.HasPrefix(data, "uunban:"):
		return s.replyUserBan(ctx, strings.TrimPrefix(data, "uunban:"), true), true
	case strings.HasPrefix(data, "udel:"):
		return s.replyUserDelete(ctx, strings.TrimPrefix(data, "udel:")), true
	case strings.HasPrefix(data, "urenew:"):
		return s.replyUserRenew(ctx, strings.TrimPrefix(data, "urenew:")), true
	case data == "adm_devicepolicy":
		return s.replyDevicePolicy(ctx), true
	case strings.HasPrefix(data, "dp_toggle:"):
		return s.replyDevicePolicyToggle(ctx, strings.TrimPrefix(data, "dp_toggle:")), true
	}
	return telegramCommandReply{}, false
}

// handlePendingText consumes a button-initiated text prompt. Returns (reply,
// handled). handled=false means there was no pending prompt for this user.
func (s *TelegramBotService) handlePendingText(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, text string) (telegramCommandReply, bool) {
	p, ok := s.takePending(int64(msg.From.ID))
	if !ok {
		return telegramCommandReply{}, false
	}
	switch p.Kind {
	case "register":
		return s.cmdRegister(ctx, channel, msg, strings.Fields(text)), true
	case "redeem_register":
		return s.redeemRegisterFlow(ctx, channel, msg, text), true
	case "redeem_renew":
		return s.redeemRenewFlow(ctx, msg, text), true
	case "setname":
		return s.selfSetName(ctx, msg, text), true
	case "setpass":
		return s.selfSetPass(ctx, msg, text), true
	case "openreg_limit":
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 0 {
			return telegramCommandReply{Text: "请输入有效的非负整数。"}, true
		}
		if err := s.openRegistration(ctx, n); err != nil {
			return telegramCommandReply{Text: "开注失败：" + err.Error()}, true
		}
		return telegramCommandReply{Text: fmt.Sprintf("已开放注册：%d 个名额。", n)}, true
	}
	return telegramCommandReply{}, false
}

// ── 用户自助 ──────────────────────────────────────────────────────────────

func (s *TelegramBotService) replyAccount(ctx context.Context, msg *TelegramMessage) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号：<code>/start 用户名 密码</code>"}
	}
	streak := 0
	if rec, _ := s.repo.SignIn.Get(ctx, user.ID); rec != nil {
		streak = rec.StreakDays
	}
	devices, _ := s.repo.UserDevice.ListByUser(ctx, user.ID)
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
	devices, _ := s.repo.UserDevice.ListByUser(ctx, user.ID)
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
		}
		sb.WriteString(fmt.Sprintf("\n%d. <b>%s</b>%s\n   最近活跃：%s", i+1, deviceLabel(d.DeviceName, d.Client), status, d.LastSeenAt.Format("01-02 15:04")))
		if !d.Kicked {
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
	// Verify the device belongs to this user before kicking.
	var d model.UserDevice
	if err := s.repo.DB.WithContext(ctx).Where("id = ? AND user_id = ?", deviceRowID, user.ID).First(&d).Error; err != nil {
		return telegramCommandReply{Text: "未找到该设备。"}
	}
	if err := s.repo.UserDevice.SetKicked(ctx, d.ID, true); err != nil {
		return telegramCommandReply{Text: "操作失败：" + err.Error()}
	}
	return s.replyDevices(ctx, msg)
}

func (s *TelegramBotService) selfSetName(ctx context.Context, msg *TelegramMessage, newName string) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号。"}
	}
	newName = strings.TrimSpace(newName)
	if len(newName) < 2 || strings.ContainsAny(newName, " \t\n") {
		return telegramCommandReply{Text: "用户名至少 2 位且不能含空格，请重试。"}
	}
	if existing, _ := s.repo.User.FindByUsername(ctx, newName); existing != nil && existing.ID != user.ID {
		return telegramCommandReply{Text: "该用户名已被占用，请换一个。"}
	}
	if err := s.repo.User.UpdateFields(ctx, user.ID, map[string]any{"username": newName}); err != nil {
		return telegramCommandReply{Text: "修改失败：" + err.Error()}
	}
	return telegramCommandReply{Text: fmt.Sprintf("用户名已修改为 <b>%s</b>。请用新用户名登录。", newName)}
}

func (s *TelegramBotService) selfSetPass(ctx context.Context, msg *TelegramMessage, newPass string) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号。"}
	}
	newPass = strings.TrimSpace(newPass)
	if s.auth == nil {
		return telegramCommandReply{Text: "服务暂不可用。"}
	}
	if err := s.auth.ResetPassword(ctx, user.ID, newPass); err != nil {
		return telegramCommandReply{Text: "修改失败：" + err.Error()}
	}
	return telegramCommandReply{Text: "密码已修改，请用新密码重新登录第三方客户端。"}
}

// ── 兑换码流程 ───────────────────────────────────────────────────────────────

func (s *TelegramBotService) redeemRegisterFlow(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, raw string) telegramCommandReply {
	rc, errMsg := s.lookupRedeemableCode(ctx, raw, model.RegistrationCodeRegister)
	if rc == nil {
		return telegramCommandReply{Text: errMsg}
	}
	if s.auth == nil {
		return telegramCommandReply{Text: "注册服务暂不可用。"}
	}
	if binding := s.telegramBinding(ctx, msg.From.ID); binding != nil {
		if u, _ := s.repo.User.FindByID(ctx, binding.UserID); u != nil {
			return telegramCommandReply{Text: fmt.Sprintf("当前 Telegram 已绑定账号 <b>%s</b>，无需再用注册码。", u.Username)}
		}
	}
	// Generate a memorable default account from the code; users can rename via
	//「改用户名/改密码」afterwards. We avoid asking for two more text turns here.
	username := "u" + strings.ToLower(rc.Code[:8])
	password := randomCode(10)
	user, _, err := s.auth.Register(ctx, username, password)
	if err != nil {
		return telegramCommandReply{Text: "注册失败：" + err.Error()}
	}
	if err := s.repo.RegCode.MarkUsed(ctx, rc.ID, user.ID); err != nil {
		// Code was raced; roll back the just-created account to avoid free signups.
		_ = s.repo.User.Delete(ctx, user.ID)
		return telegramCommandReply{Text: "兑换码刚刚被使用，请换一个。"}
	}
	if rc.DurationDays > 0 {
		_ = s.applyRenewal(ctx, user.ID, rc.DurationDays)
	}
	_ = s.upsertTelegramBinding(ctx, msg, user.ID)
	return telegramCommandReply{
		Text: fmt.Sprintf("兑换成功并已创建账号：\n用户名：<b>%s</b>\n密码：<b>%s</b>\n到期：<b>%s</b>\n\n请尽快用「改用户名/改密码」修改为你自己的凭据。",
			username, password, formatExpiry(s.userExpiry(ctx, user.ID))),
		Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回菜单", Data: "menu_main"}}},
	}
}

func (s *TelegramBotService) redeemRenewFlow(ctx context.Context, msg *TelegramMessage, raw string) telegramCommandReply {
	user := s.boundUser(ctx, msg.From.ID)
	if user == nil {
		return telegramCommandReply{Text: "请先绑定账号再续期。"}
	}
	rc, errMsg := s.lookupRedeemableCode(ctx, raw, model.RegistrationCodeRenew)
	if rc == nil {
		return telegramCommandReply{Text: errMsg}
	}
	if err := s.repo.RegCode.MarkUsed(ctx, rc.ID, user.ID); err != nil {
		return telegramCommandReply{Text: "兑换码刚刚被使用，请换一个。"}
	}
	if err := s.applyRenewal(ctx, user.ID, rc.DurationDays); err != nil {
		return telegramCommandReply{Text: "续期失败：" + err.Error()}
	}
	return telegramCommandReply{Text: fmt.Sprintf("续期成功 ✅ 当前到期：<b>%s</b>", formatExpiry(s.userExpiry(ctx, user.ID)))}
}

func (s *TelegramBotService) userExpiry(ctx context.Context, userID string) *time.Time {
	if u, _ := s.repo.User.FindByID(ctx, userID); u != nil {
		return u.ExpiredAt
	}
	return nil
}

// ── 管理员：容量 / 开注 / 兑换码 / 用户管理 / 设备策略 ─────────────────────────

func (s *TelegramBotService) replyCapacity(ctx context.Context) telegramCommandReply {
	c := s.loadCapacity(ctx)
	quota := "未开放"
	if c.OpenRegOn {
		if c.OpenRegLimit > 0 {
			quota = fmt.Sprintf("已开放（%d/%d 名额）", c.OpenRegUsed, c.OpenRegLimit)
		} else {
			quota = "已开放（不限名额，受授权上限约束）"
		}
	}
	text := fmt.Sprintf("<b>容量 / 状态</b>\n\n授权上限：<b>%d</b> 人（随凭证授权实时变化）\n已用：<b>%d</b> 人\n剩余可注册：<b>%d</b> 人\n开注状态：<b>%s</b>",
		c.MaxUsers, c.UsedUsers, c.Remaining(), quota)
	return telegramCommandReply{Text: text, Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回菜单", Data: "menu_main"}}}}
}

func (s *TelegramBotService) replyOpenRegMenu(ctx context.Context) telegramCommandReply {
	c := s.loadCapacity(ctx)
	state := "未开放"
	if c.OpenRegOn {
		state = fmt.Sprintf("已开放（%d/%d）", c.OpenRegUsed, c.OpenRegLimit)
	}
	return telegramCommandReply{
		Text: "<b>开注设置</b>\n当前：" + state + "\n选择要开放的名额：",
		Buttons: [][]telegramInlineButton{
			{{Text: "5 个", Data: "adm_openreg_set:5"}, {Text: "10 个", Data: "adm_openreg_set:10"}, {Text: "20 个", Data: "adm_openreg_set:20"}},
			{{Text: "不限名额", Data: "adm_openreg_set:0"}, {Text: "关闭注册", Data: "adm_openreg_close"}},
			{{Text: "⬅️ 返回菜单", Data: "menu_main"}},
		},
	}
}

func (s *TelegramBotService) replyGenCodeMenu() telegramCommandReply {
	return telegramCommandReply{
		Text: "<b>生成兑换码</b>\n选择类型与时长：",
		Buttons: [][]telegramInlineButton{
			{{Text: "注册码·30天", Data: "gc:register:30"}, {Text: "注册码·永久", Data: "gc:register:0"}},
			{{Text: "续期码·30天", Data: "gc:renew:30"}, {Text: "续期码·90天", Data: "gc:renew:90"}},
			{{Text: "⬅️ 返回菜单", Data: "menu_main"}},
		},
	}
}

func (s *TelegramBotService) replyGenCode(ctx context.Context, msg *TelegramMessage, data string) telegramCommandReply {
	parts := strings.Split(data, ":") // gc:<kind>:<days>
	if len(parts) != 3 {
		return telegramCommandReply{Text: "参数错误。"}
	}
	kind := parts[1]
	days, _ := strconv.Atoi(parts[2])
	createdBy := ""
	if u := s.boundUser(ctx, msg.From.ID); u != nil {
		createdBy = u.ID
	}
	code, err := s.generateCode(ctx, kind, days, 0, createdBy)
	if err != nil {
		return telegramCommandReply{Text: "生成失败：" + err.Error()}
	}
	kindLabel := map[string]string{model.RegistrationCodeRegister: "注册码", model.RegistrationCodeRenew: "续期码"}[code.Kind]
	dur := "永久"
	if days > 0 {
		dur = fmt.Sprintf("%d 天", days)
	}
	return telegramCommandReply{
		Text:    fmt.Sprintf("已生成%s（%s）：\n\n<code>%s</code>\n\n发给用户在 Bot 中兑换即可。", kindLabel, dur, code.Code),
		Buttons: [][]telegramInlineButton{{{Text: "再生成一个", Data: "adm_gencode"}, {Text: "⬅️ 返回菜单", Data: "menu_main"}}},
	}
}

func (s *TelegramBotService) replyUserList(ctx context.Context) telegramCommandReply {
	users, err := s.repo.User.List(ctx)
	if err != nil {
		return telegramCommandReply{Text: "读取用户失败：" + err.Error()}
	}
	if len(users) == 0 {
		return telegramCommandReply{Text: "暂无用户。"}
	}
	var rows [][]telegramInlineButton
	limit := len(users)
	if limit > 12 {
		limit = 12
	}
	for i := 0; i < limit; i++ {
		u := users[i]
		flag := ""
		if !u.IsActive {
			flag = "🚫"
		}
		if u.Role == "admin" {
			flag = "👑"
		}
		rows = append(rows, []telegramInlineButton{{Text: flag + " " + u.Username, Data: "usr:" + u.ID}})
	}
	rows = append(rows, []telegramInlineButton{{Text: "⬅️ 返回菜单", Data: "menu_main"}})
	return telegramCommandReply{Text: fmt.Sprintf("<b>用户管理</b>（共 %d 人，显示前 %d）\n点击用户进行操作：", len(users), limit), Buttons: rows}
}

func (s *TelegramBotService) replyUserActions(ctx context.Context, userID string) telegramCommandReply {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil || u == nil {
		return telegramCommandReply{Text: "用户不存在。"}
	}
	protected := u.Role == "admin"
	if first, _ := s.repo.User.FirstAdmin(ctx); first != nil && first.ID == u.ID {
		protected = true
	}
	text := fmt.Sprintf("<b>%s</b>\n角色：%s\n状态：%s\n到期：%s\n防共享警告：%d 次",
		u.Username, u.Role, map[bool]string{true: "正常", false: "已禁用"}[u.IsActive], formatExpiry(u.ExpiredAt), u.ShareWarnings)
	if protected {
		return telegramCommandReply{Text: text + "\n\n（受保护账号，不可禁用/删除）", Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回", Data: "adm_users"}}}}
	}
	banBtn := telegramInlineButton{Text: "🚫 禁用", Data: "uban:" + u.ID}
	if !u.IsActive {
		banBtn = telegramInlineButton{Text: "✅ 解禁", Data: "uunban:" + u.ID}
	}
	return telegramCommandReply{
		Text: text,
		Buttons: [][]telegramInlineButton{
			{banBtn, {Text: "⏳ 续期30天", Data: "urenew:" + u.ID + ":30"}},
			{{Text: "🗑 删除用户", Data: "udel:" + u.ID}},
			{{Text: "⬅️ 返回", Data: "adm_users"}},
		},
	}
}

func (s *TelegramBotService) replyUserBan(ctx context.Context, userID string, unban bool) telegramCommandReply {
	if !unban {
		if reason := s.protectReason(ctx, userID); reason != "" {
			return telegramCommandReply{Text: reason}
		}
	}
	if err := s.repo.User.UpdateFields(ctx, userID, map[string]any{"is_active": unban}); err != nil {
		return telegramCommandReply{Text: "操作失败：" + err.Error()}
	}
	return s.replyUserActions(ctx, userID)
}

func (s *TelegramBotService) replyUserDelete(ctx context.Context, userID string) telegramCommandReply {
	if reason := s.protectReason(ctx, userID); reason != "" {
		return telegramCommandReply{Text: reason}
	}
	u, _ := s.repo.User.FindByID(ctx, userID)
	_ = s.repo.UserDevice.DeleteByUser(ctx, userID)
	if err := s.repo.User.Delete(ctx, userID); err != nil {
		return telegramCommandReply{Text: "删除失败：" + err.Error()}
	}
	name := userID
	if u != nil {
		name = u.Username
	}
	return telegramCommandReply{Text: fmt.Sprintf("已删除用户 <b>%s</b>。", name), Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回", Data: "adm_users"}}}}
}

func (s *TelegramBotService) replyUserRenew(ctx context.Context, payload string) telegramCommandReply {
	parts := strings.Split(payload, ":") // <id>:<days>
	if len(parts) != 2 {
		return telegramCommandReply{Text: "参数错误。"}
	}
	days, _ := strconv.Atoi(parts[1])
	if err := s.applyRenewal(ctx, parts[0], days); err != nil {
		return telegramCommandReply{Text: "续期失败：" + err.Error()}
	}
	return s.replyUserActions(ctx, parts[0])
}

// protectReason returns a non-empty message when a user must not be
// disabled/deleted (admins and the default admin are protected).
func (s *TelegramBotService) protectReason(ctx context.Context, userID string) string {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil || u == nil {
		return "用户不存在。"
	}
	if u.Role == "admin" {
		return "管理员账号受保护，不可禁用/删除。"
	}
	if first, _ := s.repo.User.FirstAdmin(ctx); first != nil && first.ID == u.ID {
		return "默认管理员账号受保护，不可禁用/删除。"
	}
	return ""
}

func (s *TelegramBotService) replyDevicePolicy(ctx context.Context) telegramCommandReply {
	cfg := loadBotConfig(ctx, s.repo)
	text := fmt.Sprintf(
		"<b>设备策略</b>\n\n① 防共享（警告制）：<b>%s</b>\n   并发播放上限 %d / 登录客户端上限 %d / 警告 %d 次后删号\n\n② 不活跃清理：<b>%s</b>\n   随机 %d~%d 天窗口观看 < %d 小时则删号（新号 %d 天宽限）\n\n两套策略默认关闭、互不干扰；删号前会先通过 Bot 通知用户；管理员/受保护账号永不自动处理。",
		onOff(cfg.AntiShareEnabled), cfg.MaxConcurrentPlay, cfg.MaxLoggedClients, cfg.WarnThreshold,
		onOff(cfg.InactiveEnabled), cfg.InactiveWindowMin, cfg.InactiveWindowMax, cfg.InactiveMinHours, cfg.InactiveGraceDays)
	return telegramCommandReply{
		Text: text,
		Buttons: [][]telegramInlineButton{
			{{Text: toggleLabel("防共享", cfg.AntiShareEnabled), Data: "dp_toggle:antishare"}},
			{{Text: toggleLabel("不活跃清理", cfg.InactiveEnabled), Data: "dp_toggle:inactive"}},
			{{Text: "⬅️ 返回菜单", Data: "menu_main"}},
		},
	}
}

func (s *TelegramBotService) replyDevicePolicyToggle(ctx context.Context, which string) telegramCommandReply {
	cfg := loadBotConfig(ctx, s.repo)
	switch which {
	case "antishare":
		_ = s.repo.Setting.Set(ctx, SettingAntiShareEnabled, strconv.FormatBool(!cfg.AntiShareEnabled))
	case "inactive":
		_ = s.repo.Setting.Set(ctx, SettingInactiveEnabled, strconv.FormatBool(!cfg.InactiveEnabled))
	}
	return s.replyDevicePolicy(ctx)
}

func onOff(b bool) string {
	return map[bool]string{true: "已开启", false: "已关闭"}[b]
}

func toggleLabel(name string, enabled bool) string {
	if enabled {
		return "关闭" + name
	}
	return "开启" + name
}
