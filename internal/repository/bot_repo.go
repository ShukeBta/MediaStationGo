package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ─── RegistrationCode ─────────────────────────────────────────────────────────

// RegistrationCodeRepository persists model.RegistrationCode records.
type RegistrationCodeRepository struct{ db *gorm.DB }

// Create inserts a new code.
func (r *RegistrationCodeRepository) Create(ctx context.Context, c *model.RegistrationCode) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// FindByCode returns the code row, or (nil, nil) when absent.
func (r *RegistrationCodeRepository) FindByCode(ctx context.Context, code string) (*model.RegistrationCode, error) {
	var c model.RegistrationCode
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// MarkUsed atomically consumes one use of a redeemable code. It returns
// gorm.ErrRecordNotFound when the code is exhausted so callers can avoid
// double-spend races.
func (r *RegistrationCodeRepository) MarkUsed(ctx context.Context, id, userID string) error {
	now := time.Now()
	res := r.db.WithContext(ctx).Model(&model.RegistrationCode{}).
		Where("id = ? AND used_at IS NULL AND used_count < CASE WHEN max_uses > 0 THEN max_uses ELSE 1 END", id).
		Updates(map[string]any{
			"used_by_user_id": userID,
			"used_count":      gorm.Expr("used_count + 1"),
			"used_at":         gorm.Expr("CASE WHEN used_count + 1 >= CASE WHEN max_uses > 0 THEN max_uses ELSE 1 END THEN ? ELSE used_at END", now),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// List returns the most recent codes (newest first).
func (r *RegistrationCodeRepository) List(ctx context.Context, limit int) ([]model.RegistrationCode, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []model.RegistrationCode
	err := r.db.WithContext(ctx).Order("created_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}

// CountUnused returns the number of codes that are still redeemable.
func (r *RegistrationCodeRepository) CountUnused(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.RegistrationCode{}).
		Where("used_at IS NULL AND used_count < CASE WHEN max_uses > 0 THEN max_uses ELSE 1 END").Count(&n).Error
	return n, err
}

// ─── SignIn ───────────────────────────────────────────────────────────────────

// SignInRepository persists model.SignIn records.
type SignInRepository struct{ db *gorm.DB }

// Get returns the sign-in row for a user, or (nil, nil) when absent.
func (r *SignInRepository) Get(ctx context.Context, userID string) (*model.SignIn, error) {
	var s model.SignIn
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Save inserts or updates a sign-in row.
func (r *SignInRepository) Save(ctx context.Context, s *model.SignIn) error {
	return r.db.WithContext(ctx).Save(s).Error
}

// ─── UserDevice ───────────────────────────────────────────────────────────────

// UserDeviceRepository persists model.UserDevice records.
type UserDeviceRepository struct{ db *gorm.DB }

// Find returns the device row for (user, device), or (nil, nil) when absent.
func (r *UserDeviceRepository) Find(ctx context.Context, userID, deviceID string) (*model.UserDevice, error) {
	var d model.UserDevice
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND device_id = ?", userID, deviceID).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// FindByFingerprint returns the newest row for a terminal fingerprint.
func (r *UserDeviceRepository) FindByFingerprint(ctx context.Context, userID, fingerprint string) (*model.UserDevice, error) {
	var d model.UserDevice
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND fingerprint = ?", userID, fingerprint).
		Order("last_seen_at desc").
		First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// Create inserts a new device row.
func (r *UserDeviceRepository) Create(ctx context.Context, d *model.UserDevice) error {
	return r.db.WithContext(ctx).Create(d).Error
}

// Save persists changes to an existing device row.
func (r *UserDeviceRepository) Save(ctx context.Context, d *model.UserDevice) error {
	return r.db.WithContext(ctx).Save(d).Error
}

// ListByUser returns all device rows for a user, newest activity first.
func (r *UserDeviceRepository) ListByUser(ctx context.Context, userID string) ([]model.UserDevice, error) {
	var rows []model.UserDevice
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("last_seen_at desc").Find(&rows).Error
	return rows, err
}

// CountActiveClients counts distinct terminal devices for a user that were
// seen on or after `since`. Multiple apps on the same terminal share the same
// fingerprint and count as one terminal; rows remain as login channels.
func (r *UserDeviceRepository) CountActiveClients(ctx context.Context, userID string, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.UserDevice{}).
		Where("user_id = ? AND last_seen_at >= ? AND kicked = ?", userID, since, false).
		Select("COUNT(DISTINCT COALESCE(NULLIF(fingerprint, ''), device_id))").Scan(&n).Error
	return n, err
}

// CountConcurrentPlaying counts terminal devices for a user whose last playback
// ping was on or after `since` (used for the max concurrent playback rule).
func (r *UserDeviceRepository) CountConcurrentPlaying(ctx context.Context, userID string, since time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.UserDevice{}).
		Where("user_id = ? AND last_play_at IS NOT NULL AND last_play_at >= ?", userID, since).
		Select("COUNT(DISTINCT COALESCE(NULLIF(fingerprint, ''), device_id))").Scan(&n).Error
	return n, err
}

// Delete removes a single device row by primary key.
func (r *UserDeviceRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Unscoped().Delete(&model.UserDevice{}, "id = ?", id).Error
}

// DeleteByUser removes every device row for a user (used on account deletion).
func (r *UserDeviceRepository) DeleteByUser(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Unscoped().Where("user_id = ?", userID).Delete(&model.UserDevice{}).Error
}

// DeleteByFingerprintExcept removes stale login-channel rows for the same
// terminal, leaving the selected canonical row.
func (r *UserDeviceRepository) DeleteByFingerprintExcept(ctx context.Context, userID, fingerprint, keepID string) error {
	return r.db.WithContext(ctx).Unscoped().
		Where("user_id = ? AND fingerprint = ? AND id <> ?", userID, fingerprint, keepID).
		Delete(&model.UserDevice{}).Error
}

// SetKicked marks a device as kicked (forces re-login on next request).
func (r *UserDeviceRepository) SetKicked(ctx context.Context, id string, kicked bool) error {
	return r.db.WithContext(ctx).Model(&model.UserDevice{}).Where("id = ?", id).
		Update("kicked", kicked).Error
}

// SetKickedByFingerprint marks every row for a terminal fingerprint.
func (r *UserDeviceRepository) SetKickedByFingerprint(ctx context.Context, userID, fingerprint string, kicked bool) error {
	return r.db.WithContext(ctx).Model(&model.UserDevice{}).
		Where("user_id = ? AND fingerprint = ?", userID, fingerprint).
		Update("kicked", kicked).Error
}

// SetKickedByUser marks every device for a user as kicked/un-kicked.
func (r *UserDeviceRepository) SetKickedByUser(ctx context.Context, userID string, kicked bool) error {
	return r.db.WithContext(ctx).Model(&model.UserDevice{}).Where("user_id = ?", userID).
		Update("kicked", kicked).Error
}

// WatchedMillisSince approximates the total watched milliseconds for a user
// since `since`, using the last known playback position per media. Playback
// history keeps one row per (user, media), so this is an activity proxy rather
// than an exact watch-time integral — sufficient for the inactivity rule.
func (r *UserDeviceRepository) WatchedMillisSince(ctx context.Context, userID string, since time.Time) (int64, error) {
	var total int64
	err := r.db.WithContext(ctx).Model(&model.PlaybackHistory{}).
		Where("user_id = ? AND watched_at >= ?", userID, since).
		Select("COALESCE(SUM(position_ms), 0)").Scan(&total).Error
	return total, err
}
