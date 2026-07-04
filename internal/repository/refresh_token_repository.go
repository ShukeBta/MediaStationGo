package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// RefreshTokenRepository persists model.RefreshToken records.
type RefreshTokenRepository struct{ db *gorm.DB }

// Create inserts a new refresh token record.
func (r *RefreshTokenRepository) Create(ctx context.Context, t *model.RefreshToken) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Create(t).Error
	})
}

// FindByHash returns the refresh token matching the hash, or (nil, nil).
func (r *RefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	err := withSQLiteBusyRetry(ctx, func() error {
		t = model.RefreshToken{}
		return r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&t).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// RevokeByUserID revokes all refresh tokens for a user.
func (r *RefreshTokenRepository) RevokeByUserID(ctx context.Context, userID string) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Model(&model.RefreshToken{}).
			Where("user_id = ?", userID).Update("revoked", true).Error
	})
}

// RevokeOldestActiveByUserID keeps at most limit active refresh tokens for a
// user by revoking the oldest non-expired, non-revoked tokens.
func (r *RefreshTokenRepository) RevokeOldestActiveByUserID(ctx context.Context, userID string, limit int) error {
	if limit < 1 {
		limit = 1
	}
	return withSQLiteBusyRetry(ctx, func() error {
		var tokens []model.RefreshToken
		if err := r.db.WithContext(ctx).
			Where("user_id = ? AND revoked = ? AND expires_at > ?", userID, false, time.Now()).
			Order("created_at desc, id desc").
			Find(&tokens).Error; err != nil {
			return err
		}
		if len(tokens) <= limit {
			return nil
		}
		ids := make([]string, 0, len(tokens)-limit)
		for _, token := range tokens[limit:] {
			ids = append(ids, token.ID)
		}
		return r.db.WithContext(ctx).Model(&model.RefreshToken{}).
			Where("id IN ?", ids).Update("revoked", true).Error
	})
}

// DeleteExpired removes all expired refresh tokens.
func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(&model.RefreshToken{}).Error
	})
}

// Revoke revokes a specific refresh token.
func (r *RefreshTokenRepository) Revoke(ctx context.Context, hash string) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Model(&model.RefreshToken{}).
			Where("token_hash = ?", hash).Update("revoked", true).Error
	})
}

// HashToken returns the SHA256 hash of a token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}
