package service

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestAIStatusUsesDatabaseOpenAIConfig(t *testing.T) {
	db := newServiceTestDB(t, &model.APIConfig{})
	repo := &repository.Container{DB: db}
	crypto := NewCryptoService("test-secret", zap.NewNop())
	apiConfig := NewAPIConfigService(zap.NewNop(), repo, crypto)
	key := "sk-test"
	baseURL := "https://example.test/v1"
	enabled := true
	if _, err := apiConfig.Update(context.Background(), "openai", APIConfigPatch{
		APIKey:  &key,
		BaseURL: &baseURL,
		Enabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}

	ai := NewAIService(&config.Config{
		AI: config.AIConfig{
			Enabled: false,
			Model:   "gpt-4o-mini",
		},
	}, zap.NewNop(), apiConfig)

	status := ai.Status(context.Background())
	if !status.Enabled {
		t.Fatalf("AI status disabled, want enabled from database config")
	}
	if status.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", status.Provider)
	}
}

func TestAIStatusHonorsDisabledDatabaseOpenAIConfig(t *testing.T) {
	db := newServiceTestDB(t, &model.APIConfig{})
	repo := &repository.Container{DB: db}
	apiConfig := NewAPIConfigService(zap.NewNop(), repo, NewCryptoService("test-secret", zap.NewNop()))
	key := "sk-test"
	enabled := false
	if _, err := apiConfig.Update(context.Background(), "openai", APIConfigPatch{
		APIKey:  &key,
		Enabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}

	ai := NewAIService(&config.Config{
		AI: config.AIConfig{
			Enabled: true,
			APIKey:  "sk-file",
			Model:   "gpt-4o-mini",
		},
	}, zap.NewNop(), apiConfig)

	if ai.Status(context.Background()).Enabled {
		t.Fatalf("AI status enabled, want disabled when database config is explicitly disabled")
	}
}
