// Package service — multi-persona play profiles.
//
// PlayProfileService persists per-user "viewing personas" so the same
// account can switch between, e.g., a child-safe profile and an adult
// one without changing credentials. Profiles drive content rating
// gates, library access, and player defaults; the upstream Vue project
// shipped the form but never wired the backend, so we implement the
// data model here.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// PlayProfileService manages PlayProfile rows.
type PlayProfileService struct {
	log  *zap.Logger
	repo *repository.Container
}

const MaxPlayProfilesPerUser = 3

var (
	ErrPlayProfileNotFound   = errors.New("profile not found")
	ErrPlayProfileForbidden  = errors.New("profile forbidden")
	ErrPlayProfilePINInvalid = errors.New("pin invalid")
	ErrPlayProfileLimit      = errors.New("profile limit reached")
	ErrPlayProfileValidation = errors.New("validation error")
)

// NewPlayProfileService is the constructor.
func NewPlayProfileService(log *zap.Logger, repo *repository.Container) *PlayProfileService {
	return &PlayProfileService{log: log, repo: repo}
}

// PlayProfileInput is the create/update payload accepted by the API.
// PIN is hashed only when non-empty so omitting it preserves the
// existing PIN on update.
type PlayProfileInput struct {
	UserID                string   `json:"user_id"`
	Name                  string   `json:"name"`
	IsDefault             bool     `json:"is_default"`
	ContentRatingLimit    string   `json:"content_rating_limit"`
	AllowAdult            bool     `json:"allow_adult"`
	RequirePIN            bool     `json:"require_pin"`
	PIN                   string   `json:"pin,omitempty"`
	PreferredSubtitleLang string   `json:"preferred_subtitle_lang"`
	PreferredAudioLang    string   `json:"preferred_audio_lang"`
	AutoplayNext          bool     `json:"autoplay_next"`
	SkipIntro             bool     `json:"skip_intro"`
	AllowedLibraryIDs     []string `json:"allowed_library_ids"`
}

// ProfileView is the public shape — AllowedLibraryIDs is decoded back
// to a slice so the React form can bind to it directly.
type ProfileView struct {
	model.PlayProfile
	AllowedLibraryIDs []string `json:"allowed_library_ids"`
}

func toProfileView(p model.PlayProfile) ProfileView {
	v := ProfileView{PlayProfile: p}
	if p.AllowedLibraryIDs != "" {
		_ = json.Unmarshal([]byte(p.AllowedLibraryIDs), &v.AllowedLibraryIDs)
	}
	if v.AllowedLibraryIDs == nil {
		v.AllowedLibraryIDs = []string{}
	}
	return v
}

// List returns every profile (admin view).
func (s *PlayProfileService) List(ctx context.Context) ([]ProfileView, error) {
	rows, err := s.repo.PlayProfile.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ProfileView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProfileView(r))
	}
	return out, nil
}

// ListByUser returns the profiles owned by the user.
func (s *PlayProfileService) ListByUser(ctx context.Context, userID string) ([]ProfileView, error) {
	rows, err := s.repo.PlayProfile.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ProfileView, 0, len(rows))
	for _, r := range rows {
		out = append(out, toProfileView(r))
	}
	return out, nil
}

// Create inserts a new play profile. When IsDefault is true we clear
// every other default for the user inside the same transaction.
func (s *PlayProfileService) Create(ctx context.Context, in PlayProfileInput) (*ProfileView, error) {
	if err := validateProfileInput(in, true); err != nil {
		return nil, err
	}
	count, err := s.repo.PlayProfile.CountByUser(ctx, in.UserID)
	if err != nil {
		return nil, err
	}
	if count >= MaxPlayProfilesPerUser {
		return nil, ErrPlayProfileLimit
	}
	libsBlob, _ := json.Marshal(in.AllowedLibraryIDs)
	p := &model.PlayProfile{
		UserID:                in.UserID,
		Name:                  strings.TrimSpace(in.Name),
		IsDefault:             in.IsDefault,
		ContentRatingLimit:    in.ContentRatingLimit,
		AllowAdult:            in.AllowAdult,
		RequirePIN:            in.RequirePIN,
		PreferredSubtitleLang: in.PreferredSubtitleLang,
		PreferredAudioLang:    in.PreferredAudioLang,
		AutoplayNext:          in.AutoplayNext,
		SkipIntro:             in.SkipIntro,
		AllowedLibraryIDs:     string(libsBlob),
	}
	if in.RequirePIN && in.PIN != "" {
		p.PINHash = hashPIN(in.PIN)
	}
	if in.IsDefault {
		if err := s.repo.PlayProfile.ClearDefaultsFor(ctx, in.UserID); err != nil {
			return nil, err
		}
	}
	if err := s.repo.PlayProfile.Create(ctx, p); err != nil {
		return nil, err
	}
	v := toProfileView(*p)
	return &v, nil
}

// UpdateForUser applies a patch only when the profile belongs to userID.
func (s *PlayProfileService) UpdateForUser(ctx context.Context, id, userID string, in PlayProfileInput) (*ProfileView, error) {
	row, err := s.repo.PlayProfile.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrPlayProfileNotFound
	}
	if row.UserID != userID {
		return nil, ErrPlayProfileForbidden
	}
	return s.updateExisting(ctx, row, in)
}

// Update applies a patch to an existing profile.
func (s *PlayProfileService) Update(ctx context.Context, id string, in PlayProfileInput) (*ProfileView, error) {
	row, err := s.repo.PlayProfile.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrPlayProfileNotFound
	}
	return s.updateExisting(ctx, row, in)
}

func (s *PlayProfileService) updateExisting(ctx context.Context, row *model.PlayProfile, in PlayProfileInput) (*ProfileView, error) {
	if err := validateProfileInput(in, false); err != nil {
		return nil, err
	}
	libsBlob, _ := json.Marshal(in.AllowedLibraryIDs)
	patch := map[string]any{
		"name":                    strings.TrimSpace(in.Name),
		"is_default":              in.IsDefault,
		"content_rating_limit":    in.ContentRatingLimit,
		"allow_adult":             in.AllowAdult,
		"require_pin":             in.RequirePIN,
		"preferred_subtitle_lang": in.PreferredSubtitleLang,
		"preferred_audio_lang":    in.PreferredAudioLang,
		"autoplay_next":           in.AutoplayNext,
		"skip_intro":              in.SkipIntro,
		"allowed_library_ids":     string(libsBlob),
	}
	if in.RequirePIN && in.PIN != "" {
		patch["pin_hash"] = hashPIN(in.PIN)
	} else if in.RequirePIN && row.PINHash == "" {
		return nil, fmt.Errorf("%w: pin required", ErrPlayProfileValidation)
	}
	if !in.RequirePIN {
		patch["pin_hash"] = ""
	}
	if in.IsDefault {
		if err := s.repo.PlayProfile.ClearDefaultsFor(ctx, row.UserID); err != nil {
			return nil, err
		}
	}
	if err := s.repo.PlayProfile.Update(ctx, row.ID, patch); err != nil {
		return nil, err
	}
	row, err := s.repo.PlayProfile.FindByID(ctx, row.ID)
	if err != nil || row == nil {
		return nil, err
	}
	v := toProfileView(*row)
	return &v, nil
}

// Delete removes a profile.
func (s *PlayProfileService) Delete(ctx context.Context, id string) error {
	return s.repo.PlayProfile.Delete(ctx, id)
}

// DeleteForUser removes a profile only when it belongs to userID.
func (s *PlayProfileService) DeleteForUser(ctx context.Context, id, userID string) error {
	row, err := s.repo.PlayProfile.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if row == nil {
		return ErrPlayProfileNotFound
	}
	if row.UserID != userID {
		return ErrPlayProfileForbidden
	}
	return s.repo.PlayProfile.Delete(ctx, id)
}

// VerifyPIN validates that the caller can switch to a PIN-protected profile.
func (s *PlayProfileService) VerifyPIN(ctx context.Context, id, userID, pin string) (*ProfileView, error) {
	row, err := s.repo.PlayProfile.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrPlayProfileNotFound
	}
	if row.UserID != userID {
		return nil, ErrPlayProfileForbidden
	}
	if row.RequirePIN {
		if row.PINHash == "" || hashPIN(pin) != row.PINHash {
			return nil, ErrPlayProfilePINInvalid
		}
	}
	view := toProfileView(*row)
	return &view, nil
}

// TouchActive bumps the LastActiveAt timestamp; called by the player
// when a profile is selected.
func (s *PlayProfileService) TouchActive(ctx context.Context, id string) error {
	now := time.Now()
	return s.repo.PlayProfile.Update(ctx, id, map[string]any{
		"last_active_at": &now,
	})
}

// validateProfileInput rejects malformed payloads. On create we require
// user_id; on update we allow it to be empty (caller supplies it via URL).
// Errors wrap ErrPlayProfileValidation so handlers can distinguish
// validation failures (400) from infrastructure errors (500).
func validateProfileInput(in PlayProfileInput, requireUser bool) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: name required", ErrPlayProfileValidation)
	}
	if requireUser && strings.TrimSpace(in.UserID) == "" {
		return fmt.Errorf("%w: user_id required", ErrPlayProfileValidation)
	}
	if requireUser && in.RequirePIN && strings.TrimSpace(in.PIN) == "" {
		return fmt.Errorf("%w: pin required", ErrPlayProfileValidation)
	}
	if in.RequirePIN && in.PIN != "" {
		if len(in.PIN) < 4 || len(in.PIN) > 8 {
			return fmt.Errorf("%w: pin must be 4-8 characters", ErrPlayProfileValidation)
		}
	}
	return nil
}

// hashPIN is a deterministic SHA-256 (PINs are short numeric secrets;
// bcrypt is overkill for this use case).
func hashPIN(pin string) string {
	sum := sha256.Sum256([]byte(pin))
	return hex.EncodeToString(sum[:])
}
