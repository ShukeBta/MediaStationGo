package handler

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type licenseClient struct {
	baseURL          string
	hmacSecret       string
	ed25519PublicKey ed25519.PublicKey
	httpClient       *http.Client
}

func newLicenseClient(ctx context.Context, svc *service.Container) (*licenseClient, error) {
	baseURL, _ := svc.Repo.Setting.Get(ctx, licenseServerURLSetting)
	if strings.TrimSpace(baseURL) == "" {
		baseURL = svc.Cfg.License.ServerURL
	}
	secret, _ := svc.Repo.Setting.Get(ctx, licenseHMACSecretSetting)
	if strings.TrimSpace(secret) == "" {
		secret = svc.Cfg.License.HMACSecret
	}
	publicKeyRaw, _ := svc.Repo.Setting.Get(ctx, "license.public_key")
	if strings.TrimSpace(publicKeyRaw) == "" {
		publicKeyRaw = svc.Cfg.License.PublicKey
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, errors.New("license server url not configured")
	}
	secret = strings.TrimSpace(secret)
	publicKey, err := parseLicenseEd25519PublicKey(publicKeyRaw)
	if err != nil {
		return nil, err
	}
	if secret == "" && len(publicKey) == 0 {
		return nil, errors.New("license public key or hmac secret not configured")
	}
	return &licenseClient{
		baseURL:          baseURL,
		hmacSecret:       secret,
		ed25519PublicKey: publicKey,
		httpClient:       &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (c *licenseClient) post(ctx context.Context, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *licenseClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *licenseClient) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var er struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &er)
		if er.Message != "" {
			return fmt.Errorf("license server: %s", er.Message)
		}
		if er.Error != "" {
			return fmt.Errorf("license server: %s", er.Error)
		}
		return fmt.Errorf("license server http %d", resp.StatusCode)
	}
	return json.Unmarshal(data, out)
}

func (c *licenseClient) verifySigned(resp *licenseServerSignedResp) error {
	if strings.TrimSpace(resp.Signature) == "" {
		return errors.New("license server signature missing")
	}
	switch strings.ToLower(strings.TrimSpace(resp.SignatureAlg)) {
	case "ed25519":
		return c.verifyEd25519Signed(*resp)
	case "", "hmac", "hmac-sha256":
		return c.verifyHMACSigned(resp)
	default:
		return fmt.Errorf("unsupported license signature algorithm %q", resp.SignatureAlg)
	}
}

func (c *licenseClient) verifyEd25519Signed(resp licenseServerSignedResp) error {
	if len(c.ed25519PublicKey) != ed25519.PublicKeySize {
		return errors.New("license Ed25519 public key not configured")
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(resp.Signature))
	if err != nil {
		return err
	}
	payload, err := json.Marshal(licenseSignedPayload(resp))
	if err != nil {
		return err
	}
	if !ed25519.Verify(c.ed25519PublicKey, payload, signature) {
		return errors.New("license server signature verification failed")
	}
	return nil
}

func (c *licenseClient) verifyHMACSigned(resp *licenseServerSignedResp) error {
	if c.hmacSecret == "" {
		return errors.New("license hmac secret not configured")
	}
	payload, err := json.Marshal(licenseSignedPayload(*resp))
	if err != nil {
		return err
	}
	mac := hmac.New(sha256.New, []byte(c.hmacSecret))
	_, _ = mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(resp.Signature)) {
		if c.verifyLegacySigned(*resp) {
			resp.LegacySignature = true
			return nil
		}
		return errors.New("license server signature verification failed")
	}
	return nil
}

func (c *licenseClient) verifyLegacySigned(resp licenseServerSignedResp) bool {
	unsigned := struct {
		Valid         bool    `json:"valid"`
		LicenseType   string  `json:"license_type"`
		ExpiryDate    *string `json:"expiry_date"`
		MaxDevices    int     `json:"max_devices"`
		DaysRemaining *int    `json:"days_remaining"`
		NextHeartbeat string  `json:"next_heartbeat"`
	}{
		Valid:         resp.Valid,
		LicenseType:   resp.LicenseType,
		ExpiryDate:    resp.ExpiryDate,
		MaxDevices:    resp.MaxDevices,
		DaysRemaining: resp.DaysRemaining,
		NextHeartbeat: resp.NextHeartbeat,
	}
	payload, err := json.Marshal(unsigned)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(c.hmacSecret))
	_, _ = mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(resp.Signature))
}

func parseLicenseEd25519PublicKey(encoded string) (ed25519.PublicKey, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode license Ed25519 public key: %w", err)
	}
	if key, err := x509.ParsePKIXPublicKey(raw); err == nil {
		if publicKey, ok := key.(ed25519.PublicKey); ok && len(publicKey) == ed25519.PublicKeySize {
			return publicKey, nil
		}
		return nil, errors.New("license public key is not an Ed25519 PKIX public key")
	}
	if len(raw) == ed25519.PublicKeySize {
		return ed25519.PublicKey(raw), nil
	}
	return nil, errors.New("license public key must be base64 PKIX or raw 32-byte Ed25519 public key")
}

func licenseSignedPayload(resp licenseServerSignedResp) any {
	return struct {
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
}
