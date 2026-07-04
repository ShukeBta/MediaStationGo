package service

import (
	"errors"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func newPlayProfileTestService(t *testing.T) *PlayProfileService {
	t.Helper()
	db := newServiceTestDB(t, &model.PlayProfile{})
	return NewPlayProfileService(zap.NewNop(), repository.New(db))
}

func TestPlayProfileVerifyPIN(t *testing.T) {
	service := newPlayProfileTestService(t)
	profile, err := service.Create(t.Context(), PlayProfileInput{
		UserID:     "user-1",
		Name:       "成人模式",
		AllowAdult: true,
		RequirePIN: true,
		PIN:        "1234",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.VerifyPIN(t.Context(), profile.ID, "user-1", "0000"); !errors.Is(err, ErrPlayProfilePINInvalid) {
		t.Fatalf("wrong PIN error = %v", err)
	}
	if _, err := service.VerifyPIN(t.Context(), profile.ID, "user-2", "1234"); !errors.Is(err, ErrPlayProfileForbidden) {
		t.Fatalf("wrong owner error = %v", err)
	}
	if verified, err := service.VerifyPIN(t.Context(), profile.ID, "user-1", "1234"); err != nil || verified.ID != profile.ID {
		t.Fatalf("verify PIN got profile=%v err=%v", verified, err)
	}
}

func TestPlayProfileCreateRequiresPINWhenEnabled(t *testing.T) {
	service := newPlayProfileTestService(t)
	if _, err := service.Create(t.Context(), PlayProfileInput{
		UserID:     "user-1",
		Name:       "锁定模式",
		RequirePIN: true,
	}); err == nil {
		t.Fatal("expected PIN-required profile create to fail without PIN")
	}
}

func TestPlayProfileCreateLimitIsPerUser(t *testing.T) {
	service := newPlayProfileTestService(t)

	for i := 1; i <= MaxPlayProfilesPerUser; i++ {
		if _, err := service.Create(t.Context(), PlayProfileInput{
			UserID: "user-1",
			Name:   "模式 " + string(rune('0'+i)),
		}); err != nil {
			t.Fatalf("create profile %d: %v", i, err)
		}
	}

	if _, err := service.Create(t.Context(), PlayProfileInput{
		UserID: "user-1",
		Name:   "超限模式",
	}); !errors.Is(err, ErrPlayProfileLimit) {
		t.Fatalf("expected limit error, got %v", err)
	}

	if _, err := service.Create(t.Context(), PlayProfileInput{
		UserID: "user-2",
		Name:   "另一个用户的模式",
	}); err != nil {
		t.Fatalf("different user should have independent limit: %v", err)
	}
}

func TestPlayProfileUpdateDeleteRequireOwner(t *testing.T) {
	service := newPlayProfileTestService(t)
	profile, err := service.Create(t.Context(), PlayProfileInput{
		UserID: "user-1",
		Name:   "私人模式",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.UpdateForUser(t.Context(), profile.ID, "user-2", PlayProfileInput{
		Name: "越权修改",
	}); !errors.Is(err, ErrPlayProfileForbidden) {
		t.Fatalf("expected forbidden update, got %v", err)
	}

	if err := service.DeleteForUser(t.Context(), profile.ID, "user-2"); !errors.Is(err, ErrPlayProfileForbidden) {
		t.Fatalf("expected forbidden delete, got %v", err)
	}

	updated, err := service.UpdateForUser(t.Context(), profile.ID, "user-1", PlayProfileInput{
		Name: "已修改",
	})
	if err != nil {
		t.Fatalf("owner update failed: %v", err)
	}
	if updated.Name != "已修改" || updated.UserID != "user-1" {
		t.Fatalf("unexpected updated profile: %+v", updated)
	}

	if err := service.DeleteForUser(t.Context(), profile.ID, "user-1"); err != nil {
		t.Fatalf("owner delete failed: %v", err)
	}
}
