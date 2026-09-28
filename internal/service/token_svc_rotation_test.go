package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func tokenRotationFixture(t *testing.T) (*TokenService, *model.User) {
	t.Helper()
	repos := newTokenTestRepo(t)
	sqlDB, err := repos.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	// Keep the in-memory database on one connection; transaction callbacks below
	// run after their queries have released that connection.
	sqlDB.SetMaxOpenConns(1)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	svc := NewTokenService(cfg, zap.NewNop(), repos)
	u := &model.User{Username: "rotation-user", PasswordHash: "x", Role: "user", Tier: "free", IsActive: true}
	if err := repos.User.Create(t.Context(), u); err != nil {
		t.Fatal(err)
	}
	return svc, u
}

func TestRevokeAllRemovesPendingRefreshTokens(t *testing.T) {
	svc, u := tokenRotationFixture(t)
	const token = "pending-before-logout"
	hash := repository.HashToken(token)
	svc.trackDelayedStore(u.ID, hash, time.Now().Add(time.Hour))
	svc.trackDelayedStore("other-user", "other-hash", time.Now().Add(time.Hour))
	if err := svc.RevokeAll(t.Context(), u.ID); err != nil {
		t.Fatal(err)
	}
	if _, pending := svc.pendingDelayedStore(hash); pending {
		t.Error("logout left its refresh token pending")
	}
	if _, pending := svc.pendingDelayedStore("other-hash"); !pending {
		t.Error("logout removed another user's token")
	}
	if pair, err := svc.Refresh(t.Context(), token); err == nil || pair != nil {
		t.Fatalf("logged-out pending token refreshed: pair=%+v err=%v", pair, err)
	}
}

func TestRefreshConsumesTokenOnceAcrossServiceInstances(t *testing.T) {
	svc, u := tokenRotationFixture(t)
	pair, err := svc.IssuePair(t.Context(), u.ID, u.Role, u.Tier)
	if err != nil {
		t.Fatal(err)
	}
	other := NewTokenService(svc.cfg, zap.NewNop(), svc.repo)
	var ready sync.WaitGroup
	ready.Add(2)
	if err := svc.repo.DB.Callback().Query().After("gorm:query").Register("rotation-read-barrier", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			ready.Done()
			ready.Wait()
		}
	}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for _, instance := range []*TokenService{svc, other} {
		go func(s *TokenService) {
			_, err := s.Refresh(t.Context(), pair.RefreshToken)
			results <- err
		}(instance)
	}
	successes := 0
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				successes++
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent rotation did not finish")
		}
	}
	if successes != 1 {
		t.Fatalf("same refresh token produced %d successful rotations, want 1", successes)
	}
}

func TestRefreshFailsClosedWhenRevocationFails(t *testing.T) {
	svc, u := tokenRotationFixture(t)
	pair, err := svc.IssuePair(t.Context(), u.ID, u.Role, u.Tier)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("refresh-token update unavailable")
	if err := svc.repo.DB.Callback().Update().Before("gorm:update").Register("fail-refresh-revocation", func(tx *gorm.DB) {
		if tx.Statement.Table == "refresh_tokens" {
			tx.AddError(wantErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Refresh(t.Context(), pair.RefreshToken)
	if got != nil || !errors.Is(err, wantErr) {
		t.Fatalf("revocation failure must not return credentials: pair=%+v err=%v", got, err)
	}
	var count int64
	if err := svc.repo.DB.Model(&model.RefreshToken{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("failed rotation created a token: count=%d err=%v", count, err)
	}
}

func TestRefreshRollsBackConsumptionWhenReplacementStoreFails(t *testing.T) {
	svc, u := tokenRotationFixture(t)
	pair, err := svc.IssuePair(t.Context(), u.ID, u.Role, u.Tier)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("refresh-token insert unavailable")
	if err := svc.repo.DB.Callback().Create().Before("gorm:create").Register("fail-refresh-replacement", func(tx *gorm.DB) {
		if tx.Statement.Table == "refresh_tokens" {
			tx.AddError(wantErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Refresh(t.Context(), pair.RefreshToken)
	if got != nil || !errors.Is(err, wantErr) {
		t.Fatalf("failed replacement must not return credentials: pair=%+v err=%v", got, err)
	}
	old, err := svc.repo.RefreshToken.FindByHash(t.Context(), repository.HashToken(pair.RefreshToken))
	if err != nil || old == nil || old.Revoked {
		t.Fatalf("failed replacement consumed old token: record=%+v err=%v", old, err)
	}
}

func TestLogoutCannotBeUndoneByInflightBestEffortStore(t *testing.T) {
	svc, u := tokenRotationFixture(t)
	// Hold the actual insert before it touches SQLite. Without lifecycle
	// coordination, logout can update zero rows and the insert then resurrects it.
	svc.repo.DB.Config.SkipDefaultTransaction = true
	entered, release, inserted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var enterOnce, insertedOnce sync.Once
	if err := svc.repo.DB.Callback().Create().Before("gorm:create").Register("pause-refresh-insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "refresh_tokens" {
			enterOnce.Do(func() { close(entered) })
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DB.Callback().Create().After("gorm:create").Register("observe-refresh-insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "refresh_tokens" {
			insertedOnce.Do(func() { close(inserted) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	login := make(chan *TokenPair, 1)
	go func() {
		pair, _ := svc.IssuePairBestEffort(t.Context(), u.ID, u.Role, u.Tier)
		login <- pair
	}()
	<-entered
	var pair *TokenPair
	select {
	case pair = <-login:
		if pair == nil {
			t.Fatal("best-effort login failed during delayed insert")
		}
	case <-time.After(1500 * time.Millisecond):
		close(release)
		t.Fatal("best-effort login waited for the blocked insert")
	}
	logout := make(chan error, 1)
	go func() { logout <- svc.RevokeAll(context.Background(), u.ID) }()
	// Give logout an opportunity to finish while the insert is blocked.
	select {
	case err := <-logout:
		if err != nil {
			t.Fatal(err)
		}
		logout <- nil
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	<-inserted
	if err := <-logout; err != nil {
		t.Fatal(err)
	}
	old, err := svc.repo.RefreshToken.FindByHash(t.Context(), repository.HashToken(pair.RefreshToken))
	if err != nil {
		t.Fatal(err)
	}
	if old != nil && !old.Revoked {
		t.Fatal("in-flight delayed insert revived a token after logout")
	}
}

func TestRotationCannotBeUndoneByInflightBestEffortStore(t *testing.T) {
	svc, u := tokenRotationFixture(t)
	svc.repo.DB.Config.SkipDefaultTransaction = true
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	if err := svc.repo.DB.Callback().Create().Before("gorm:create").Register("pause-first-refresh-insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "refresh_tokens" {
			once.Do(func() {
				close(entered)
				<-release
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	login := make(chan *TokenPair, 1)
	go func() {
		pair, _ := svc.IssuePairBestEffort(t.Context(), u.ID, u.Role, u.Tier)
		login <- pair
	}()
	<-entered
	pair := <-login
	if pair == nil {
		close(release)
		t.Fatal("best-effort login failed")
	}
	rotated := make(chan error, 1)
	go func() {
		_, err := svc.Refresh(t.Context(), pair.RefreshToken)
		rotated <- err
	}()
	// The old implementation can consume the memory-only token here while its
	// initial insert is paused, then incorrectly persist it as active afterward.
	select {
	case err := <-rotated:
		rotated <- err
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-rotated; err != nil {
		t.Fatal(err)
	}
	old, err := svc.repo.RefreshToken.FindByHash(t.Context(), repository.HashToken(pair.RefreshToken))
	if err != nil || old == nil || !old.Revoked {
		t.Fatalf("in-flight insert revived rotated token: record=%+v err=%v", old, err)
	}
	if _, err := svc.Refresh(t.Context(), pair.RefreshToken); err == nil {
		t.Fatal("rotated token accepted a second time")
	}
}

func TestConcurrentRefreshConsumesPendingTokenOnce(t *testing.T) {
	svc, u := tokenRotationFixture(t)
	const token = "concurrent-pending-token"
	svc.trackDelayedStore(u.ID, repository.HashToken(token), time.Now().Add(time.Hour))
	start := make(chan struct{})
	results := make(chan error, 16)
	for range cap(results) {
		go func() {
			<-start
			_, err := svc.Refresh(t.Context(), token)
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range cap(results) {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("pending token produced %d successful rotations, want 1", successes)
	}
}

func TestRefreshPendingReplacementFailurePreservesRetry(t *testing.T) {
	svc, u := tokenRotationFixture(t)
	const token = "pending-retry-after-database-error"
	hash := repository.HashToken(token)
	svc.trackDelayedStore(u.ID, hash, time.Now().Add(time.Hour))
	wantErr := errors.New("replacement insert failed")
	if err := svc.repo.DB.Callback().Create().Before("gorm:create").Register("fail-pending-replacement", func(tx *gorm.DB) {
		if record, ok := tx.Statement.Dest.(*model.RefreshToken); ok && record.TokenHash != hash {
			tx.AddError(wantErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if pair, err := svc.Refresh(t.Context(), token); pair != nil || !errors.Is(err, wantErr) {
		t.Fatalf("failed pending rotation returned credentials: hasPair=%t err=%v", pair != nil, err)
	}
	if _, tracked := svc.pendingDelayedStore(hash); !tracked {
		t.Fatal("failed rotation discarded the retryable pending token")
	}
	if old, err := svc.repo.RefreshToken.FindByHash(t.Context(), hash); err != nil || old != nil {
		t.Fatalf("failed transaction retained consumed token: record=%+v err=%v", old, err)
	}
	if err := svc.repo.DB.Callback().Create().Remove("fail-pending-replacement"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(t.Context(), token); err != nil {
		t.Fatalf("retry after database recovery failed: %v", err)
	}
}
