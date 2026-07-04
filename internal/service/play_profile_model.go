package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// PlayProfileInput is the create/update payload accepted by the API.
// PIN is hashed only when non-empty so omitting it preserves the existing PIN on update.
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

// ProfileView is the public shape for React forms.
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

func hashPIN(pin string) string {
	sum := sha256.Sum256([]byte(pin))
	return hex.EncodeToString(sum[:])
}
