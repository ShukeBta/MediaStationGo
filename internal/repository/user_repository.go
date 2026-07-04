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

// UserRepository persists model.User records.
type UserRepository struct{ db *gorm.DB }

// Create inserts a new user. Caller must pre-hash the password.
func (r *UserRepository) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// ReleaseDeletedUsername renames soft-deleted rows that still hold a unique
// username so the same account name can be created again.
func (r *UserRepository) ReleaseDeletedUsername(ctx context.Context, username string) error {
	if username == "" {
		return nil
	}
	released := username + "__deleted__" + time.Now().Format("20060102150405.000000000")
	if len(released) > 64 {
		sum := sha256.Sum256([]byte(released))
		released = username
		if len(released) > 43 {
			released = released[:43]
		}
		released += "__deleted__" + hex.EncodeToString(sum[:])[:10]
	}
	return r.db.WithContext(ctx).Unscoped().
		Model(&model.User{}).
		Where("username = ? AND deleted_at IS NOT NULL", username).
		Update("username", released).Error
}

// FindByUsername returns the user matching username, or (nil, nil) when absent.
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := withSQLiteBusyRetry(ctx, func() error {
		u = model.User{}
		err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
		if errors.Is(err, gorm.ErrRecordNotFound) && username != "" {
			err = r.db.WithContext(ctx).Where("LOWER(username) = LOWER(?)", username).First(&u).Error
		}
		return err
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByID returns the user with the matching primary key, or (nil, nil).
func (r *UserRepository) FindByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := withSQLiteBusyRetry(ctx, func() error {
		u = model.User{}
		return r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Count returns the total number of non-deleted users.
func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Count(&n).Error
	return n, err
}

// CountAdmins returns the number of users that hold the admin role.
func (r *UserRepository) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("role = ?", "admin").Count(&n).Error
	return n, err
}

// FirstAdmin returns the earliest admin user. This row represents the protected
// built-in/default administrator even if its username is later changed.
func (r *UserRepository) FirstAdmin(ctx context.Context) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("role = ?", "admin").Order("created_at asc").First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns all users ordered by creation time desc.
func (r *UserRepository) List(ctx context.Context) ([]model.User, error) {
	var users []model.User
	err := r.db.WithContext(ctx).Order("created_at desc").Find(&users).Error
	return users, err
}

// UpdateFields applies a narrow set of user field updates.
func (r *UserRepository) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// UpdatePassword sets a new password hash and clears ForcePasswordReset.
func (r *UserRepository) UpdatePassword(ctx context.Context, id, hash string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Updates(map[string]any{"password_hash": hash, "force_password_reset": false}).Error
}

// TouchLogin updates the last login timestamp.
func (r *UserRepository) TouchLogin(ctx context.Context, id string) error {
	now := time.Now()
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
			Update("last_login_at", &now).Error
	})
}

// Delete removes a user (soft-delete via gorm.DeletedAt), releases the unique
// username, and drops Telegram bindings so future re-created users bind cleanly.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("user_id = ?", id).Delete(&model.TelegramBinding{}).Error; err != nil {
			return err
		}
		released := user.Username + "__deleted__" + time.Now().Format("20060102150405.000000000")
		if len(released) > 64 {
			sum := sha256.Sum256([]byte(user.ID + user.Username))
			base := user.Username
			if len(base) > 43 {
				base = base[:43]
			}
			released = base + "__deleted__" + hex.EncodeToString(sum[:])[:10]
		}
		if err := tx.Model(&model.User{}).Where("id = ?", id).Update("username", released).Error; err != nil {
			return err
		}
		return tx.Delete(&model.User{}, "id = ?", id).Error
	})
}
