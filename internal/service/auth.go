// Package service — authentication / user management.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// AuthService handles registration, login, and JWT issuance.
type AuthService struct {
	cfg           *config.Config
	log           *zap.Logger
	repo          *repository.Container
	tokenSvc      *TokenService
	permissionSvc *PermissionService
}

// NewAuthService is the constructor.
func NewAuthService(cfg *config.Config, log *zap.Logger, repo *repository.Container, tokenSvc *TokenService, permissionSvc *PermissionService) *AuthService {
	return &AuthService{cfg: cfg, log: log, repo: repo, tokenSvc: tokenSvc, permissionSvc: permissionSvc}
}

// Common service-level errors.
var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUsernameTaken      = errors.New("username already taken")
	ErrUserInactive       = errors.New("user account is inactive")
	ErrUserLimitReached   = errors.New("user limit reached")
)

// MaxUsers is kept for compatibility with tests and callers; dynamic runtime
// checks use LicensedMaxUsers so official licensed builds can raise the quota.
const MaxUsers = OpenSourceUserLimit

// SeedAdmin makes sure at least one admin user exists. It mirrors the
// MediaStation behaviour: if no admin row is found we create
// `admin / admin123` (overridable through ADMIN_INITIAL_PASSWORD) and warn.
func (s *AuthService) SeedAdmin(ctx context.Context) error {
	n, err := s.repo.User.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	pwd := os.Getenv("ADMIN_INITIAL_PASSWORD")
	if pwd == "" {
		pwd = "admin123"
	}
	hash, err := hashPassword(pwd)
	if err != nil {
		return err
	}
	user := &model.User{
		Username:           "admin",
		PasswordHash:       hash,
		Role:               "admin",
		Tier:               "plus",
		HideAdult:          true,
		ForcePasswordReset: pwd == "admin123",
	}
	if err := s.repo.User.Create(ctx, user); err != nil {
		return err
	}
	// 确保管理员有权限记录
	_, _ = s.permissionSvc.EnsureForUser(ctx, user.ID)
	s.log.Warn("default admin created — change the password after first login",
		zap.String("username", "admin"),
		zap.String("password_source", "ADMIN_INITIAL_PASSWORD or admin123"),
	)
	return nil
}

// Register creates a new user. The first registered user is auto-promoted to
// admin to support fresh installs that did not run SeedAdmin.
func (s *AuthService) Register(ctx context.Context, username, password string) (*model.User, *TokenPair, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, nil, fmt.Errorf("username and password required")
	}
	if existing, err := s.repo.User.FindByUsername(ctx, username); err != nil {
		return nil, nil, err
	} else if existing != nil {
		return nil, nil, ErrUsernameTaken
	}
	if err := s.repo.User.ReleaseDeletedUsername(ctx, username); err != nil {
		return nil, nil, err
	}
	if n, err := s.repo.User.Count(ctx); err != nil {
		return nil, nil, err
	} else if n >= LicensedMaxUsers(ctx, s.repo) {
		return nil, nil, ErrUserLimitReached
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, nil, err
	}
	role := "user"
	if n, err := s.repo.User.CountAdmins(ctx); err == nil && n == 0 {
		role = "admin"
	}
	u := &model.User{
		Username:     username,
		PasswordHash: hash,
		Role:         role,
		Tier:         "free",
		HideAdult:    true,
	}
	if err := s.repo.User.Create(ctx, u); err != nil {
		return nil, nil, err
	}
	// 自动为新用户创建默认权限
	_, _ = s.permissionSvc.EnsureForUser(ctx, u.ID)
	// 签发令牌对
	tokens, err := s.tokenSvc.IssuePair(ctx, u.ID, u.Role, u.Tier)
	if err != nil {
		return u, nil, nil // 用户已创建，令牌签发失败不影响注册成功
	}
	return u, tokens, nil
}

// LoginResponse 登录响应结构。
type LoginResponse struct {
	User   *model.User `json:"user"`
	Tokens *TokenPair  `json:"tokens"`
}

// Login validates credentials and returns the user + a fresh JWT token pair.
func (s *AuthService) Login(ctx context.Context, username, password string) (*LoginResponse, error) {
	u, err := s.repo.User.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, ErrInvalidCredentials
	}
	// 检查用户是否激活
	if !u.IsActive {
		return nil, ErrUserInactive
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	// 签发令牌对
	tokens, err := s.tokenSvc.IssuePair(ctx, u.ID, u.Role, u.Tier)
	if err != nil {
		return nil, err
	}
	_ = s.repo.User.TouchLogin(ctx, u.ID)
	return &LoginResponse{User: u, Tokens: tokens}, nil
}

// ChangePassword updates the user password if the old one matches.
func (s *AuthService) ChangePassword(ctx context.Context, userID, oldPwd, newPwd string) error {
	if strings.TrimSpace(newPwd) == "" || len(newPwd) < 6 {
		return errors.New("new password must be at least 6 characters")
	}
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPwd)); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := hashPassword(newPwd)
	if err != nil {
		return err
	}
	return s.repo.User.UpdatePassword(ctx, userID, hash)
}

// ResetPassword lets an administrator set a new password without knowing the
// user's old password.
func (s *AuthService) ResetPassword(ctx context.Context, userID, newPwd string) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("missing user id")
	}
	if strings.TrimSpace(newPwd) == "" || len(newPwd) < 6 {
		return errors.New("new password must be at least 6 characters")
	}
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil {
		return errors.New("user not found")
	}
	hash, err := hashPassword(newPwd)
	if err != nil {
		return err
	}
	return s.repo.User.UpdatePassword(ctx, userID, hash)
}

// VerifyPassword checks a user's current password without mutating account
// state. It is used for sensitive self-service actions such as hiding adult
// libraries or deleting play profiles.
func (s *AuthService) VerifyPassword(ctx context.Context, userID, password string) error {
	u, err := s.repo.User.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if u == nil || strings.TrimSpace(password) == "" {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// IssueToken signs a JWT for the given user (60min validity, includes tier).
func (s *AuthService) IssueToken(u *model.User) (string, error) {
	claims := Claims{
		UserID: u.ID,
		Role:   u.Role,
		Tier:   u.Tier,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(60 * time.Minute)),
			Issuer:    "mediastationgo",
			Subject:   u.ID,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(s.cfg.Secrets.JWTSecret))
}

// EmbyTokenDuration 是第三方 Emby/Jellyfin 客户端访问令牌的有效期。
// Emby 协议没有 refresh token 机制——客户端登录一次后把 AccessToken
// 长期保存并反复使用，直到用户主动登出。若给它们签发 60 分钟的普通
// access token，客户端每小时就会掉登录、无法播放、媒体库无法刷新。
// 因此为这些设备签发长期令牌（与 refresh token 一致的 30 天），匹配
// Emby 持久化令牌的语义。
const EmbyTokenDuration = 30 * 24 * time.Hour

// IssueEmbyToken 为第三方客户端（Emby/Jellyfin 兼容层）签发一个长期
// JWT。它与普通 access token 使用相同的密钥与 Claims，因此沿用现有的
// EmbyAuthRequired 校验逻辑，只是有效期更长。
func (s *AuthService) IssueEmbyToken(u *model.User) (string, error) {
	claims := Claims{
		UserID: u.ID,
		Role:   u.Role,
		Tier:   u.Tier,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(EmbyTokenDuration)),
			Issuer:    "mediastationgo",
			Subject:   u.ID,
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(s.cfg.Secrets.JWTSecret))
}

// RefreshTokens 使用刷新令牌获取新的令牌对。
func (s *AuthService) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	return s.tokenSvc.Refresh(ctx, refreshToken)
}

// Logout 撤销用户的所有刷新令牌。
func (s *AuthService) Logout(ctx context.Context, userID string) error {
	return s.tokenSvc.RevokeAll(ctx, userID)
}

func hashPassword(p string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}
