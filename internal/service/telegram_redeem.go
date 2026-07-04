package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

var (
	errRegistrationCodeAlreadyUsed = errors.New("registration code already used")
	errRegistrationCodeExpired     = errors.New("registration code expired")
)

func (s *TelegramBotService) cmdRedeem(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: "请发送：<code>/redeem 兑换码</code>\n未绑定账号时自动尝试注册码；已绑定账号时自动尝试续期码。"}
	}
	code := strings.Join(args, " ")
	if s.boundUser(ctx, msg.From.ID) == nil {
		return s.redeemRegisterFlow(ctx, channel, msg, code)
	}
	return s.redeemRenewFlow(ctx, msg, code)
}

func (s *TelegramBotService) cmdRedeemRegister(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: "请发送：<code>/redeem_register 注册兑换码</code>"}
	}
	return s.redeemRegisterFlow(ctx, channel, msg, strings.Join(args, " "))
}

func (s *TelegramBotService) cmdRedeemRenew(ctx context.Context, msg *TelegramMessage, args []string) telegramCommandReply {
	if len(args) == 0 {
		return telegramCommandReply{Text: "请发送：<code>/redeem_renew 续期兑换码</code>"}
	}
	return s.redeemRenewFlow(ctx, msg, strings.Join(args, " "))
}

func (s *TelegramBotService) redeemRegisterFlow(ctx context.Context, channel *model.NotifyChannel, msg *TelegramMessage, raw string) telegramCommandReply {
	if channel == nil {
		channel = s.findChannelForMessage(ctx, msg)
	}
	if dec := s.telegramUserBindDecision(ctx, channel, msg.From.ID); dec != bindAllowed {
		return telegramCommandReply{Text: telegramBindRejectText(dec, "兑换注册账号")}
	}
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
	user, password, claimedCode, err := s.createUserFromRegistrationCode(ctx, rc.Code)
	if err != nil {
		if errors.Is(err, errRegistrationCodeAlreadyUsed) {
			return telegramCommandReply{Text: "兑换码刚刚被使用，请换一个。"}
		}
		if errors.Is(err, errRegistrationCodeExpired) {
			return telegramCommandReply{Text: "兑换码已过期。"}
		}
		if errors.Is(err, ErrUserLimitReached) {
			return telegramCommandReply{Text: "注册失败：用户数量已达授权上限。"}
		}
		return telegramCommandReply{Text: "注册失败：" + err.Error()}
	}
	if claimedCode == nil {
		return telegramCommandReply{Text: "兑换码刚刚被使用，请换一个。"}
	}
	_ = s.upsertTelegramBinding(ctx, msg, user.ID)
	return telegramCommandReply{
		Text: fmt.Sprintf("兑换成功并已创建账号：\n用户名：<b>%s</b>\n密码：<b>%s</b>\n到期：<b>%s</b>\n\n请尽快用「改用户名/改密码」修改为你自己的凭据。",
			user.Username, password, formatExpiry(s.userExpiry(ctx, user.ID))),
		Buttons: [][]telegramInlineButton{{{Text: "⬅️ 返回菜单", Data: "menu_main"}}},
	}
}

func (s *TelegramBotService) createUserFromRegistrationCode(ctx context.Context, rawCode string) (*model.User, string, *model.RegistrationCode, error) {
	code := normalizeRedemptionCode(rawCode)
	if code == "" {
		return nil, "", nil, errRegistrationCodeAlreadyUsed
	}
	password := randomCode(10)
	var created model.User
	var claimed model.RegistrationCode
	err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("code = ? AND kind = ? AND used_at IS NULL AND used_count < CASE WHEN max_uses > 0 THEN max_uses ELSE 1 END", code, model.RegistrationCodeRegister).
			First(&claimed).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errRegistrationCodeAlreadyUsed
			}
			return err
		}
		if claimed.IsExpired() {
			return errRegistrationCodeExpired
		}
		var count int64
		if err := tx.Model(&model.User{}).Count(&count).Error; err != nil {
			return err
		}
		if count >= LicensedMaxUsers(ctx, s.repo) {
			return ErrUserLimitReached
		}
		hash, err := hashPassword(password)
		if err != nil {
			return err
		}
		codePrefix := strings.ToLower(claimed.Code)
		if len(codePrefix) > 8 {
			codePrefix = codePrefix[:8]
		}
		created = model.User{
			Username:     "u" + codePrefix,
			PasswordHash: hash,
			Role:         "user",
			Tier:         "free",
			HideAdult:    true,
			ExpiredAt:    renewExpiry(nil, claimed.DurationDays),
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		if err := tx.Create(DefaultPermissions(created.ID)).Error; err != nil {
			return err
		}
		now := time.Now()
		res := tx.Model(&model.RegistrationCode{}).
			Where("id = ? AND used_at IS NULL AND used_count < CASE WHEN max_uses > 0 THEN max_uses ELSE 1 END", claimed.ID).
			Updates(map[string]any{
				"used_by_user_id": created.ID,
				"used_count":      gorm.Expr("used_count + 1"),
				"used_at":         gorm.Expr("CASE WHEN used_count + 1 >= CASE WHEN max_uses > 0 THEN max_uses ELSE 1 END THEN ? ELSE used_at END", now),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errRegistrationCodeAlreadyUsed
		}
		claimed.UsedByUserID = created.ID
		claimed.UsedCount++
		if claimed.UsedCount >= claimed.EffectiveMaxUses() {
			claimed.UsedAt = &now
		}
		return nil
	})
	if err != nil {
		return nil, "", nil, err
	}
	return &created, password, &claimed, nil
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
