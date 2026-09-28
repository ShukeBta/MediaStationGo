package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// TokenDeviceKicked uses signed identity. An unbound legacy token cannot prove
// which terminal it belongs to, so any outstanding kick requires a new login.
func (s *DeviceService) TokenDeviceKicked(ctx context.Context, userID, deviceID, deviceName, client string) (bool, error) {
	if deviceID == "" {
		var count int64
		err := s.repo.DB.WithContext(ctx).Model(&model.UserDevice{}).
			Where("user_id = ? AND kicked = ?", userID, true).Count(&count).Error
		return count > 0, err
	}
	d, err := s.repo.UserDevice.Find(ctx, userID, deviceID)
	if err != nil {
		return false, err
	}
	if d != nil {
		return d.Kicked, nil
	}
	if strings.TrimSpace(deviceName) != "" {
		d, err = s.repo.UserDevice.FindByFingerprint(ctx, userID, fingerprint(client, deviceName))
		if err != nil {
			return false, err
		}
		if d != nil {
			return d.Kicked, nil
		}
	}
	return true, nil
}
