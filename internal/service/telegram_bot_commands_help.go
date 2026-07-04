package service

import "context"

// cmdHelp 处理 /help 命令。
func (s *TelegramBotService) cmdHelp(ctx context.Context, msg *TelegramMessage) string {
	channel := s.findChannelForMessage(ctx, msg)
	if telegramIsGroupChat(msg.Chat.Type) {
		adminHint := ""
		if s.telegramUserIsAdmin(ctx, channel, msg.From.ID) {
			adminHint = "\n\n管理员可在已绑定群组直接发送文本管理命令；管理面板和账号凭据操作请私聊 Bot。"
		}
		return "<b>MediaStationGo 群组可用命令</b>\n\n" +
			"<b>/menu</b> — 打开群组自助菜单\n" +
			"<b>/account</b> — 查看账号状态\n" +
			"<b>/signin</b> — 签到\n" +
			"<b>/devices</b> — 查看登录设备\n" +
			"<b>/kick all|编号</b> — 踢下线设备\n" +
			"<b>/hideadult on|off</b> — 隐藏或显示成人目录\n\n" +
			"绑定、注册、兑换、改名、改密等包含敏感信息的操作请私聊 Bot。" +
			adminHint
	}
	if !s.telegramUserIsAdmin(ctx, channel, msg.From.ID) {
		register := ""
		if s.openRegEnabled(ctx) {
			register = "<b>/register 用户名 密码</b> — 注册新账号\n"
		}
		return "<b>MediaStationGo 用户命令</b>\n\n" +
			register +
			"<b>/start 用户名 密码</b> — 绑定账号\n" +
			"<b>/account</b> — 查看账号状态\n" +
			"<b>/signin</b> — 签到\n" +
			"<b>/devices</b> — 查看登录设备\n" +
			"<b>/kick all|编号</b> — 踢下线设备\n" +
			"<b>/setname 当前密码 新用户名</b> — 修改用户名\n" +
			"<b>/setpass 当前密码 新密码</b> — 修改密码\n" +
			"<b>/redeem 兑换码</b> — 注册或续期兑换\n" +
			"<b>/hideadult on|off</b> — 隐藏或显示成人目录\n\n" +
			"系统状态、搜索、下载列表与统计命令仅管理员可用。"
	}
	return "<b>MediaStationGo 命令列表</b>\n\n" +
		"<b>/start</b> — 开始使用\n" +
		"<b>/help</b> — 帮助信息\n" +
		"<b>/account</b> / <b>/devices</b> / <b>/kick all|编号</b> — 用户自助设备管理\n" +
		"<b>/signin</b> / <b>/redeem 兑换码</b> — 签到与兑换\n" +
		"<b>/setname 当前密码 新用户名</b> / <b>/setpass 当前密码 新密码</b> — 用户自助改名改密\n" +
		"<b>/register 用户名 密码</b> — 注册新账号（需管理员开启）\n" +
		"<b>/registration on [名额]|off</b> — 开启/关闭普通用户注册（管理员）\n" +
		"<b>/capacity</b> / <b>/users</b> — 容量与用户管理（管理员）\n" +
		"<b>/gencode register|renew 天数 [有效天数]</b> — 生成兑换码（管理员）\n" +
		"<b>/renew_user 用户名 天数</b> / <b>/delete_user 用户名 confirm</b> — 续期/删除用户（管理员）\n" +
		"<b>/unbind 用户1 用户2</b> — 批量解绑 Telegram 绑定（管理员）\n" +
		"<b>/unbind_duplicates</b> / <b>/unbind_inactive 天数</b> — 清理重复/无效绑定或久未登录绑定（管理员）\n" +
		"<b>/antishare on play=3 login=3 warn=2</b> — 防共享策略（管理员）\n" +
		"<b>/cleanup run</b> — 预览保号清理候选（管理员）\n" +
		"<b>/cleanup run confirm</b> — 确认清理候选账号（管理员）\n" +
		"<b>/cleanup on|off</b> — 保号规则开关（管理员）\n" +
		"<b>/cleanup_rule list|add|edit|修改|del|enable|disable</b> — Mgo 保号规则（管理员）\n" +
		"<b>/ban 用户名</b> / <b>/unban 用户名</b> — 禁用/解禁用户（管理员）\n" +
		"<b>/hideadult on|off</b> — 隐藏/显示当前绑定账号的成人目录\n" +
		"<b>/status</b> — 系统运行状态\n" +
		"<b>/search 关键词</b> — 搜索媒体库\n" +
		"<b>/downloads</b> — 下载列表\n" +
		"<b>/stats</b> — 媒体库统计\n\n" +
		telegramMgoAdminCommandHelp() + "\n\n" +
		"<b>自动推送事件：</b>\n" +
		"• 订阅命中新资源\n" +
		"• 下载任务完成\n" +
		"• 刮削失败告警\n" +
		"• 系统异常通知"
}

func telegramMgoAdminCommandHelp() string {
	return "<b>Mgo 管理命令（管理员可用，已注册到命令栏）：</b>\n" +
		"用户：<code>/ucr 用户名 密码 [天数]</code> 创建账号；<code>/uinfo 用户名</code> 查询账号；<code>/rmemby 用户名 confirm</code> 删除账号；<code>/only_rm_record tg:ID|用户名</code> 仅删 Bot 绑定；<code>/renewall 天数 confirm</code> 批量续期。\n" +
		"审计：<code>/userip 用户名</code> 查用户 IP；<code>/auditip IP</code> 按 IP 审计；<code>/auditdevice 关键词</code> 按终端设备审计；<code>/auditclient 关键词</code> 按客户端审计；<code>/udeviceid 设备ID</code> 按设备指纹审计。\n" +
		"清理：<code>/syncunbound</code> 检查未绑定账号；<code>/syncgroupm</code> 校验群成员；<code>/check_ex</code> 检查过期账号；<code>/deleted</code> 按保号规则预览清理候选。\n" +
		"权限：<code>/embyadmin 用户名 on|off</code> 设置管理员；<code>/banall confirm</code>/<code>/unbanall confirm</code> 批量禁用/解禁；<code>/prouser 用户名</code>/<code>/revuser 用户名</code> 管理保护名单；<code>/embylibs_blockall</code>/<code>/embylibs_unblockall</code> 批量禁用/开放媒体库权限。\n" +
		"运维：<code>/proadmin TelegramID</code>/<code>/revadmin TelegramID</code> 管理 Bot 管理员；<code>/backup_db</code> 备份数据库；<code>/restore_from_db 文件名 confirm</code> 恢复数据库。\n" +
		"说明：重复别名如 <code>/low_activity</code>、<code>/urm</code> 仍可兼容识别，但不显示在命令栏。"
}
