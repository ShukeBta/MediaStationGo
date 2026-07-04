package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func newAuthTestServices(t *testing.T) (*repository.Container, *AuthService, *ProfileService, *PermissionService) {
	t.Helper()
	db := newServiceTestDB(t, &model.User{}, &model.UserPermission{}, &model.RefreshToken{}, &model.TelegramBinding{}, &model.Setting{})
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	permissions := NewPermissionService(log, repos)
	tokenSvc := NewTokenService(cfg, log, repos)
	auth := NewAuthService(cfg, log, repos, tokenSvc, permissions)
	profile := NewProfileService(log, repos)
	return repos, auth, profile, permissions
}

func TestRegisterRejectsMoreThanTwentyUsers(t *testing.T) {
	ctx := context.Background()
	repos, auth, _, _ := newAuthTestServices(t)
	for i := 0; i < MaxUsers; i++ {
		if err := repos.User.Create(ctx, &model.User{
			Username:     fmt.Sprintf("user-%02d", i),
			PasswordHash: "hash",
			Role:         "user",
			Tier:         "free",
		}); err != nil {
			t.Fatal(err)
		}
	}

	_, _, err := auth.Register(ctx, "overflow", "password")
	if !errors.Is(err, ErrUserLimitReached) {
		t.Fatalf("expected ErrUserLimitReached, got %v", err)
	}
}

func TestRegisterUsesLicensedUserLimit(t *testing.T) {
	ctx := context.Background()
	repos, auth, _, _ := newAuthTestServices(t)
	maxUsers := 25
	state := LicenseActivationState{Valid: true, LicenseType: "plus", MaxUsers: &maxUsers}
	raw, _ := json.Marshal(state)
	if err := repos.Setting.Set(ctx, LicenseSettingActivation, string(raw)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < OpenSourceUserLimit; i++ {
		if err := repos.User.Create(ctx, &model.User{
			Username:     fmt.Sprintf("licensed-%02d", i),
			PasswordHash: "hash",
			Role:         "user",
			Tier:         "free",
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, _, err := auth.Register(ctx, "extra", "password"); err != nil {
		t.Fatalf("licensed user limit should allow user 21: %v", err)
	}
}

func TestLicensedMaxUsersCanBeUnlimited(t *testing.T) {
	ctx := context.Background()
	repos, _, _, _ := newAuthTestServices(t)
	state := LicenseActivationState{Valid: true, LicenseType: "enterprise", UnlimitedUsers: true}
	raw, _ := json.Marshal(state)
	if err := repos.Setting.Set(ctx, LicenseSettingActivation, string(raw)); err != nil {
		t.Fatal(err)
	}
	if got := LicensedMaxUsers(ctx, repos); got <= 1_000_000 {
		t.Fatalf("unlimited license should return a very high limit, got %d", got)
	}
}

func TestRegisterDefaultsAdultLibrariesHidden(t *testing.T) {
	_, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(context.Background(), "viewer", "password")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !user.HideAdult {
		t.Fatal("new users should hide adult libraries by default")
	}
}

func TestDeletedUserCanBeRecreatedWithSameUsername(t *testing.T) {
	ctx := context.Background()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "old-password")
	if err != nil {
		t.Fatalf("register old user: %v", err)
	}
	if err := repos.DB.Create(&model.TelegramBinding{
		TelegramUserID: 10001,
		TelegramName:   "@viewer",
		ChatID:         10001,
		UserID:         user.ID,
	}).Error; err != nil {
		t.Fatalf("create telegram binding: %v", err)
	}
	if err := repos.User.Delete(ctx, user.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	next, _, err := auth.Register(ctx, "viewer", "new-password")
	if err != nil {
		t.Fatalf("register same username after delete: %v", err)
	}
	if next.ID == user.ID {
		t.Fatal("recreated user should be a new account row")
	}
	if _, err := auth.Login(ctx, "viewer", "new-password"); err != nil {
		t.Fatalf("login recreated user: %v", err)
	}
	var bindings int64
	if err := repos.DB.Model(&model.TelegramBinding{}).Where("telegram_user_id = ?", 10001).Count(&bindings).Error; err != nil {
		t.Fatalf("count bindings: %v", err)
	}
	if bindings != 0 {
		t.Fatalf("deleted user telegram bindings should be removed, got %d", bindings)
	}
}

func TestRegisterReleasesLegacySoftDeletedUsername(t *testing.T) {
	ctx := context.Background()
	repos, auth, _, _ := newAuthTestServices(t)
	if err := repos.User.Create(ctx, &model.User{
		Username:     "legacy",
		PasswordHash: "hash",
		Role:         "user",
		Tier:         "free",
	}); err != nil {
		t.Fatal(err)
	}
	legacy, err := repos.User.FindByUsername(ctx, "legacy")
	if err != nil || legacy == nil {
		t.Fatalf("find legacy user: %v", err)
	}
	if err := repos.DB.Delete(&model.User{}, "id = ?", legacy.ID).Error; err != nil {
		t.Fatalf("legacy soft delete: %v", err)
	}

	if _, _, err := auth.Register(ctx, "legacy", "new-password"); err != nil {
		t.Fatalf("register should release old soft-deleted username: %v", err)
	}
}

func TestAdminResetPasswordAllowsLoginWithNewPassword(t *testing.T) {
	ctx := context.Background()
	_, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "old-password")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if err := auth.ResetPassword(ctx, user.ID, "new-password"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
	if _, err := auth.Login(ctx, "viewer", "old-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password should fail, got %v", err)
	}
	if _, err := auth.Login(ctx, "viewer", "new-password"); err != nil {
		t.Fatalf("new password should login: %v", err)
	}
}

func TestLoginAcceptsUsernameCaseVariant(t *testing.T) {
	ctx := context.Background()
	_, auth, _, _ := newAuthTestServices(t)
	if _, _, err := auth.Register(ctx, "viewer", "password"); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := auth.Login(ctx, "Viewer", "password"); err != nil {
		t.Fatalf("case variant username should login: %v", err)
	}
}

func TestLoginKeepsOnlyConfiguredActiveRefreshTokens(t *testing.T) {
	ctx := context.Background()
	repos, auth, _, _ := newAuthTestServices(t)
	user, _, err := auth.Register(ctx, "viewer", "password")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.Setting.Set(ctx, SettingMaxLoggedClients, "3"); err != nil {
		t.Fatalf("set max clients: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := auth.Login(ctx, "viewer", "password"); err != nil {
			t.Fatalf("login %d: %v", i+1, err)
		}
	}

	var active int64
	if err := repos.DB.Model(&model.RefreshToken{}).
		Where("user_id = ? AND revoked = ? AND expires_at > ?", user.ID, false, time.Now()).
		Count(&active).Error; err != nil {
		t.Fatalf("count active refresh tokens: %v", err)
	}
	if active != 3 {
		t.Fatalf("active refresh tokens should be capped at 3, got %d", active)
	}
}

func TestLoginRetriesTransientSQLiteBusy(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	cfg.Database.DBPath = filepath.Join(cfg.App.DataDir, "busy-login.db")
	cfg.Database.WALMode = true
	cfg.Database.BusyTimeout = 20
	cfg.Database.MaxOpenConns = 4
	cfg.Database.MaxIdleConns = 2
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	db, err := database.Open(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	permissions := NewPermissionService(log, repos)
	auth := NewAuthService(cfg, log, repos, NewTokenService(cfg, log, repos), permissions)
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.User.Create(ctx, &model.User{
		Username:     "viewer",
		PasswordHash: string(hash),
		Role:         "user",
		Tier:         "free",
		IsActive:     true,
	}); err != nil {
		t.Fatal(err)
	}

	tx := repos.DB.Begin()
	if err := tx.Exec("UPDATE users SET updated_at = updated_at WHERE username = ?", "viewer").Error; err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	go func() {
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-release:
		case <-timer.C:
		}
		_ = tx.Rollback().Error
	}()
	defer close(release)

	if _, err := auth.Login(ctx, "viewer", "password"); err != nil {
		t.Fatalf("login should survive a transient sqlite write lock: %v", err)
	}
}

func TestLoginSurvivesOneReservedSQLiteConnectionWithWALPool(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	cfg.Database.DBPath = filepath.Join(cfg.App.DataDir, "reserved-connection-login.db")
	cfg.Database.WALMode = true
	cfg.Database.BusyTimeout = 20
	cfg.Database.MaxOpenConns = 4
	cfg.Database.MaxIdleConns = 2
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	db, err := database.Open(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	permissions := NewPermissionService(log, repos)
	auth := NewAuthService(cfg, log, repos, NewTokenService(cfg, log, repos), permissions)
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := repos.User.Create(ctx, &model.User{
		Username:     "viewer",
		PasswordHash: string(hash),
		Role:         "user",
		Tier:         "free",
		IsActive:     true,
	}); err != nil {
		t.Fatal(err)
	}

	reserved, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reserved.Close() }()

	loginCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := auth.Login(loginCtx, "viewer", "password"); err != nil {
		t.Fatalf("login should not be blocked by one reserved sqlite connection: %v", err)
	}
}

func TestLoginReturnsTokensWhenSQLiteWriteLockPersists(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	cfg.Database.DBPath = filepath.Join(cfg.App.DataDir, "busy-login-degraded.db")
	cfg.Database.WALMode = true
	cfg.Database.BusyTimeout = 20
	cfg.Database.MaxOpenConns = 4
	cfg.Database.MaxIdleConns = 2
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	db, err := database.Open(cfg, log)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sqlDB.Close() }()
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	permissions := NewPermissionService(log, repos)
	auth := NewAuthService(cfg, log, repos, NewTokenService(cfg, log, repos), permissions)
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := &model.User{
		Username:     "viewer",
		PasswordHash: string(hash),
		Role:         "user",
		Tier:         "free",
		IsActive:     true,
	}
	if err := repos.User.Create(ctx, user); err != nil {
		t.Fatal(err)
	}

	tx := repos.DB.Begin()
	if err := tx.Exec("UPDATE users SET updated_at = updated_at WHERE username = ?", "viewer").Error; err != nil {
		t.Fatal(err)
	}
	resp, err := auth.Login(ctx, "viewer", "password")
	if err != nil {
		t.Fatalf("login should return tokens while refresh token store is delayed: %v", err)
	}
	if resp == nil || resp.Tokens == nil || resp.Tokens.AccessToken == "" || resp.Tokens.RefreshToken == "" {
		t.Fatalf("login returned incomplete token pair: %#v", resp)
	}

	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	wantHash := repository.HashToken(resp.Tokens.RefreshToken)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var count int64
		if err := repos.DB.Model(&model.RefreshToken{}).Where("token_hash = ?", wantHash).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("delayed refresh token store did not complete")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestDefaultPermissionsAreViewerOnly(t *testing.T) {
	perms := DefaultPermissions("user-1")
	if !perms.CanViewDashboard || !perms.CanPlayMedia || !perms.CanExternalPlayer {
		t.Fatal("viewer defaults must allow library viewing, playback, and external players")
	}
	if perms.CanManageDownloads || perms.CanManageSubscriptions || perms.CanManageFiles ||
		perms.CanEditMedia || perms.CanRescrape || perms.CanCaptureFrames ||
		perms.CanManageSites || perms.CanManageUsers || perms.CanManageStrm {
		t.Fatal("viewer defaults must not allow downloads, scraping, media edits, or file management")
	}
}

func TestAdminEffectivePermissionsAreAllGranted(t *testing.T) {
	ctx := context.Background()
	repos, _, _, permissions := newAuthTestServices(t)
	admin := &model.User{Username: "admin", PasswordHash: "hash", Role: "admin", Tier: "plus"}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}

	perms, err := permissions.Effective(ctx, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !perms.CanEditMedia || !perms.CanRescrape || !perms.CanUseAI ||
		!perms.CanCaptureFrames || !perms.CanManageUsers || !perms.CanAccessSettings {
		t.Fatal("admin effective permissions must grant every advanced capability")
	}
}

func TestDefaultAdminCannotBeDemoted(t *testing.T) {
	ctx := context.Background()
	repos, _, profile, _ := newAuthTestServices(t)
	admin := &model.User{Username: "admin", PasswordHash: "hash", Role: "admin", Tier: "plus"}
	if err := repos.User.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}

	_, err := profile.AdminUpdateRole(ctx, admin.ID, "user")
	if err == nil {
		t.Fatal("expected default admin demotion to be rejected")
	}
}

// TestIssueEmbyTokenIsLongLived verifies the Emby/Jellyfin compatibility token
// outlives the 60-minute access token (Emby clients have no refresh mechanism,
// so a short token caused them to drop login / fail playback hourly), parses
// with the JWT secret, and carries the user's identity/role/tier.
func TestIssueEmbyTokenIsLongLived(t *testing.T) {
	_, auth, _, _ := newAuthTestServices(t)
	u := &model.User{Base: model.Base{ID: "u-emby"}, Username: "emby", Role: "user", Tier: "plus"}

	tok, err := auth.IssueEmbyToken(u)
	if err != nil {
		t.Fatalf("IssueEmbyToken: %v", err)
	}

	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("token did not parse/validate: %v", err)
	}
	if claims.UserID != "u-emby" || claims.Role != "user" || claims.Tier != "plus" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	ttl := time.Until(claims.ExpiresAt.Time)
	if ttl <= AccessTokenDuration {
		t.Fatalf("emby token ttl %v not longer than access token ttl %v", ttl, AccessTokenDuration)
	}
	if ttl < EmbyTokenDuration-time.Hour {
		t.Fatalf("emby token ttl %v shorter than expected ~%v", ttl, EmbyTokenDuration)
	}
}
