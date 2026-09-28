package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyPersonsAndPersonFilter(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Adult", Path: `/media/adult`, Type: "adult", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "m1"}, LibraryID: lib.ID, Title: "Movie 1", Path: "/media/adult/1.mp4", Actors: "演员甲,演员乙", NSFW: true},
		{Base: model.Base{ID: "m2"}, LibraryID: lib.ID, Title: "Movie 2", Path: "/media/adult/2.mp4", Actors: "演员甲", NSFW: true},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	persons, err := svc.Persons(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	people := persons["Items"].([]map[string]any)
	if len(people) != 2 {
		t.Fatalf("people = %#v", people)
	}

	personID := embyPersonID("演员甲")
	items, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, PersonIDs: []string{personID}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	media := items["Items"].([]map[string]any)
	if len(media) != 2 {
		t.Fatalf("filtered media = %#v", media)
	}
	peopleOnItem, ok := media[0]["People"].([]model.EmbyPerson)
	if !ok || len(peopleOnItem) == 0 || peopleOnItem[0].Type != "Actor" {
		t.Fatalf("item people = %#v", media[0]["People"])
	}

	person, err := svc.Item(t.Context(), personID, "")
	if err != nil {
		t.Fatal(err)
	}
	if person == nil || person["Name"] != "演员甲" || person["RecursiveItemCount"] != 2 {
		t.Fatalf("person item = %#v", person)
	}
}
