package handler

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
)

func signedTestToken(t *testing.T, secret string) string {
	t.Helper()
	return signedTestTokenWithPurpose(t, secret, "")
}

func signedTestTokenWithPurpose(t *testing.T, secret, purpose string) string {
	t.Helper()
	claims := middleware.Claims{
		UserID:  "user-1",
		Role:    "admin",
		Tier:    "plus",
		Purpose: purpose,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			Issuer:    "mediastationgo-test",
			Subject:   "user-1",
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}
