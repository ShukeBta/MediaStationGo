package service

import (
	"os"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

func TestMetadataActorEditReconcilesCreditsAndPreservesRoles(t *testing.T) {
	s, m := nfoEditingFixture(t, false)
	if err := s.repo.DB.AutoMigrate(&model.Person{}, &model.PersonCredit{}); err != nil {
		t.Fatal(err)
	}
	people := []model.Person{
		{Name: "Alice", NameKey: "alice", Aliases: "Alice Original", TranslatedName: "爱丽丝"},
		{Name: "Bob", NameKey: "bob"},
		{Name: "Director", NameKey: "director"},
	}
	if err := s.repo.DB.Create(&people).Error; err != nil {
		t.Fatal(err)
	}
	credits := []model.PersonCredit{
		{CreditKey: "alice-hero", MediaID: m.ID, PersonID: people[0].ID, Type: "Actor", OriginalRole: "Hero", Role: "主角"},
		{CreditKey: "bob-villain", MediaID: m.ID, PersonID: people[1].ID, Type: "Actor", OriginalRole: "Villain", Role: "反派"},
		{CreditKey: "director", MediaID: m.ID, PersonID: people[2].ID, Type: "Director", OriginalRole: "Director", Role: "导演"},
	}
	if err := s.repo.DB.Create(&credits).Error; err != nil {
		t.Fatal(err)
	}
	actors := "爱丽丝,Charlie"
	if _, err := s.UpdateMetadata(t.Context(), m.ID, MediaMetadataUpdate{Actors: &actors}); err != nil {
		t.Fatal(err)
	}
	var saved []model.PersonCredit
	if err := s.repo.DB.Where("media_id = ?", m.ID).Preload("Person").Find(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if len(saved) != 3 {
		t.Fatalf("wanted retained actor, new actor and director: %+v", saved)
	}
	for _, credit := range saved {
		switch credit.Person.Name {
		case "Alice":
			if credit.ID != credits[0].ID || credit.Role != "主角" || credit.OriginalRole != "Hero" {
				t.Fatalf("retained actor lost identity/role: %+v", credit)
			}
		case "Charlie":
			if credit.Type != "Actor" {
				t.Fatalf("new person is not an actor: %+v", credit)
			}
		case "Director":
			if credit.ID != credits[2].ID || credit.Role != "导演" {
				t.Fatalf("director changed: %+v", credit)
			}
		default:
			t.Fatalf("removed actor remains: %+v", credit)
		}
	}
	// A failed NFO write must also roll back actor membership.
	if err := os.WriteFile(nfoPath(m.Path), []byte(`<movie><broken>`), 0644); err != nil {
		t.Fatal(err)
	}
	empty := ""
	if _, err := s.UpdateMetadata(t.Context(), m.ID, MediaMetadataUpdate{Actors: &empty, WriteNFO: true}); err == nil {
		t.Fatal("malformed NFO should abort the edit")
	}
	var count int64
	if err := s.repo.DB.Model(&model.PersonCredit{}).Where("media_id = ?", m.ID).Count(&count).Error; err != nil || count != 3 {
		t.Fatalf("NFO failure did not roll back credits: %d, %v", count, err)
	}
	if _, err := s.UpdateMetadata(t.Context(), m.ID, MediaMetadataUpdate{Actors: &empty}); err != nil {
		t.Fatal(err)
	}
	saved = nil
	if err := s.repo.DB.Where("media_id = ?", m.ID).Find(&saved).Error; err != nil || len(saved) != 1 || saved[0].Type != "Director" {
		t.Fatalf("clearing actors changed crew: %+v, %v", saved, err)
	}
}

func TestMetadataActorAliasDoesNotDuplicatePeopleSearch(t *testing.T) {
	emby, scraper := newPeopleFeatureService(t)
	lib := model.Library{Name: "Movies", Path: "/movies", Type: "movie", Enabled: true}
	if err := emby.repo.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "Example", Path: "/movies/example.mkv", Actors: "Alice"}
	if err := emby.repo.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := scraper.persistMediaPeople(t.Context(), media.ID, &Match{Actors: []string{"Alice"}, People: []PersonMetadata{{Name: "Alice", Type: "Actor", Role: "Hero"}}}); err != nil {
		t.Fatal(err)
	}
	if err := emby.repo.DB.Model(&model.Person{}).Where("name_key = ?", "alice").Update("translated_name", "爱丽丝").Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), emby.repo)
	actors := "爱丽丝"
	if _, err := svc.UpdateMetadata(t.Context(), media.ID, MediaMetadataUpdate{Actors: &actors}); err != nil {
		t.Fatal(err)
	}
	result, err := emby.Persons(t.Context(), ItemsParams{Limit: 50})
	if err != nil || embyEnvelopeCount(result) != 1 {
		t.Fatalf("translated actor duplicated in people search: %+v, %v", result, err)
	}
}
