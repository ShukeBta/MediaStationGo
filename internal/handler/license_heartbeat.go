package handler

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const (
	licenseHeartbeatInterval      = 12 * time.Hour
	licenseHeartbeatCheckInterval = 30 * time.Minute
	licenseHeartbeatStartupDelay  = 2 * time.Minute
)

func refreshLicenseCapacityBestEffort(ctx context.Context, svc *service.Container) {
	if svc == nil || svc.Repo == nil || svc.Repo.Setting == nil {
		return
	}
	_, _, _ = maybeSendLicenseHeartbeat(ctx, svc, 0)
}

// RunLicenseHeartbeatLoop keeps the license server aware of active deployments.
// The loop checks periodically, but only sends when the last stored heartbeat is
// older than licenseHeartbeatInterval.
func RunLicenseHeartbeatLoop(ctx context.Context, svc *service.Container) {
	if svc == nil {
		return
	}
	run := func(interval time.Duration) {
		state, sent, err := maybeSendLicenseHeartbeat(ctx, svc, interval)
		logLicenseHeartbeatResult(svc, state, sent, err)
	}
	runStartup := func() {
		state, sent, err := maybeSendStartupLicenseHeartbeat(ctx, svc)
		logLicenseHeartbeatResult(svc, state, sent, err)
	}

	timer := time.NewTimer(licenseHeartbeatStartupDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		runStartup()
	}

	ticker := time.NewTicker(licenseHeartbeatCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run(licenseHeartbeatInterval)
		}
	}
}

func logLicenseHeartbeatResult(svc *service.Container, state service.LicenseActivationState, sent bool, err error) {
	if svc == nil || svc.Log == nil {
		return
	}
	if err != nil {
		svc.Log.Warn("license heartbeat failed", zap.Error(err))
		return
	}
	if sent {
		svc.Log.Info("license heartbeat sent", zap.String("device_id", state.DeviceID))
	}
}

func maybeSendStartupLicenseHeartbeat(ctx context.Context, svc *service.Container) (service.LicenseActivationState, bool, error) {
	state, err := loadLicenseState(ctx, svc)
	if err != nil {
		return state, false, nil
	}
	if strings.TrimSpace(state.LicenseKey) == "" {
		return state, false, nil
	}
	return maybeSendLicenseHeartbeat(ctx, svc, 0)
}

func maybeSendLicenseHeartbeat(ctx context.Context, svc *service.Container, interval time.Duration) (service.LicenseActivationState, bool, error) {
	state, err := loadLicenseState(ctx, svc)
	if err != nil {
		return state, false, nil
	}
	if !licenseHeartbeatEligible(state) {
		return state, false, nil
	}
	if !licenseHeartbeatDue(state, interval) {
		client, clientErr := newLicenseClient(ctx, svc)
		if clientErr != nil {
			return state, false, nil
		}
		deviceID, idErr := ensureLicenseDeviceID(ctx, svc, state.DeviceID)
		if idErr != nil {
			return state, false, idErr
		}
		refreshed, ok, requested, refreshErr := refreshLicenseServerStatus(ctx, client, state, deviceID)
		if refreshErr == nil && ok {
			state = refreshed
			_ = persistLicenseState(ctx, svc, state)
		}
		if !requested {
			return state, false, nil
		}
	}
	next, err := sendLicenseHeartbeat(ctx, svc)
	if err != nil {
		return state, false, err
	}
	return next, true, nil
}

func licenseHeartbeatEligible(state service.LicenseActivationState) bool {
	return strings.TrimSpace(state.LicenseKey) != ""
}

func licenseHeartbeatDue(state service.LicenseActivationState, interval time.Duration) bool {
	if interval <= 0 {
		return true
	}
	updatedAt := strings.TrimSpace(state.UpdatedAt)
	if updatedAt == "" {
		return true
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, updatedAt); err == nil {
			return time.Since(t) >= interval
		}
	}
	return true
}

func sendLicenseHeartbeat(ctx context.Context, svc *service.Container) (service.LicenseActivationState, error) {
	client, err := newLicenseClient(ctx, svc)
	if err != nil {
		return service.LicenseActivationState{}, err
	}
	oldState, _ := loadLicenseState(ctx, svc)
	deviceID, err := ensureLicenseDeviceID(ctx, svc, oldState.DeviceID)
	if err != nil {
		return service.LicenseActivationState{}, err
	}
	deviceName, _ := svc.Repo.Setting.Get(ctx, licenseDeviceNameSetting)
	if strings.TrimSpace(deviceName) == "" {
		deviceName = defaultLicenseDeviceName()
		_ = svc.Repo.Setting.Set(ctx, licenseDeviceNameSetting, deviceName)
	}
	var upstream licenseServerSignedResp
	if err := client.post(ctx, "/api/v1/heartbeat", licenseHeartbeatPayload(oldState, deviceID, deviceName), &upstream); err != nil {
		return service.LicenseActivationState{}, err
	}
	if err := client.verifySigned(&upstream); err != nil {
		return service.LicenseActivationState{}, err
	}
	state := licenseStateFromSigned(upstream, deviceID, deviceName)
	state.LicenseKey = oldState.LicenseKey
	if refreshed, ok, _, refreshErr := refreshLicenseServerStatus(ctx, client, state, deviceID); refreshErr == nil && ok {
		refreshed.LicenseKey = state.LicenseKey
		state = refreshed
	}
	if err := persistLicenseState(ctx, svc, state); err != nil {
		return service.LicenseActivationState{}, err
	}
	return state, nil
}
