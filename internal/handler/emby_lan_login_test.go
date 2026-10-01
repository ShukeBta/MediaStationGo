package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyLANDiscoveryAndLoginOverHTTP(t *testing.T) {
	cfg, svc, user := currentRoleTestServices(t)
	cfg.App.Port = 8080 // Container port differs from the address entered by the client.
	if err := svc.Repo.DB.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("secret-pass"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Repo.User.UpdateFields(t.Context(), user.ID, map[string]any{"password_hash": string(hash)}); err != nil {
		t.Fatal(err)
	}
	svc.Auth = service.NewAuthService(cfg, svc.Log, svc.Repo, service.NewTokenService(cfg, svc.Log, svc.Repo), svc.Permissions)
	svc.Emby = service.NewEmbyService(cfg, svc.Log, svc.Repo)
	svc.Device = service.NewDeviceService(svc.Log, svc.Repo)
	router := gin.New()
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, svc)
	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	request := func(method, path, body, auth string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "nas-lan:18080"
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", auth)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, payload
	}
	for _, tc := range []struct{ client, prefix, discovery, login string }{
		{"Hills", "", "/System/Info/Public", "/Users/AuthenticateByName"},
		{"Infuse", "/emby", "/System/Info/Public", "/Users/AuthenticateByName"},
		{"VidHub", "/emby", "/system/info/public", "/users/authenticatebyname"},
		{"SenPlayer", "", "/System/Info/Public/", "/Users/AuthenticateByName/"},
		{"MobilePlayer", "/emby", "/system/info/public/", "/users/authenticatebyname/"},
	} {
		t.Run(tc.client, func(t *testing.T) {
			header := fmt.Sprintf(`MediaBrowser Client="%s", Device="Phone", DeviceId="mobile-%s", Version="1.0"`, tc.client, tc.client)
			status, body := request(http.MethodGet, tc.prefix+tc.discovery, "", header)
			if status != http.StatusOK {
				t.Fatalf("pre-login discovery status=%d body=%s", status, body)
			}
			var identity map[string]any
			if err := json.Unmarshal(body, &identity); err != nil {
				t.Fatal(err)
			}
			if identity["Id"] == "" || identity["Version"] == "" || identity["LocalAddress"] != "http://nas-lan:18080" || identity["HttpServerPortNumber"] != float64(18080) {
				t.Fatalf("invalid NAS discovery identity: %s", body)
			}
			for _, key := range []string{"Users", "Paths", "ProtocolExtensions", "OperatingSystem", "Configuration"} {
				if _, present := identity[key]; present {
					t.Fatalf("public discovery exposed internal field %q", key)
				}
			}
			if status, body := request(http.MethodHead, tc.prefix+tc.discovery, "", header); status != http.StatusOK || len(body) != 0 {
				t.Fatalf("HEAD discovery status=%d body=%s", status, body)
			}
			if status, body := request(http.MethodPost, tc.prefix+tc.login, `{"Username":"viewer","Pw":"wrong-pass"}`, header); status != http.StatusUnauthorized {
				t.Fatalf("wrong password accepted: status=%d body=%s", status, body)
			}
			status, body = request(http.MethodPost, tc.prefix+tc.login, `{"Username":"viewer","Pw":"secret-pass"}`, header)
			if status != http.StatusOK {
				t.Fatalf("login status=%d body=%s", status, body)
			}
			var login struct{ AccessToken, ServerId string }
			if err := json.Unmarshal(body, &login); err != nil || login.AccessToken == "" || login.ServerId != identity["Id"] {
				t.Fatalf("invalid login result: %s, %v", body, err)
			}
			if status, body := request(http.MethodGet, tc.prefix+"/Users/Me?api_key="+login.AccessToken, "", header); status != http.StatusOK {
				t.Fatalf("client metadata masked valid query token: status=%d body=%s", status, body)
			}
			for _, path := range []string{"/Users/Me", "/Users/Public", "/System/Info"} {
				if status, body := request(http.MethodGet, tc.prefix+path, "", header); status != http.StatusUnauthorized {
					t.Fatalf("private route %s accepted metadata without token: status=%d body=%s", path, status, body)
				}
			}
		})
	}
}
