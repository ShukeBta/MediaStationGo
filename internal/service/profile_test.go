package service

import (
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestBlockedUserCannotDisableAdultHidingThroughProfileAPI(t *testing.T) {
	db := newServiceTestDB(t, &model.User{})
	repos := repository.New(db)
	viewer := &model.User{
		Base:                model.Base{ID: "blocked-viewer"},
		Username:            "blocked-viewer",
		PasswordHash:        "hash",
		Role:                "user",
		HideAdult:           true,
		AdultContentBlocked: true,
	}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	hideAdult := false
	_, err := NewProfileService(zap.NewNop(), repos).UpdateProfile(
		t.Context(), viewer.ID, ProfileUpdate{HideAdult: &hideAdult},
	)
	if !errors.Is(err, ErrAdultContentBlockedByAdmin) {
		t.Fatalf("expected administrator block error, got %v", err)
	}
	updated, findErr := repos.User.FindByID(t.Context(), viewer.ID)
	if findErr != nil {
		t.Fatal(findErr)
	}
	if !updated.HideAdult || !updated.AdultContentBlocked {
		t.Fatalf("blocked profile flags changed unexpectedly: %#v", updated)
	}
}
