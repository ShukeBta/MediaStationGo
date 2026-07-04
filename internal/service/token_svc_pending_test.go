package service

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func newTokenTestRepo(t *testing.T) *repository.Container {
	t.Helper()
	db := newServiceTestDB(t, &model.User{}, &model.RefreshToken{}, &model.Setting{})
	return repository.New(db)
}

// TestRefreshAcceptsPendingDelayedToken 验证：登录时因 SQLite 写压力未及时
// 落库的 refresh token（仍在后台补写队列中）在刷新时被接受，而不是把用户
// 踢回登录页（历史上「经常登录报错」的来源之一）。
func TestRefreshAcceptsPendingDelayedToken(t *testing.T) {
	repos := newTokenTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	svc := NewTokenService(cfg, zap.NewNop(), repos)

	u := &model.User{Username: "u1", PasswordHash: "x", Role: "user", Tier: "free", IsActive: true}
	if err := repos.User.Create(t.Context(), u); err != nil {
		t.Fatal(err)
	}

	refreshToken := "pending-token-value"
	hash := repository.HashToken(refreshToken)
	if !svc.trackDelayedStore(u.ID, hash, time.Now().Add(time.Hour)) {
		t.Fatal("trackDelayedStore returned false")
	}

	pair, err := svc.Refresh(t.Context(), refreshToken)
	if err != nil {
		t.Fatalf("Refresh rejected pending delayed token: %v", err)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("Refresh returned incomplete pair: %+v", pair)
	}
	// 轮换后旧令牌应从 pending 表移除，不能再次使用。
	if _, still := svc.pendingDelayedStore(hash); still {
		t.Fatal("rotated pending token still tracked")
	}
	if _, err := svc.Refresh(t.Context(), refreshToken); err == nil {
		t.Fatal("rotated pending token should not refresh twice")
	}
}

// TestRefreshRejectsExpiredPendingToken 验证过期的待落库令牌不会被接受。
func TestRefreshRejectsExpiredPendingToken(t *testing.T) {
	repos := newTokenTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	svc := NewTokenService(cfg, zap.NewNop(), repos)

	refreshToken := "expired-pending"
	hash := repository.HashToken(refreshToken)
	svc.trackDelayedStore("user-x", hash, time.Now().Add(-time.Minute))

	if _, err := svc.Refresh(t.Context(), refreshToken); err == nil {
		t.Fatal("expired pending token should be rejected")
	}
}
