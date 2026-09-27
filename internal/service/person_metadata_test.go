package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestTMDbGetDetailsIncludesPersonImages(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/movie/42" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"original_language": "en",
			"credits": map[string]any{
				"cast": []map[string]any{{
					"id":           123,
					"name":         "Actor One",
					"profile_path": "/actor-one.jpg",
				}},
			},
		})
	}))
	defer upstream.Close()

	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	cfg.Secrets.TMDbImageProxy = "https://images.example.test/t/p"
	provider := NewTMDbProvider(cfg, zap.NewNop(), nil)

	details, err := provider.GetDetails(t.Context(), 42, "movie")
	if err != nil {
		t.Fatal(err)
	}
	if len(details.People) != 1 {
		t.Fatalf("people = %#v", details.People)
	}
	person := details.People[0]
	if person.Name != "Actor One" || person.Source != "tmdb" || person.SourceID != "123" {
		t.Fatalf("person metadata = %#v", person)
	}
	if person.ImageURL != "https://images.example.test/t/p/w500/actor-one.jpg" {
		t.Fatalf("person image = %q", person.ImageURL)
	}
	if len(details.Actors) != 1 || details.Actors[0] != "Actor One" {
		t.Fatalf("actors = %#v", details.Actors)
	}
}

func TestTopTMDbPeopleDoesNotPersistZeroSourceID(t *testing.T) {
	people := topTMDbPeople([]tmdbCreditCast{{Name: "Actor Without ID"}}, "https://images.example.test/t/p")
	if len(people) != 1 || people[0].SourceID != "" {
		t.Fatalf("people = %#v", people)
	}
}

func TestParseAdultDetailHTMLIncludesActorImage(t *testing.T) {
	body := `<html>
<h3>ABF-001 Test Title</h3>
<a class="actor-name" href="/actors/actor-one">
  <img data-src="/images/actor-one.jpg" alt="Actor One">
</a>
</html>`

	match := parseAdultDetailHTML(body, "ABF-001", "javbus", "https://adult.example.test/ABF-001")
	if match == nil || len(match.People) != 1 {
		t.Fatalf("match = %#v", match)
	}
	person := match.People[0]
	if person.Name != "Actor One" || person.Source != "javbus" {
		t.Fatalf("person metadata = %#v", person)
	}
	if person.ImageURL != "https://adult.example.test/images/actor-one.jpg" {
		t.Fatalf("person image = %q", person.ImageURL)
	}
	if person.ProfileURL != "https://adult.example.test/actors/actor-one" {
		t.Fatalf("person profile = %q", person.ProfileURL)
	}
}

func TestPersistPeopleUpsertsAndPreservesExistingImage(t *testing.T) {
	db := newServiceTestDB(t, &model.Person{})
	scraper := &ScraperService{repo: repository.New(db)}

	first := PersonMetadata{
		Name:       "Actor One",
		ImageURL:   "https://images.example.test/actor-one-v1.jpg",
		ProfileURL: "https://metadata.example.test/actor-one",
		Source:     "tmdb",
		SourceID:   "123",
	}
	if err := scraper.persistPeople(t.Context(), []PersonMetadata{first}, nil); err != nil {
		t.Fatal(err)
	}
	if err := scraper.persistPeople(t.Context(), nil, []string{"  Actor   One  "}); err != nil {
		t.Fatal(err)
	}

	var stored model.Person
	if err := db.Where("name_key = ?", "actor one").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ImageURL != first.ImageURL || stored.ProfileURL != first.ProfileURL {
		t.Fatalf("stored metadata was cleared: %#v", stored)
	}

	secondImage := "https://images.example.test/actor-one-v2.jpg"
	if err := scraper.persistPeople(t.Context(), []PersonMetadata{{
		Name:     "Actor One",
		ImageURL: secondImage,
		Source:   "tmdb",
		SourceID: "123",
	}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.Where("name_key = ?", "actor one").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ImageURL != secondImage {
		t.Fatalf("stored image = %q, want %q", stored.ImageURL, secondImage)
	}

	var count int64
	if err := db.Model(&model.Person{}).Where("name_key = ?", "actor one").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("person rows = %d, want 1", count)
	}
}

func TestPipelineResolvedMatchPreservesPeople(t *testing.T) {
	person := PersonMetadata{
		Name:       "Actor One",
		ImageURL:   "https://images.example.test/actor-one.jpg",
		ProfileURL: "https://metadata.example.test/actor-one",
		Source:     "tmdb",
		SourceID:   "123",
	}
	match := pipelineMatchFromExternalResult(ExternalMediaResult{
		Title:  "Movie",
		Actors: []string{"Actor One"},
		People: []PersonMetadata{person},
	})
	if len(match.People) != 1 || match.People[0] != person {
		t.Fatalf("match people = %#v", match.People)
	}
}
