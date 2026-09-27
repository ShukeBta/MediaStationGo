// Package service — user profile management.
package service

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// ProfileService handles non-credential user mutations.
type ProfileService struct {
	log  *zap.Logger
	repo *repository.Container
}

var ErrAdultContentBlockedByAdmin = errors.New("成人内容已被管理员屏蔽")

// NewProfileService is the constructor.
func NewProfileService(log *zap.Logger, repo *repository.Container) *ProfileService {
	return &ProfileService{log: log, repo: repo}
}

// ProfileUpdate is the patch object accepted by UpdateProfile. Empty
// fields are ignored so the same payload can be reused across screens.
type ProfileUpdate struct {
	Username  *string `json:"username,omitempty"`
	Nickname  *string `json:"nickname,omitempty"`
	Email     *string `json:"email,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
	HideAdult *bool   `json:"hide_adult,omitempty"`
	Password  string  `json:"password,omitempty"`
}

// UpdateProfile applies a non-credential patch to the user.
func (p *ProfileService) UpdateProfile(ctx context.Context, userID string, patch ProfileUpdate) (*model.User, error) {
	if userID == "" {
		return nil, errors.New("missing user id")
	}
	current, err := p.repo.User.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, errors.New("user not found")
	}
	updates := map[string]any{}
	if patch.Username != nil {
		v := strings.TrimSpace(*patch.Username)
		if v == "" {
			return nil, errors.New("username required")
		}
		if existing, err := p.repo.User.FindByUsername(ctx, v); err != nil {
			return nil, err
		} else if existing != nil && existing.ID != userID {
			return nil, ErrUsernameTaken
		}
		updates["username"] = v
	}
	if patch.Nickname != nil {
		updates["nickname"] = strings.TrimSpace(*patch.Nickname)
	}
	if patch.Email != nil {
		v := strings.TrimSpace(*patch.Email)
		updates["email"] = v
	}
	if patch.AvatarURL != nil {
		updates["avatar_url"] = strings.TrimSpace(*patch.AvatarURL)
	}
	if patch.HideAdult != nil {
		if current.AdultContentBlocked && !*patch.HideAdult {
			return nil, ErrAdultContentBlockedByAdmin
		}
		updates["hide_adult"] = *patch.HideAdult
	}
	if len(updates) > 0 {
		if err := p.repo.DB.Model(&model.User{}).Where("id = ?", userID).
			Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return p.repo.User.FindByID(ctx, userID)
}

// AdminUpdateRole lets administrators promote / demote another user. The
// caller is expected to gate the route with AdminRequired.
func (p *ProfileService) AdminUpdateRole(ctx context.Context, userID, role string) (*model.User, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	if role != "admin" && role != "user" {
		return nil, errors.New("role must be admin or user")
	}
	if firstAdmin, err := p.repo.User.FirstAdmin(ctx); err != nil {
		return nil, err
	} else if firstAdmin != nil && firstAdmin.ID == userID && role != "admin" {
		return nil, errors.New("default admin must keep admin role")
	}
	updates := map[string]any{"role": role}
	if role == "admin" {
		updates["tier"] = "plus"
	}
	if err := p.repo.User.UpdateFields(ctx, userID, updates); err != nil {
		return nil, err
	}
	return p.repo.User.FindByID(ctx, userID)
}
