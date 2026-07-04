package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func refreshLicenseServerStatus(ctx context.Context, client *licenseClient, state service.LicenseActivationState, deviceID string) (service.LicenseActivationState, bool, bool, error) {
	var upstream licenseServerStatusResp
	if err := client.get(ctx, "/api/v1/status/"+url.PathEscape(deviceID), &upstream); err != nil {
		return state, false, false, err
	}
	if !upstream.Valid {
		state.Valid = false
		state.UpdatedAt = time.Now().Format(time.RFC3339)
		return state, false, upstream.HeartbeatRequested, nil
	}
	applyLicenseStatus(&state, upstream, deviceID)
	return state, true, upstream.HeartbeatRequested, nil
}

func ensureLicenseDeviceID(ctx context.Context, svc *service.Container, candidate string) (string, error) {
	if strings.TrimSpace(candidate) != "" {
		return strings.TrimSpace(candidate), svc.Repo.Setting.Set(ctx, licenseDeviceIDSetting, strings.TrimSpace(candidate))
	}
	existing, err := svc.Repo.Setting.Get(ctx, licenseDeviceIDSetting)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(existing) != "" {
		return strings.TrimSpace(existing), nil
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	id := "msgo-" + hex.EncodeToString(buf[:])
	return id, svc.Repo.Setting.Set(ctx, licenseDeviceIDSetting, id)
}

func defaultLicenseDeviceName() string {
	host, _ := os.Hostname()
	if strings.TrimSpace(host) == "" {
		return "MediaStationGo Server"
	}
	return "MediaStationGo - " + host
}

func licenseStateFromSigned(resp licenseServerSignedResp, deviceID, deviceName string) service.LicenseActivationState {
	expiry := ""
	if resp.ExpiryDate != nil {
		expiry = *resp.ExpiryDate
	}
	return service.LicenseActivationState{
		Valid:          resp.Valid,
		LicenseType:    resp.LicenseType,
		ExpiryDate:     expiry,
		MaxDevices:     resp.MaxDevices,
		MaxUsers:       resp.MaxUsers,
		UnlimitedUsers: !resp.LegacySignature && resp.MaxUsers == nil,
		DaysRemaining:  resp.DaysRemaining,
		NextHeartbeat:  resp.NextHeartbeat,
		DeviceID:       deviceID,
		DeviceName:     deviceName,
		UpdatedAt:      time.Now().Format(time.RFC3339),
	}
}

func licenseHeartbeatPayload(state service.LicenseActivationState, deviceID, deviceName string) map[string]any {
	payload := map[string]any{
		"fingerprint": deviceID,
		"instance_id": deviceID,
		"device_name": deviceName,
	}
	if key := strings.TrimSpace(state.LicenseKey); key != "" {
		payload["key"] = key
	}
	return payload
}

func licenseStatusMaxUsers(state service.LicenseActivationState) any {
	active := state.Valid && !licenseStateExpired(state.ExpiryDate)
	if active {
		if state.UnlimitedUsers {
			return nil
		}
		if state.MaxUsers != nil && *state.MaxUsers > 0 {
			return *state.MaxUsers
		}
		return service.LicensedUserLimit
	}
	return service.OpenSourceUserLimit
}

func applyLicenseStatus(state *service.LicenseActivationState, upstream licenseServerStatusResp, deviceID string) {
	state.Valid = upstream.Valid
	if upstream.LicenseType != nil {
		state.LicenseType = *upstream.LicenseType
	}
	if upstream.ExpiryDate != nil {
		state.ExpiryDate = *upstream.ExpiryDate
	} else {
		state.ExpiryDate = ""
	}
	if upstream.MaxDevices > 0 {
		state.MaxDevices = upstream.MaxDevices
	}
	state.MaxUsers = upstream.MaxUsers
	state.UnlimitedUsers = upstream.UnlimitedUsers
	state.DaysRemaining = upstream.DaysRemaining
	if upstream.DeviceName != "" {
		state.DeviceName = upstream.DeviceName
	}
	state.DeviceID = deviceID
	state.UpdatedAt = time.Now().Format(time.RFC3339)
}

func persistLicenseState(ctx context.Context, svc *service.Container, state service.LicenseActivationState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if strings.TrimSpace(state.DeviceID) != "" {
		_ = svc.Repo.Setting.Set(ctx, licenseDeviceIDSetting, strings.TrimSpace(state.DeviceID))
	}
	return svc.Repo.Setting.Set(ctx, service.LicenseSettingActivation, string(data))
}

func loadLicenseState(ctx context.Context, svc *service.Container) (service.LicenseActivationState, error) {
	raw, err := svc.Repo.Setting.Get(ctx, service.LicenseSettingActivation)
	if err != nil || raw == "" {
		return service.LicenseActivationState{}, err
	}
	var state service.LicenseActivationState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return service.LicenseActivationState{}, err
	}
	return state, nil
}

func licenseActivationView(state service.LicenseActivationState) gin.H {
	updatedAt := state.UpdatedAt
	if strings.TrimSpace(updatedAt) == "" {
		updatedAt = time.Now().Format(time.RFC3339)
	}
	return gin.H{
		"id":              state.DeviceID,
		"key_id":          state.LicenseType,
		"key":             maskLicenseKey(state.LicenseKey),
		"device_id":       state.DeviceID,
		"device_name":     state.DeviceName,
		"plan":            state.LicenseType,
		"max_activations": state.MaxDevices,
		"max_users":       state.MaxUsers,
		"unlimited_users": state.UnlimitedUsers,
		"expires_at":      emptyAsNil(state.ExpiryDate),
		"valid":           state.Valid && !licenseStateExpired(state.ExpiryDate),
		"heartbeat_at":    updatedAt,
		"created_at":      updatedAt,
	}
}

func maskLicenseKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return key
	}
	return key[:5] + "..." + key[len(key)-4:]
}

func licenseStatusMessage(active bool, clientErr error) string {
	if active {
		return "已激活"
	}
	if clientErr != nil && !strings.Contains(clientErr.Error(), "not configured") {
		return clientErr.Error()
	}
	return "开源版：最多 20 个用户"
}

func licenseStateExpired(expiry string) bool {
	if expiry == "" {
		return false
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, expiry); err == nil {
			return time.Now().After(t)
		}
	}
	return false
}

func emptyAsNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
