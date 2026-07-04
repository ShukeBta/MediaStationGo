package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const (
	licenseServerURLSetting  = "license.server_url"
	licenseHMACSecretSetting = "license.hmac_secret" // #nosec G101 -- setting key name, not the HMAC secret value.
	licenseDeviceIDSetting   = "license.device_id"
	licenseDeviceNameSetting = "license.device_name"
)

type licenseActivateReq struct {
	Key string `json:"key" binding:"required"`
	// DeviceID is accepted for wire compatibility with older web clients but is
	// intentionally ignored. Licensing binds to this MediaStationGo server
	// instance, not to the browser that opened the admin page.
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
}

type licenseServerSignedResp struct {
	Valid           bool    `json:"valid"`
	LicenseType     string  `json:"license_type"`
	ExpiryDate      *string `json:"expiry_date"`
	MaxDevices      int     `json:"max_devices"`
	MaxUsers        *int    `json:"max_users"`
	DaysRemaining   *int    `json:"days_remaining"`
	NextHeartbeat   string  `json:"next_heartbeat"`
	Signature       string  `json:"signature"`
	SignatureAlg    string  `json:"signature_alg"`
	LegacySignature bool    `json:"-"`
}

type licenseServerStatusResp struct {
	Valid              bool    `json:"valid"`
	LicenseType        *string `json:"license_type"`
	ExpiryDate         *string `json:"expiry_date"`
	MaxDevices         int     `json:"max_devices"`
	MaxUsers           *int    `json:"max_users"`
	UnlimitedUsers     bool    `json:"unlimited_users"`
	DaysRemaining      *int    `json:"days_remaining"`
	DeviceName         string  `json:"device_name"`
	IsActive           bool    `json:"is_active"`
	HeartbeatRequested bool    `json:"heartbeat_requested"`
}

func licenseActivateHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req licenseActivateReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		client, err := newLicenseClient(c.Request.Context(), svc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		deviceID, err := ensureLicenseDeviceID(c.Request.Context(), svc, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		deviceName := strings.TrimSpace(req.DeviceName)
		if deviceName == "" {
			deviceName = defaultLicenseDeviceName()
		}
		_ = svc.Repo.Setting.Set(c.Request.Context(), licenseDeviceNameSetting, deviceName)

		payload := map[string]any{
			"key":         strings.TrimSpace(req.Key),
			"fingerprint": deviceID,
			"device_name": deviceName,
			"instance_id": deviceID,
		}
		var upstream licenseServerSignedResp
		if err := client.post(c.Request.Context(), "/api/v1/activate", payload, &upstream); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if err := client.verifySigned(&upstream); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		state := licenseStateFromSigned(upstream, deviceID, deviceName)
		state.LicenseKey = strings.TrimSpace(req.Key)
		if err := persistLicenseState(c.Request.Context(), svc, state); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, licenseActivationView(state))
	}
}

func licenseStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		state, _ := loadLicenseState(c.Request.Context(), svc)
		client, err := newLicenseClient(c.Request.Context(), svc)
		hasLicenseKey := strings.TrimSpace(state.LicenseKey) != ""
		if err == nil && hasLicenseKey {
			deviceID, idErr := ensureLicenseDeviceID(c.Request.Context(), svc, state.DeviceID)
			if idErr == nil {
				deviceName, _ := svc.Repo.Setting.Get(c.Request.Context(), licenseDeviceNameSetting)
				if strings.TrimSpace(deviceName) == "" {
					deviceName = defaultLicenseDeviceName()
					_ = svc.Repo.Setting.Set(c.Request.Context(), licenseDeviceNameSetting, deviceName)
				}
				var signed licenseServerSignedResp
				if heartbeatErr := client.post(c.Request.Context(), "/api/v1/heartbeat", licenseHeartbeatPayload(state, deviceID, deviceName), &signed); heartbeatErr == nil && client.verifySigned(&signed) == nil {
					nextState := licenseStateFromSigned(signed, deviceID, deviceName)
					nextState.LicenseKey = state.LicenseKey
					state = nextState
					if refreshed, ok, _, refreshErr := refreshLicenseServerStatus(c.Request.Context(), client, state, deviceID); refreshErr == nil && ok {
						refreshed.LicenseKey = state.LicenseKey
						state = refreshed
					}
					_ = persistLicenseState(c.Request.Context(), svc, state)
				} else {
					if refreshed, ok, _, getErr := refreshLicenseServerStatus(c.Request.Context(), client, state, deviceID); getErr == nil && ok {
						state = refreshed
						_ = persistLicenseState(c.Request.Context(), svc, state)
					} else if getErr == nil {
						state.Valid = false
						_ = persistLicenseState(c.Request.Context(), svc, state)
					}
				}
			}
		} else if err == nil && state.Valid {
			deviceID, idErr := ensureLicenseDeviceID(c.Request.Context(), svc, state.DeviceID)
			if idErr == nil {
				if refreshed, ok, _, getErr := refreshLicenseServerStatus(c.Request.Context(), client, state, deviceID); getErr == nil && ok {
					state = refreshed
					_ = persistLicenseState(c.Request.Context(), svc, state)
				} else if getErr == nil {
					state.Valid = false
					_ = persistLicenseState(c.Request.Context(), svc, state)
				}
			}
		}
		active := state.Valid && !licenseStateExpired(state.ExpiryDate)
		c.JSON(http.StatusOK, gin.H{
			"active":          active,
			"message":         licenseStatusMessage(active, err),
			"max_users":       licenseStatusMaxUsers(state),
			"unlimited_users": state.Valid && !licenseStateExpired(state.ExpiryDate) && state.UnlimitedUsers,
			"activation":      licenseActivationView(state),
		})
	}
}

func licenseHeartbeatHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		state, err := sendLicenseHeartbeat(c.Request.Context(), svc)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, licenseActivationView(state))
	}
}
