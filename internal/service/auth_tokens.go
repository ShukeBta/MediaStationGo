// Package service — authentication token issuance helpers.
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// IssueToken signs a JWT for the given user (60min validity, includes tier).
func (s *AuthService) IssueToken(u *model.User) (string, error) {
	claims := Claims{
		UserID: u.ID,
		Role:   u.Role,
		Tier:   u.Tier,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(60 * time.Minute)),
			Issuer:    "mediastationgo",
			Subject:   u.ID,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(s.cfg.Secrets.JWTSecret))
}

const (
	ExternalPlaybackTokenPurpose         = "external_play"
	ExternalPlaybackTokenMinDuration     = 15 * time.Minute
	ExternalPlaybackTokenGraceDuration   = 30 * time.Minute
	ExternalPlaybackTokenUnknownDuration = 6 * time.Hour
	ExternalPlaybackTokenMaxDuration     = 24 * time.Hour
)

func ExternalPlaybackTokenDurationForMedia(durationSec int) time.Duration {
	if durationSec <= 0 {
		return ExternalPlaybackTokenUnknownDuration
	}
	duration := time.Duration(durationSec)*time.Second + ExternalPlaybackTokenGraceDuration
	if duration < ExternalPlaybackTokenMinDuration {
		return ExternalPlaybackTokenMinDuration
	}
	if duration > ExternalPlaybackTokenMaxDuration {
		return ExternalPlaybackTokenMaxDuration
	}
	return duration
}

// IssueExternalPlaybackToken signs a short-lived, media-scoped JWT for URLs
// that are handed to third-party players. It must not be accepted as a
// reusable account/session token for arbitrary media playback.
func (s *AuthService) IssueExternalPlaybackToken(u *model.User, mediaID string, durationSec int) (string, error) {
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return "", errors.New("media id required")
	}
	claims := Claims{
		UserID:  u.ID,
		Role:    u.Role,
		Tier:    u.Tier,
		Purpose: ExternalPlaybackTokenPurpose,
		MediaID: mediaID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ExternalPlaybackTokenDurationForMedia(durationSec))),
			Issuer:    "mediastationgo",
			Subject:   u.ID,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(s.cfg.Secrets.JWTSecret))
}

// EmbyTokenDuration 是第三方 Emby/Jellyfin 客户端访问令牌的有效期。
// Emby 协议没有 refresh token 机制——客户端登录一次后把 AccessToken
// 长期保存并反复使用，直到用户主动登出。若给它们签发 60 分钟的普通
// access token，客户端每小时就会掉登录、无法播放、媒体库无法刷新。
// 因此为这些设备签发长期令牌（与 refresh token 一致的 30 天），匹配
// Emby 持久化令牌的语义。
const EmbyTokenDuration = 30 * 24 * time.Hour

// IssueEmbyToken 为第三方客户端（Emby/Jellyfin 兼容层）签发一个长期
// JWT。它与普通 access token 使用相同的密钥与 Claims，因此沿用现有的
// EmbyAuthRequired 校验逻辑，只是有效期更长。
func (s *AuthService) IssueEmbyToken(u *model.User) (string, error) {
	claims := Claims{
		UserID: u.ID,
		Role:   u.Role,
		Tier:   u.Tier,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(EmbyTokenDuration)),
			Issuer:    "mediastationgo",
			Subject:   u.ID,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(s.cfg.Secrets.JWTSecret))
}

// RefreshTokens 使用刷新令牌获取新的令牌对。
func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return s.tokenSvc.Refresh(ctx, refreshToken)
}

// Logout 撤销用户的所有刷新令牌。
func (s *AuthService) Logout(ctx context.Context, userID string) error {
	return s.tokenSvc.RevokeAll(ctx, userID)
}
