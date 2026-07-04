package handler

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLicenseStatusMaxUsersUsesLicensedLimit(t *testing.T) {
	maxUsers := 25
	state := service.LicenseActivationState{Valid: true, MaxUsers: &maxUsers}

	if got := licenseStatusMaxUsers(state); got != maxUsers {
		t.Fatalf("expected licensed max users %d, got %#v", maxUsers, got)
	}
}

func TestLicenseStatusMaxUsersAllowsUnlimited(t *testing.T) {
	state := service.LicenseActivationState{Valid: true, UnlimitedUsers: true}

	if got := licenseStatusMaxUsers(state); got != nil {
		t.Fatalf("expected unlimited max users to be nil, got %#v", got)
	}
}

func TestLicenseStatusMaxUsersFallsBackToOpenSourceLimit(t *testing.T) {
	state := service.LicenseActivationState{}

	if got := licenseStatusMaxUsers(state); got != service.OpenSourceUserLimit {
		t.Fatalf("expected open-source max users %d, got %#v", service.OpenSourceUserLimit, got)
	}
}

func TestApplyLicenseStatusReflectsEditedLimitAndClearsExpiry(t *testing.T) {
	maxUsers := 60
	licenseType := "subscription"
	state := service.LicenseActivationState{
		Valid:          true,
		LicenseType:    "enterprise",
		ExpiryDate:     "2026-01-01",
		MaxDevices:     2,
		UnlimitedUsers: true,
	}

	applyLicenseStatus(&state, licenseServerStatusResp{
		Valid:          true,
		LicenseType:    &licenseType,
		ExpiryDate:     nil,
		MaxDevices:     5,
		MaxUsers:       &maxUsers,
		UnlimitedUsers: false,
		DeviceName:     "Edited Device",
	}, "device-1")

	if !state.Valid || state.LicenseType != "subscription" || state.ExpiryDate != "" || state.MaxDevices != 5 {
		t.Fatalf("status fields were not fully refreshed: %+v", state)
	}
	if state.MaxUsers == nil || *state.MaxUsers != 60 || state.UnlimitedUsers {
		t.Fatalf("user limit was not refreshed from status: %+v", state)
	}
	if state.DeviceID != "device-1" || state.DeviceName != "Edited Device" {
		t.Fatalf("device fields were not refreshed: %+v", state)
	}
}

func TestApplyLicenseStatusReflectsUnlimitedUsers(t *testing.T) {
	maxUsers := 30
	state := service.LicenseActivationState{Valid: true, MaxUsers: &maxUsers}

	applyLicenseStatus(&state, licenseServerStatusResp{
		Valid:          true,
		MaxUsers:       nil,
		UnlimitedUsers: true,
	}, "device-1")

	if state.MaxUsers != nil || !state.UnlimitedUsers {
		t.Fatalf("unlimited status should clear previous finite user limit: %+v", state)
	}
}

func TestRefreshLicenseServerStatusReflectsEditedLimitAndHeartbeatRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status/device-1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"valid": true,
			"license_type": "subscription",
			"max_devices": 5,
			"max_users": 60,
			"unlimited_users": false,
			"device_name": "NAS",
			"heartbeat_requested": true,
			"is_active": true
		}`))
	}))
	defer upstream.Close()

	state := service.LicenseActivationState{Valid: true, UnlimitedUsers: true}
	client := &licenseClient{baseURL: upstream.URL, httpClient: upstream.Client()}

	refreshed, ok, requested, err := refreshLicenseServerStatus(t.Context(), client, state, "device-1")
	if err != nil {
		t.Fatalf("refresh status: %v", err)
	}
	if !ok || !requested {
		t.Fatalf("expected valid status with requested heartbeat, ok=%v requested=%v", ok, requested)
	}
	if refreshed.MaxUsers == nil || *refreshed.MaxUsers != 60 || refreshed.UnlimitedUsers {
		t.Fatalf("edited user limit was not reflected: %+v", refreshed)
	}
	if refreshed.MaxDevices != 5 || refreshed.DeviceName != "NAS" {
		t.Fatalf("server status fields were not applied: %+v", refreshed)
	}
}

func TestLicenseHeartbeatPayloadIncludesStoredLicenseKey(t *testing.T) {
	payload := licenseHeartbeatPayload(service.LicenseActivationState{
		LicenseKey: "MS-ABCD-EFGH-JKLM-NPQR",
	}, "device-1", "NAS")

	if payload["fingerprint"] != "device-1" || payload["instance_id"] != "device-1" || payload["device_name"] != "NAS" {
		t.Fatalf("heartbeat identity payload is wrong: %#v", payload)
	}
	if payload["key"] != "MS-ABCD-EFGH-JKLM-NPQR" {
		t.Fatalf("heartbeat should include stored license key for server-side backfill: %#v", payload)
	}
}

func TestLicenseClientVerifiesEd25519Signature(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	resp := licenseServerSignedResp{
		Valid:         true,
		LicenseType:   "subscription",
		MaxDevices:    2,
		NextHeartbeat: time.Now().Add(time.Hour).Format(time.RFC3339),
		SignatureAlg:  "ed25519",
	}
	resp.Signature = signLicenseTestPayloadEd25519(privateKey, resp)

	client := &licenseClient{ed25519PublicKey: publicKey}
	if err := client.verifySigned(&resp); err != nil {
		t.Fatalf("verify ed25519 signature: %v", err)
	}
}

func TestLicenseClientRejectsEd25519WithoutPublicKey(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	resp := licenseServerSignedResp{
		Valid:         true,
		LicenseType:   "subscription",
		MaxDevices:    2,
		NextHeartbeat: time.Now().Add(time.Hour).Format(time.RFC3339),
		SignatureAlg:  "ed25519",
	}
	resp.Signature = signLicenseTestPayloadEd25519(privateKey, resp)

	client := &licenseClient{}
	if err := client.verifySigned(&resp); err == nil || !strings.Contains(err.Error(), "public key") {
		t.Fatalf("expected missing public key error, got %v", err)
	}
}

func TestLicenseHeartbeatDueUsesTwelveHourWindow(t *testing.T) {
	state := service.LicenseActivationState{
		Valid:     true,
		UpdatedAt: time.Now().Add(-11 * time.Hour).Format(time.RFC3339),
	}
	if licenseHeartbeatDue(state, 12*time.Hour) {
		t.Fatalf("heartbeat should not be due before interval")
	}

	state.UpdatedAt = time.Now().Add(-13 * time.Hour).Format(time.RFC3339)
	if !licenseHeartbeatDue(state, 12*time.Hour) {
		t.Fatalf("heartbeat should be due after interval")
	}
}

func TestStartupLicenseHeartbeatIgnoresTwelveHourWindow(t *testing.T) {
	heartbeatCount := 0
	maxUsers := 40
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/heartbeat":
			heartbeatCount++
			resp := licenseServerSignedResp{
				Valid:         true,
				LicenseType:   "subscription",
				MaxDevices:    2,
				MaxUsers:      &maxUsers,
				NextHeartbeat: time.Now().Add(time.Hour).Format(time.RFC3339),
			}
			resp.Signature = signLicenseTestPayload("test-secret", resp)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		case "/api/v1/status/device-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"valid": true,
				"license_type": "subscription",
				"max_devices": 2,
				"max_users": 40,
				"unlimited_users": false,
				"device_name": "NAS",
				"is_active": true
			}`))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	svc := newLicenseHandlerTestService(t)
	if err := svc.Repo.Setting.Set(t.Context(), licenseServerURLSetting, upstream.URL); err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.Setting.Set(t.Context(), licenseHMACSecretSetting, "test-secret"); err != nil {
		t.Fatal(err)
	}
	state := service.LicenseActivationState{
		Valid:      true,
		LicenseKey: "MS-ABCD-EFGH-JKLM-NPQR",
		DeviceID:   "device-1",
		DeviceName: "NAS",
		UpdatedAt:  time.Now().Format(time.RFC3339),
	}
	if err := persistLicenseState(t.Context(), svc, state); err != nil {
		t.Fatal(err)
	}

	refreshed, sent, err := maybeSendStartupLicenseHeartbeat(t.Context(), svc)
	if err != nil {
		t.Fatalf("startup heartbeat: %v", err)
	}
	if !sent || heartbeatCount != 1 {
		t.Fatalf("startup heartbeat should be sent once, sent=%v count=%d", sent, heartbeatCount)
	}
	if refreshed.MaxUsers == nil || *refreshed.MaxUsers != 40 {
		t.Fatalf("startup heartbeat should refresh licensed user capacity, got %+v", refreshed)
	}
}

func TestStartupLicenseHeartbeatSkipsStateWithoutStoredKey(t *testing.T) {
	heartbeatCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		heartbeatCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	svc := newLicenseHandlerTestService(t)
	if err := svc.Repo.Setting.Set(t.Context(), licenseServerURLSetting, upstream.URL); err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.Setting.Set(t.Context(), licenseHMACSecretSetting, "test-secret"); err != nil {
		t.Fatal(err)
	}
	if err := persistLicenseState(t.Context(), svc, service.LicenseActivationState{
		Valid:     true,
		DeviceID:  "device-1",
		UpdatedAt: time.Now().Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	_, sent, err := maybeSendStartupLicenseHeartbeat(t.Context(), svc)
	if err != nil {
		t.Fatalf("startup heartbeat should skip without error, got %v", err)
	}
	if sent || heartbeatCount != 0 {
		t.Fatalf("startup heartbeat without stored license key should be skipped, sent=%v count=%d", sent, heartbeatCount)
	}
}

func TestLicenseHeartbeatEligibleRequiresActivationState(t *testing.T) {
	if licenseHeartbeatEligible(service.LicenseActivationState{DeviceID: "device-only"}) {
		t.Fatalf("device id alone should not trigger automatic license heartbeat")
	}
	if !licenseHeartbeatEligible(service.LicenseActivationState{LicenseKey: "MS-KEY"}) {
		t.Fatalf("stored license key should trigger automatic license heartbeat")
	}
	if licenseHeartbeatEligible(service.LicenseActivationState{Valid: true}) {
		t.Fatalf("valid state without stored license key should not trigger automatic license heartbeat")
	}
}

func TestLicenseStatusSkipsUnlicensedHeartbeatWithDefaultServer(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	svc := newLicenseHandlerTestService(t)
	svc.Cfg.License.ServerURL = upstream.URL
	svc.Cfg.License.HMACSecret = "test-secret"

	router := gin.New()
	router.GET("/license/status", licenseStatusHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/license/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("unlicensed status should not contact license server, got %d calls", upstreamCalls)
	}
}

func TestLicenseActivateBindsServerInstanceNotBrowserFingerprint(t *testing.T) {
	var upstreamFingerprint string
	maxUsers := 60
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream payload: %v", err)
		}
		upstreamFingerprint, _ = payload["fingerprint"].(string)
		resp := licenseServerSignedResp{
			Valid:         true,
			LicenseType:   "subscription",
			MaxDevices:    3,
			MaxUsers:      &maxUsers,
			NextHeartbeat: time.Now().Add(time.Hour).Format(time.RFC3339),
		}
		resp.Signature = signLicenseTestPayload("test-secret", resp)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstream.Close()

	svc := newLicenseHandlerTestService(t)
	if err := svc.Repo.Setting.Set(t.Context(), licenseServerURLSetting, upstream.URL); err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.Setting.Set(t.Context(), licenseHMACSecretSetting, "test-secret"); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/license/activate", licenseActivateHandler(svc))
	req := httptest.NewRequest(http.MethodPost, "/license/activate", strings.NewReader(`{
		"key": "MS-ABCD-EFGH-JKLM-NPQR",
		"device_id": "browser-fingerprint",
		"device_name": ""
	}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("activate status = %d body=%s", w.Code, w.Body.String())
	}
	if upstreamFingerprint == "" || upstreamFingerprint == "browser-fingerprint" || !strings.HasPrefix(upstreamFingerprint, "msgo-") {
		t.Fatalf("activation should use server-generated msgo id, got %q", upstreamFingerprint)
	}
	stored, err := svc.Repo.Setting.Get(t.Context(), licenseDeviceIDSetting)
	if err != nil {
		t.Fatal(err)
	}
	if stored != upstreamFingerprint {
		t.Fatalf("stored device id = %q, upstream fingerprint = %q", stored, upstreamFingerprint)
	}
}

func TestNewLicenseClientRequiresSignatureVerifier(t *testing.T) {
	svc := newLicenseHandlerTestService(t)
	if err := svc.Repo.Setting.Set(t.Context(), licenseServerURLSetting, "http://127.0.0.1:8001"); err != nil {
		t.Fatal(err)
	}
	if _, err := newLicenseClient(t.Context(), svc); err == nil || !strings.Contains(err.Error(), "public key or hmac secret") {
		t.Fatalf("expected missing signature verifier error, got %v", err)
	}
}

func newLicenseHandlerTestService(t *testing.T) *service.Container {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	return &service.Container{
		Cfg:  &config.Config{},
		Repo: repository.New(db),
	}
}

func signLicenseTestPayload(secret string, resp licenseServerSignedResp) string {
	unsigned := struct {
		Valid         bool    `json:"valid"`
		LicenseType   string  `json:"license_type"`
		ExpiryDate    *string `json:"expiry_date"`
		MaxDevices    int     `json:"max_devices"`
		MaxUsers      *int    `json:"max_users"`
		DaysRemaining *int    `json:"days_remaining"`
		NextHeartbeat string  `json:"next_heartbeat"`
	}{
		Valid:         resp.Valid,
		LicenseType:   resp.LicenseType,
		ExpiryDate:    resp.ExpiryDate,
		MaxDevices:    resp.MaxDevices,
		MaxUsers:      resp.MaxUsers,
		DaysRemaining: resp.DaysRemaining,
		NextHeartbeat: resp.NextHeartbeat,
	}
	payload, _ := json.Marshal(unsigned)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func signLicenseTestPayloadEd25519(privateKey ed25519.PrivateKey, resp licenseServerSignedResp) string {
	payload, _ := json.Marshal(licenseSignedPayload(resp))
	return base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
}
