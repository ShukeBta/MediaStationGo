package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *TelegramBotService) telegramUserIsAdmin(ctx context.Context, channel *model.NotifyChannel, telegramUserID int) bool {
	if s.telegramUserIDConfigured(channel, telegramUserID) {
		return true
	}
	binding := s.telegramBinding(ctx, telegramUserID)
	if binding == nil {
		return false
	}
	user, err := s.repo.User.FindByID(ctx, binding.UserID)
	return err == nil && user != nil && user.Role == "admin" && user.IsActive
}

// telegramBindDecision 表示成员资格校验的三态结果：通过 / 明确不通过 /
// 无法验证（getChatMember 出错，如 Bot 不在群、群 ID 失效、网络或代理不可达）。
// 区分「明确不是成员」和「查不了」，是为了避免把验证失败误报成「你不在群」。
type telegramBindDecision int

const (
	bindDenied       telegramBindDecision = iota // 已查实：不在任何绑定群组/频道
	bindAllowed                                  // 管理员，或查实是某绑定群组/频道成员
	bindUnverifiable                             // 配了群组/频道但 getChatMember 全部失败
)

// telegramMembership 表示单个 chat 的成员资格三态。
type telegramMembership int

const (
	membershipNo      telegramMembership = iota // 查实不是成员（left/kicked 等）
	membershipYes                               // 查实是成员
	membershipUnknown                           // getChatMember 出错，无法判定
)

func (s *TelegramBotService) telegramUserBindDecision(ctx context.Context, channel *model.NotifyChannel, telegramUserID int) telegramBindDecision {
	if telegramUserID == 0 || channel == nil {
		return bindDenied
	}
	if s.telegramUserIDConfigured(channel, telegramUserID) {
		return bindAllowed
	}
	chatIDs := s.telegramMembershipChatIDs(channel)
	if len(chatIDs) == 0 {
		return bindDenied
	}
	sawUnknown := false
	for _, chatID := range chatIDs {
		switch s.telegramChatMembership(ctx, channel, chatID, telegramUserID) {
		case membershipYes:
			return bindAllowed
		case membershipUnknown:
			sawUnknown = true
		}
	}
	if sawUnknown {
		return bindUnverifiable
	}
	return bindDenied
}

// telegramUserCanBind 是 telegramUserBindDecision 的布尔包装，供尽力而为的场景
// 使用（如私聊时挑选可用渠道）：只有查实通过才返回 true。
func (s *TelegramBotService) telegramUserCanBind(ctx context.Context, channel *model.NotifyChannel, telegramUserID int) bool {
	return s.telegramUserBindDecision(ctx, channel, telegramUserID) == bindAllowed
}

// telegramBindRejectText 根据三态结果生成面向用户的提示。action 形如「兑换注册账号」
// 「绑定媒体中心账号」。bindUnverifiable 时不再误导用户「你不在群」，而是提示
// 管理员检查 Bot 权限与群组 ID。
func telegramBindRejectText(decision telegramBindDecision, action string) string {
	if decision == bindUnverifiable {
		return fmt.Sprintf("暂时无法验证你的群组/频道成员身份，%s未成功。这通常是因为 Bot 未加入绑定群组、在频道中不是管理员，或群组 ID 配置有误（如超级群需带 -100 前缀）。请联系管理员检查 Bot 权限与「绑定群组/频道 ID」。", action)
	}
	return fmt.Sprintf("当前 Telegram 账号不在管理员配置的绑定群组/频道中，无法%s。请先加入管理员配置的群组或频道；如果尚未配置，请联系管理员。", action)
}

func (s *TelegramBotService) telegramChatMembership(ctx context.Context, channel *model.NotifyChannel, chatID string, telegramUserID int) telegramMembership {
	cfg := s.telegramChannelConfig(channel)
	if strings.TrimSpace(cfg["bot_token"]) == "" || chatID == "" || telegramUserID == 0 {
		return membershipUnknown
	}
	payload := map[string]interface{}{
		"chat_id": chatID,
		"user_id": telegramUserID,
	}
	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := telegramPostJSONDecode(ctx, cfg, "getChatMember", payload, 15*time.Second, &result); err != nil {
		s.log.Warn("telegram getChatMember failed", zap.String("chat_id", chatID), zap.Int("telegram_user_id", telegramUserID), zap.Error(sanitizeTelegramError(err)))
		return membershipUnknown
	}
	if !result.OK {
		return membershipUnknown
	}
	switch strings.ToLower(result.Result.Status) {
	case "creator", "administrator", "member", "restricted":
		return membershipYes
	default:
		return membershipNo
	}
}

// telegramUserIsChatMember 是 telegramChatMembership 的布尔包装，仅在查实是成员时
// 返回 true（查不了也视为非成员，供尽力而为的场景使用）。
func (s *TelegramBotService) telegramUserIsChatMember(ctx context.Context, channel *model.NotifyChannel, chatID string, telegramUserID int) bool {
	return s.telegramChatMembership(ctx, channel, chatID, telegramUserID) == membershipYes
}
