package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func mediaVisibilityForRequest(c *gin.Context, svc *service.Container) service.MediaVisibility {
	userID := currentUserID(c)
	profile, locked := selectedPlayProfile(c, svc)
	if locked {
		return service.MediaVisibility{
			IncludeNSFW:       false,
			AllowedLibraryIDs: []string{"__locked__"},
		}
	}
	if profile == nil {
		return service.UserDefaultMediaVisibility(c.Request.Context(), svc.Repo, userID)
	}
	adultEnabled := service.AdultContentEnabled(c.Request.Context(), svc.Repo)
	userHidesAdult := service.UserHidesAdult(c.Request.Context(), svc.Repo, userID)
	visibility := service.MediaVisibility{}
	visibility.IncludeNSFW = adultEnabled && profile.AllowAdult && !userHidesAdult
	visibility.AllowedLibraryIDs = service.CombineAllowedLibraryIDs(
		c.Request.Context(),
		svc.Repo,
		service.UserAllowedLibraryIDs(c.Request.Context(), svc.Repo, userID),
		profileAllowedLibraryIDs(*profile),
	)
	return visibility
}

func selectedPlayProfile(c *gin.Context, svc *service.Container) (*model.PlayProfile, bool) {
	if svc == nil || svc.Repo == nil || svc.Repo.PlayProfile == nil {
		return nil, false
	}
	userID := currentUserID(c)
	if userID == "" {
		return nil, false
	}
	profileID := strings.TrimSpace(c.GetHeader("X-Play-Profile-ID"))
	if profileID == "" {
		profileID = strings.TrimSpace(c.Query("profile_id"))
	}
	if profileID != "" {
		profile, err := svc.Repo.PlayProfile.FindByID(c.Request.Context(), profileID)
		if err == nil && profile != nil && profile.UserID == userID {
			if profile.RequirePIN && !validPlayProfilePINToken(c, svc, userID, profile.ID) {
				return nil, true
			}
			return profile, false
		}
	}
	rows, err := svc.Repo.PlayProfile.ListByUser(c.Request.Context(), userID)
	if err != nil {
		return nil, false
	}
	for i := range rows {
		if rows[i].IsDefault {
			if rows[i].RequirePIN && !validPlayProfilePINToken(c, svc, userID, rows[i].ID) {
				return nil, true
			}
			return &rows[i], false
		}
	}
	return nil, false
}

func mediaVisibleForRequest(c *gin.Context, svc *service.Container, media *model.Media) bool {
	return mediaVisibilityForRequest(c, svc).Allows(media)
}

func settingBool(c *gin.Context, svc *service.Container, key string, fallback bool) bool {
	if svc == nil || svc.Repo == nil || svc.Repo.Setting == nil {
		return fallback
	}
	value, err := svc.Repo.Setting.Get(c.Request.Context(), key)
	if err != nil {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled", "启用", "开启":
		return true
	case "0", "false", "no", "off", "disabled", "禁用", "关闭", "":
		return false
	default:
		return fallback
	}
}

func currentUserID(c *gin.Context) string {
	uid, _ := c.Get(middleware.CtxUserID)
	return toString(uid)
}

func profileAllowedLibraryIDs(profile model.PlayProfile) []string {
	return service.DecodeAllowedLibraryIDs(profile.AllowedLibraryIDs)
}

func signPlayProfilePINToken(svc *service.Container, userID, profileID string, expiresAt time.Time) string {
	if svc == nil || svc.Cfg == nil {
		return ""
	}
	payload := fmt.Sprintf("%s|%s|%d", userID, profileID, expiresAt.Unix())
	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	signature := playProfilePINSignature(svc.Cfg.Secrets.JWTSecret, encodedPayload)
	if signature == "" {
		return ""
	}
	return encodedPayload + "." + signature
}

func validPlayProfilePINToken(c *gin.Context, svc *service.Container, userID, profileID string) bool {
	token := strings.TrimSpace(c.GetHeader("X-Play-Profile-PIN-Token"))
	if token == "" {
		token = strings.TrimSpace(c.Query("profile_pin_token"))
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	expectedSignature := playProfilePINSignature(svc.Cfg.Secrets.JWTSecret, parts[0])
	if expectedSignature == "" || !hmac.Equal([]byte(expectedSignature), []byte(parts[1])) {
		return false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	fields := strings.Split(string(payloadBytes), "|")
	if len(fields) != 3 || fields[0] != userID || fields[1] != profileID {
		return false
	}
	expiresUnix, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return false
	}
	return time.Now().Unix() <= expiresUnix
}

func playProfilePINSignature(secret, encodedPayload string) string {
	if strings.TrimSpace(secret) == "" || encodedPayload == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(encodedPayload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
