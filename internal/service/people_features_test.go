package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func newPeopleFeatureService(t *testing.T) (*EmbyService, *ScraperService) {
	t.Helper()
	emby := newTestEmbyService(t)
	if err := emby.repo.DB.AutoMigrate(&model.PersonCredit{}, &model.PersonTranslation{}); err != nil {
		t.Fatal(err)
	}
	return emby, &ScraperService{repo: emby.repo, log: zap.NewNop()}
}

func TestPeopleCreditsPreserveTranslationsAndSeparateHomonyms(t *testing.T) {
	emby, scraper := newPeopleFeatureService(t)
	lib := model.Library{Name: "Movies", Path: "/movies", Type: "movie", Enabled: true}
	if err := emby.repo.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 2; index++ {
		media := model.Media{Base: model.Base{ID: fmt.Sprint(index)}, LibraryID: lib.ID, Title: fmt.Sprintf("Movie %d", index), Path: fmt.Sprintf("/movies/%d.mp4", index), Actors: "Alex Smith"}
		if err := emby.repo.DB.Create(&media).Error; err != nil {
			t.Fatal(err)
		}
		match := &Match{Actors: []string{"Alex Smith"}, People: []PersonMetadata{{Name: "Alex Smith", Source: "tmdb", SourceID: fmt.Sprint(index), Type: "Actor", Role: "Doctor", OriginalRole: "Doctor"}}}
		if err := scraper.persistMediaPeople(t.Context(), media.ID, match); err != nil {
			t.Fatal(err)
		}
	}
	var people []model.Person
	if err := emby.repo.DB.Order("source_id").Find(&people).Error; err != nil {
		t.Fatal(err)
	}
	if len(people) != 2 {
		t.Fatalf("homonyms merged: %#v", people)
	}
	if embyStoredPersonID(people[0]) == embyStoredPersonID(people[1]) {
		t.Fatal("homonyms share public identity")
	}
	for index, person := range people {
		works, err := emby.PersonWorks(t.Context(), embyStoredPersonID(person), "", 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		items := works["items"].([]map[string]any)
		if len(items) != 1 || items[0]["id"] != fmt.Sprint(index+1) {
			t.Fatalf("incorrect homonym works: %#v", works)
		}
		profile, err := emby.Item(t.Context(), embyStoredPersonID(person), "")
		if err != nil || profile == nil {
			t.Fatalf("person detail missing: %v %#v", err, profile)
		}
	}
	if err := emby.repo.DB.Model(&model.PersonCredit{}).Where("media_id = ?", "1").Update("role", "医生").Error; err != nil {
		t.Fatal(err)
	}
	match := &Match{Actors: []string{"Alex Smith"}, People: []PersonMetadata{{Name: "Alex Smith", Source: "tmdb", SourceID: "1", Type: "Actor", Role: "Doctor", OriginalRole: "Doctor"}, {Name: "Alex Smith", Source: "tmdb", SourceID: "1", Type: "Director", Role: "导演", OriginalRole: "Director"}}}
	if err := scraper.persistMediaPeople(t.Context(), "1", match); err != nil {
		t.Fatal(err)
	}
	credits := emby.creditsForMedia(t.Context(), model.Media{Base: model.Base{ID: "1"}})
	if len(credits) != 2 || credits[0].Role != "医生" || credits[1].Type != "Director" {
		t.Fatalf("roles lost: %#v", credits)
	}
	match.People = match.People[1:]
	match.Actors = nil
	if err := scraper.persistMediaPeople(t.Context(), "1", match); err != nil {
		t.Fatal(err)
	}
	credits = emby.creditsForMedia(t.Context(), model.Media{Base: model.Base{ID: "1"}})
	if len(credits) != 1 || credits[0].Type != "Director" {
		t.Fatalf("stale credit retained: %#v", credits)
	}
}

func TestPeopleSearchPaginationVisibilityAndCrewWorks(t *testing.T) {
	emby, scraper := newPeopleFeatureService(t)
	visible := model.Library{Base: model.Base{ID: "visible"}, Name: "Visible", Path: "/visible", Type: "movie", Enabled: true}
	hidden := model.Library{Base: model.Base{ID: "hidden"}, Name: "Hidden", Path: "/hidden", Type: "movie", Enabled: true}
	if err := emby.repo.DB.Create(&[]model.Library{visible, hidden}).Error; err != nil {
		t.Fatal(err)
	}
	for index, libraryID := range []string{visible.ID, hidden.ID} {
		media := model.Media{Base: model.Base{ID: fmt.Sprint(index)}, LibraryID: libraryID, Title: "Alex Movie", Path: fmt.Sprintf("/%s/movie.mp4", libraryID), Actors: "Alex Actor"}
		if err := emby.repo.DB.Create(&media).Error; err != nil {
			t.Fatal(err)
		}
		match := &Match{Actors: []string{"Alex Actor"}, People: []PersonMetadata{{Name: "Alex Actor", Type: "Actor"}, {Name: "Alex Director " + libraryID, Type: "Director", Role: "导演"}}}
		if err := scraper.persistMediaPeople(t.Context(), media.ID, match); err != nil {
			t.Fatal(err)
		}
	}
	ctx := emby.PeopleContext(t.Context(), MediaVisibility{IncludeNSFW: true, AllowedLibraryIDs: []string{visible.ID}})
	list, err := emby.Persons(ctx, ItemsParams{SearchTerm: "Alex", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if embyEnvelopeCount(list) != 2 {
		t.Fatalf("hidden people included: %#v", list)
	}
	for page, wantType := range []string{"Person", "Person", "Movie"} {
		result, err := emby.Items(ctx, ItemsParams{SearchTerm: "Alex", IncludeItemTypes: []string{"Person", "Movie"}, StartIndex: page, Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		items := result["Items"].([]map[string]any)
		if len(items) != 1 || items[0]["Type"] != wantType || embyEnvelopeCount(result) != 3 {
			t.Fatalf("page %d: %#v", page, result)
		}
	}
	works, err := emby.PersonWorks(ctx, embyPersonID("Alex Director visible"), "", 0, 50)
	if err != nil || works["total"] != int64(1) {
		t.Fatalf("crew works: %v %#v", err, works)
	}
	profile, err := emby.Item(ctx, embyPersonID("Alex Director hidden"), "")
	if err != nil || profile != nil {
		t.Fatalf("hidden profile: %v %#v", err, profile)
	}
}

func TestTMDbPeopleBiographyAndCrew(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/movie/7":
			_, _ = w.Write([]byte(`{"id":7,"title":"Example","credits":{"cast":[{"id":8,"name":"Actor","original_name":"Original Actor","character":"Doctor","profile_path":"/actor.jpg"}],"crew":[{"id":9,"name":"Director","job":"Director","department":"Directing"},{"id":10,"name":"Writer","job":"Screenplay","department":"Writing"}]}}`))
		case "/person/8":
			_, _ = w.Write([]byte(`{"name":"Original Actor","birthday":"1980-01-02","place_of_birth":"London","known_for_department":"Acting","profile_path":"/actor.jpg","also_known_as":["演员"],"translations":{"translations":[{"iso_639_1":"en","data":{"biography":"An actor."}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey, cfg.Secrets.TMDbAPIProxy = "test", upstream.URL
	provider := NewTMDbProvider(cfg, zap.NewNop(), nil)
	match, err := provider.GetMovieMatch(t.Context(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(match.People) != 3 || len(match.Actors) != 1 || match.People[0].Role != "Doctor" || match.People[1].Role != "导演" || match.People[2].Role != "编剧" {
		t.Fatalf("credits: %#v", match)
	}
	details, err := provider.GetPersonDetails(t.Context(), "8")
	if err != nil {
		t.Fatal(err)
	}
	if details.Overview != "An actor." || details.Birthday != "1980-01-02" || details.Aliases != "演员" || !strings.HasSuffix(details.ImageURL, "/actor.jpg") {
		t.Fatalf("person details: %#v", details)
	}
	if _, err := provider.GetPersonDetails(t.Context(), "8/../9"); err == nil {
		t.Fatal("accepted invalid provider id")
	}
	emby, scraper := newPeopleFeatureService(t)
	lib := model.Library{Name: "Movies", Path: "/movies", Type: "movie", Enabled: true}
	if err := emby.repo.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "Keep this title", Path: "/movies/example.mp4", TMDbID: 7}
	if err := emby.repo.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	container := &Container{Repo: emby.repo, TMDb: provider, Scraper: scraper}
	if err := container.RefreshMediaPeople(t.Context(), media.ID); err != nil {
		t.Fatal(err)
	}
	if err := emby.repo.DB.First(&media, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if media.Title != "Keep this title" || media.Actors != "Actor" {
		t.Fatalf("backfill changed unrelated metadata: %#v", media)
	}
	if credits := emby.creditsForMedia(t.Context(), media); len(credits) != 3 {
		t.Fatalf("backfill credits missing: %#v", credits)
	}
}

func TestPeopleTranslationsBatchCacheAndSeasonContext(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		var inputs []peopleTranslationInput
		if err := json.Unmarshal([]byte(request.Messages[len(request.Messages)-1].Content), &inputs); err != nil {
			t.Error(err)
			return
		}
		translations := map[string]string{}
		for _, input := range inputs {
			translations[input.ID] = "医生"
		}
		body, _ := json.Marshal(translations)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(body)}}}})
	}))
	defer upstream.Close()
	db := newServiceTestDB(t, &model.PersonTranslation{})
	c := &Container{Repo: repository.New(db), AI: NewAIService(&config.Config{AI: config.AIConfig{Enabled: true, APIBase: upstream.URL, APIKey: "test", Model: "test"}}, zap.NewNop(), nil)}
	inputs := []peopleTranslationInput{{Kind: "role", Text: "Doctor", Context: "tv:1 season:1"}, {Kind: "role", Text: "Doctor", Context: "tv:1 season:1"}, {Kind: "role", Text: "Doctor", Context: "tv:1 season:2"}}
	values, err := c.translatePeopleTexts(t.Context(), inputs)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(values) != 2 || translatedPeopleValue(values, inputs[0]) != "医生" {
		t.Fatalf("batch/context incorrect: calls=%d values=%#v", calls, values)
	}
	if _, err := c.translatePeopleTexts(t.Context(), inputs); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("cache not reused: %d calls", calls)
	}
}

func TestPeopleMigrationPreservesLegacyArtwork(t *testing.T) {
	db := newServiceTestDB(t)
	if err := db.Exec(`CREATE TABLE people (id varchar(36) PRIMARY KEY, created_at datetime, updated_at datetime, deleted_at datetime, name varchar(255) NOT NULL, name_key varchar(255) NOT NULL, image_url text, profile_url text, source varchar(32), source_id varchar(128))`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO people (id, name, name_key, image_url, source, source_id) VALUES ('legacy', 'Actor', 'actor', 'https://example.test/actor.jpg', 'tmdb', '7')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Person{}, &model.PersonCredit{}, &model.PersonTranslation{}); err != nil {
		t.Fatal(err)
	}
	var person model.Person
	if err := db.First(&person, "id = ?", "legacy").Error; err != nil {
		t.Fatal(err)
	}
	if person.ImageURL != "https://example.test/actor.jpg" || person.SourceID != "7" || person.NameKey != "actor" {
		t.Fatalf("legacy metadata changed: %#v", person)
	}
}

func TestPeopleProviderRenameRetainsPublicIdentity(t *testing.T) {
	emby, scraper := newPeopleFeatureService(t)
	lib := model.Library{Name: "Movies", Path: "/movies", Type: "movie", Enabled: true}
	if err := emby.repo.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "movie"}, LibraryID: lib.ID, Title: "Movie", Path: "/movies/movie.mp4", Actors: "Old Name"}
	if err := emby.repo.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Old Name", "New Name"} {
		match := &Match{Actors: []string{name}, People: []PersonMetadata{{Name: name, Source: "tmdb", SourceID: "11", Type: "Actor"}}}
		if err := scraper.persistMediaPeople(t.Context(), media.ID, match); err != nil {
			t.Fatal(err)
		}
		if err := emby.repo.DB.Model(&media).Update("actors", name).Error; err != nil {
			t.Fatal(err)
		}
	}
	people, err := emby.Persons(t.Context(), ItemsParams{SearchTerm: "New Name", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	items := people["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != embyPersonID("Old Name") {
		t.Fatalf("rename created duplicate identity: %#v", people)
	}
}
