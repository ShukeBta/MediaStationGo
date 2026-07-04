package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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

func (s *TelegramBotService) cmdGenCode(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	if len(args) < 2 {
		return telegramCommandReply{Text: "用法：<code>/gencode register|renew 天数 [有效天数] [可用次数]</code>\n示例：<code>/gencode register 30</code>、<code>/gencode renew 90 7 5</code>"}
	}
	kind := strings.ToLower(strings.TrimSpace(args[0]))
	switch kind {
	case "reg", "register", "注册码":
		kind = model.RegistrationCodeRegister
	case "renew", "续期", "续期码":
		kind = model.RegistrationCodeRenew
	default:
		return telegramCommandReply{Text: "类型无效，只支持 register / renew。"}
	}
	days, err := strconv.Atoi(args[1])
	if err != nil || days < 0 {
		return telegramCommandReply{Text: "天数必须是非负整数，0 表示永久。"}
	}
	validDays := 0
	if len(args) > 2 {
		validDays, err = strconv.Atoi(args[2])
		if err != nil || validDays < 0 {
			return telegramCommandReply{Text: "有效天数必须是非负整数。"}
		}
	}
	maxUses := 1
	if len(args) > 3 {
		maxUses, err = strconv.Atoi(args[3])
		if err != nil || maxUses <= 0 {
			return telegramCommandReply{Text: "可用次数必须是正整数。"}
		}
	}
	createdBy := ""
	if u := s.boundUser(ctx, msg.From.ID); u != nil {
		createdBy = u.ID
	}
	code, err := s.generateCodeWithUses(ctx, kind, days, validDays, maxUses, createdBy)
	if err != nil {
		return telegramCommandReply{Text: "生成失败：" + err.Error()}
	}
	kindLabel := map[string]string{model.RegistrationCodeRegister: "注册码", model.RegistrationCodeRenew: "续期码"}[code.Kind]
	dur := "永久"
	if days > 0 {
		dur = fmt.Sprintf("%d 天", days)
	}
	valid := "长期有效"
	if validDays > 0 && code.ExpiresAt != nil {
		valid = "有效至 " + code.ExpiresAt.Format("2006-01-02 15:04")
	}
	uses := "单次使用"
	if code.EffectiveMaxUses() > 1 {
		uses = fmt.Sprintf("最多 %d 次", code.EffectiveMaxUses())
	}
	return telegramCommandReply{Text: fmt.Sprintf("已生成%s（%s，%s，%s）：\n\n<code>%s</code>", kindLabel, dur, valid, uses, code.Code)}
}
