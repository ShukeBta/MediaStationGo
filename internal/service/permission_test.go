package service

import (
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestPermissionSavePersistsDisabledMenuValues(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserPermission{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	user := &model.User{Username: "viewer", PasswordHash: "hash", Role: "user", IsActive: true}
	if err := repos.User.Create(t.Context(), user); err != nil {
		t.Fatal(err)
	}
	permissions := NewPermissionService(zap.NewNop(), repos)
	initial := DefaultPermissions(user.ID)
	initial.CanViewDiscover = true
	if err := permissions.Save(t.Context(), user.ID, initial); err != nil {
		t.Fatal(err)
	}
	initial.CanViewDashboard = false
	initial.CanPlayMedia = false
	if err := permissions.Save(t.Context(), user.ID, initial); err != nil {
		t.Fatal(err)
	}
	got, err := permissions.Effective(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.CanViewDashboard || got.CanPlayMedia || !got.CanViewDiscover {
		t.Fatalf("permissions = %#v", got)
	}
}
