package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestEmbyPersonImageContractAndCacheInvalidation(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Movies", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	person := model.Person{
		Name:     "Actor One",
		NameKey:  normalizePersonNameKey("Actor One"),
		ImageURL: "https://images.example.test/actor-one-v1.jpg",
		Source:   "tmdb",
		SourceID: "123",
	}
	if err := svc.repo.DB.Create(&person).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:      model.Base{ID: "movie-with-person-image"},
		LibraryID: lib.ID,
		Title:     "Movie",
		Path:      "/media/movies/movie.mkv",
		Actors:    "Actor One",
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	persons, err := svc.Persons(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	personItems := persons["Items"].([]map[string]any)
	if len(personItems) != 1 {
		t.Fatalf("persons = %#v", personItems)
	}
	personTags, ok := personItems[0]["ImageTags"].(map[string]string)
	if !ok || personTags["Primary"] == "" {
		t.Fatalf("person image tags = %#v", personItems[0]["ImageTags"])
	}

	item, err := svc.Item(t.Context(), media.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	people, ok := item["People"].([]model.EmbyPerson)
	if !ok || len(people) != 1 || people[0].PrimaryImageTag != personTags["Primary"] {
		t.Fatalf("media people = %#v", item["People"])
	}
	personID := embyPersonID("Actor One")
	imageURL, err := svc.ImageURL(t.Context(), personID, "Primary")
	if err != nil {
		t.Fatal(err)
	}
	if imageURL != person.ImageURL {
		t.Fatalf("image URL = %q, want %q", imageURL, person.ImageURL)
	}

	scraper := &ScraperService{repo: repository.New(svc.repo.DB)}
	updatedImage := "https://images.example.test/actor-one-v2.jpg"
	if err := scraper.persistPeople(t.Context(), []PersonMetadata{{
		Name:     "Actor One",
		ImageURL: updatedImage,
		Source:   "tmdb",
		SourceID: "123",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	refreshed, err := svc.Item(t.Context(), media.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	refreshedPeople := refreshed["People"].([]model.EmbyPerson)
	if refreshedPeople[0].PrimaryImageTag == people[0].PrimaryImageTag {
		t.Fatalf("person image tag did not change after metadata update")
	}
	imageURL, err = svc.ImageURL(t.Context(), personID, "Primary")
	if err != nil {
		t.Fatal(err)
	}
	if imageURL != updatedImage {
		t.Fatalf("updated image URL = %q, want %q", imageURL, updatedImage)
	}
}
