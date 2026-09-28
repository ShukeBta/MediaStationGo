package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// The shared PostgreSQL fixture requires MEDIASTATION_TEST_POSTGRES_DSN and
// creates/drops a random disposable database; it never migrates the DSN's DB.
func postgresTokenFixture(t *testing.T) (*TokenService, *TokenService, *model.User, *TokenPair) {
	t.Helper()
	db, cfg := postgresBackupTestDatabase(t)
	if err := db.AutoMigrate(&model.User{}, &model.RefreshToken{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	cfg.Secrets.JWTSecret = "postgres-token-test-secret"
	repos := repository.New(db)
	first := NewTokenService(cfg, zap.NewNop(), repos)
	second := NewTokenService(cfg, zap.NewNop(), repos)
	user := &model.User{Username: "token-concurrency", Role: "user", Tier: "free", IsActive: true}
	if err := repos.User.Create(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	pair, err := first.IssuePair(t.Context(), user.ID, user.Role, user.Tier)
	if err != nil {
		t.Fatal(err)
	}
	return first, second, user, pair
}

func postgresTokenWait(t *testing.T, ready <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func TestPostgresRefreshTokenConsumedOnceAcrossServiceInstances(t *testing.T) {
	first, second, _, pair := postgresTokenFixture(t)
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	if err := first.repo.DB.Callback().Query().After("gorm:query").Register("postgres-token-read-barrier", func(tx *gorm.DB) {
		// Pause both callers after validating the same active token/user, before
		// either reaches the transactional SELECT FOR UPDATE (which selects id).
		if _, ok := tx.Statement.Dest.(*model.User); ok && len(tx.Statement.Selects) == 0 {
			arrived <- struct{}{}
			select {
			case <-release:
			case <-tx.Statement.Context.Done():
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	results := make(chan error, 2)
	for _, instance := range []*TokenService{first, second} {
		go func(svc *TokenService) {
			_, err := svc.Refresh(ctx, pair.RefreshToken)
			results <- err
		}(instance)
	}
	postgresTokenWait(t, arrived, "first caller reading the active token")
	postgresTokenWait(t, arrived, "second caller reading the active token")
	unblock()
	succeeded, rejected := 0, 0
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				succeeded++
			} else if errors.Is(err, ErrTokenRevoked) {
				rejected++
			} else {
				t.Fatalf("unexpected rotation failure: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("concurrent PostgreSQL rotations did not finish")
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("rotation results: successful=%d rejected=%d", succeeded, rejected)
	}
	var active int64
	if err := first.repo.DB.Model(&model.RefreshToken{}).Where("revoked = ?", false).Count(&active).Error; err != nil || active != 1 {
		t.Fatalf("active replacement count=%d err=%v", active, err)
	}
	t.Log("two service instances consumed one PostgreSQL token exactly once")
}

type postgresTokenOperationKey struct{}

func TestPostgresRefreshRotationAndLogoutSerialize(t *testing.T) {
	for _, firstOperation := range []string{"rotation", "logout"} {
		t.Run(firstOperation+"-first", func(t *testing.T) {
			first, second, user, pair := postgresTokenFixture(t)
			db := first.repo.DB
			paused, release := make(chan struct{}), make(chan struct{})
			var pauseOnce, releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			rotationCtx := context.WithValue(ctx, postgresTokenOperationKey{}, "rotation")
			logoutCtx := context.WithValue(ctx, postgresTokenOperationKey{}, "logout")
			secondOperation := "logout"
			if firstOperation == "logout" {
				secondOperation = "rotation"
			}
			// Capture the second transaction's backend before its first locking
			// statement. The fallback Update hook also works without the fix, so
			// the regression demonstrates the READ COMMITTED snapshot defect.
			backend := make(chan int, 1)
			var backendOnce sync.Once
			captureBackend := func(tx *gorm.DB) {
				if tx.Statement.Context.Value(postgresTokenOperationKey{}) != secondOperation {
					return
				}
				backendOnce.Do(func() {
					var pid int
					if err := tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
						tx.AddError(err)
						return
					}
					backend <- pid
				})
			}
			if err := db.Callback().Query().Before("gorm:query").Register("postgres-token-lock-observer", func(tx *gorm.DB) {
				if _, locking := tx.Statement.Clauses["FOR"]; locking {
					captureBackend(tx)
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Update().Before("gorm:update").Register("postgres-token-update-barrier", func(tx *gorm.DB) {
				if tx.Statement.Table != "refresh_tokens" {
					return
				}
				captureBackend(tx)
				if firstOperation == "logout" && tx.Statement.Context.Value(postgresTokenOperationKey{}) == "logout" {
					pauseOnce.Do(func() { close(paused) })
					select {
					case <-release:
					case <-ctx.Done():
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Create().Before("gorm:create").Register("postgres-token-insert-barrier", func(tx *gorm.DB) {
				if firstOperation == "rotation" && tx.Statement.Table == "refresh_tokens" && tx.Statement.Context.Value(postgresTokenOperationKey{}) == "rotation" {
					pauseOnce.Do(func() { close(paused) })
					select {
					case <-release:
					case <-ctx.Done():
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			rotated, loggedOut := make(chan error, 1), make(chan error, 1)
			rotate := func() { _, err := first.Refresh(rotationCtx, pair.RefreshToken); rotated <- err }
			logout := func() { loggedOut <- second.RevokeAll(logoutCtx, user.ID) }
			if firstOperation == "rotation" {
				go rotate()
			} else {
				go logout()
			}
			postgresTokenWait(t, paused, firstOperation+" holding its transaction open")
			if firstOperation == "rotation" {
				go logout()
			} else {
				go rotate()
			}
			var pid int
			select {
			case pid = <-backend:
			case <-time.After(5 * time.Second):
				t.Fatal("second transaction did not reach PostgreSQL")
			}
			postgresTokenWaitForLock(t, ctx, db, pid)
			unblock()
			select {
			case err := <-loggedOut:
				if err != nil {
					t.Fatalf("logout: %v", err)
				}
			case <-ctx.Done():
				t.Fatal("logout did not finish")
			}
			select {
			case err := <-rotated:
				if firstOperation == "rotation" && err != nil {
					t.Fatalf("rotation before logout: %v", err)
				}
				if firstOperation == "logout" && !errors.Is(err, ErrTokenRevoked) {
					t.Fatalf("rotation after logout should be rejected, got %v", err)
				}
			case <-ctx.Done():
				t.Fatal("rotation did not finish")
			}
			var active int64
			if err := db.Model(&model.RefreshToken{}).Where("user_id = ? AND revoked = ?", user.ID, false).Count(&active).Error; err != nil || active != 0 {
				t.Fatalf("logout missed an in-flight replacement: active=%d err=%v", active, err)
			}
			t.Logf("%s first: observed PostgreSQL lock contention; logout left zero active tokens", firstOperation)
		})
	}
}

func postgresTokenWaitForLock(t *testing.T, ctx context.Context, db *gorm.DB, pid int) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := db.WithContext(ctx).Raw("SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid = ? AND datname = current_database() AND wait_event_type = 'Lock')", pid).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("second operation did not wait on a PostgreSQL transaction lock")
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
