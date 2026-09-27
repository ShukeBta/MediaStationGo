package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestFD2PPVConfigKeepsPasswordEncryptedAndFullyMasked(t *testing.T) {
	db := newServiceTestDB(t, &model.APIConfig{})
	repos := repository.New(db)
	svc := NewAPIConfigService(zap.NewNop(), repos, NewCryptoService("test-secret", zap.NewNop()))
	username := "fd2-user"
	password := "fd2-password"
	if _, err := svc.Update(context.Background(), "fd2ppv", APIConfigPatch{APIKey: &password, Extra: &username}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.Get(context.Background(), "fd2ppv")
	if err != nil {
		t.Fatal(err)
	}
	if view == nil || !view.HasKey || view.MaskedKey != "••••••••" || view.Extra != username {
		t.Fatalf("public view = %#v", view)
	}
	var row model.APIConfig
	if err := db.Where("provider = ?", "fd2ppv").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.APIKey == password || !strings.HasPrefix(row.APIKey, encPrefix) {
		t.Fatalf("fd2ppv password was not encrypted")
	}
}

func TestDiscoverOpenAICompatibleModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-live" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"deepseek-reasoner","owned_by":"deepseek"},{"id":"deepseek-chat","owned_by":"deepseek"},{"id":"deepseek-chat"}]}`))
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.APIConfig{})
	repos := repository.New(db)
	svc := NewAPIConfigService(zap.NewNop(), repos, NewCryptoService("test-secret", zap.NewNop()))
	items, err := svc.DiscoverModels(t.Context(), "openai", AIModelDiscoveryInput{
		APIKey: "sk-live", BaseURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "deepseek-chat" || items[1].ID != "deepseek-reasoner" {
		t.Fatalf("models = %#v", items)
	}
}

func TestDeepSeekConfigStoresSelectedModel(t *testing.T) {
	db := newServiceTestDB(t, &model.APIConfig{})
	svc := NewAPIConfigService(zap.NewNop(), repository.New(db), NewCryptoService("test-secret", zap.NewNop()))
	modelID := "deepseek-chat"
	view, err := svc.Update(t.Context(), "deepseek", APIConfigPatch{Model: &modelID})
	if err != nil {
		t.Fatal(err)
	}
	if view.Model != modelID {
		t.Fatalf("model = %q, want %q", view.Model, modelID)
	}
	resolved, err := svc.Resolve(t.Context(), "deepseek")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Model != modelID {
		t.Fatalf("resolved model = %q, want %q", resolved.Model, modelID)
	}
}

func TestAIModelsEndpointAcceptsVersionedAndCompletionBaseURLs(t *testing.T) {
	for input, want := range map[string]string{
		"https://api.openai.com/v1":                "https://api.openai.com/v1/models",
		"https://api.deepseek.com/":                "https://api.deepseek.com/models",
		"https://example.test/v1/chat/completions": "https://example.test/v1/models",
	} {
		if got := aiModelsEndpoint(input); got != want {
			t.Fatalf("aiModelsEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
}
