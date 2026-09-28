package service

import (
	"reflect"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyLibraryDisplay(t *testing.T) {
	libs := make([]model.Library, 3)
	for i, id := range []string{"a", "b", "new"} {
		libs[i].ID = id
	}
	for _, tt := range []struct {
		name, value string
		want        []string
	}{
		{"default", "", []string{"a", "b", "new"}},
		{"empty", "[]", []string{"a", "b", "new"}},
		{"ordered", `[{"id":"b","hidden":false},{"id":"a","hidden":false}]`, []string{"b", "a", "new"}},
		{"hidden and deleted", `[{"id":"deleted","hidden":false},{"id":"a","hidden":true},{"id":"b","hidden":false}]`, []string{"b", "new"}},
		{"all hidden", `[{"id":"a","hidden":true},{"id":"b","hidden":true},{"id":"new","hidden":true}]`, []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := DecodeEmbyLibraryDisplay(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0)
			for _, lib := range applyEmbyLibraryDisplay(libs, entries) {
				got = append(got, lib.ID)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			if libs[0].ID != "a" {
				t.Fatal("changed source library order")
			}
		})
	}
	for _, value := range []string{`null`, `{}`, `[null]`, `[{"id":"a"}]`, `[{"id":"a","hidden":null}]`, `[{"id":"","hidden":false}]`, `[{"id":"a","hidden":"false"}]`, `[{"id":"a","hidden":false},{"id":"a","hidden":true}]`} {
		if _, err := DecodeEmbyLibraryDisplay(value); err == nil {
			t.Fatalf("accepted invalid input %s", value)
		}
	}
}

func TestEmbyAdditionalPartsOrderAndLibraryIsolation(t *testing.T) {
	e := newTestEmbyService(t)
	rows := []model.Media{
		{Base: model.Base{ID: "p1"}, LibraryID: "lib", PartGroupKey: "group", PartIndex: 1},
		{Base: model.Base{ID: "p3"}, LibraryID: "lib", PartGroupKey: "group", PartIndex: 3},
		{Base: model.Base{ID: "p2"}, LibraryID: "lib", PartGroupKey: "group", PartIndex: 2},
		{Base: model.Base{ID: "other"}, LibraryID: "other", PartGroupKey: "group", PartIndex: 4},
	}
	for i := range rows {
		rows[i].Title = rows[i].ID
		rows[i].Path = "/media/" + rows[i].ID + ".mkv"
	}
	if err := e.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	got, err := e.AdditionalParts(t.Context(), "p1", "")
	if err != nil {
		t.Fatal(err)
	}
	items := got["Items"].([]map[string]any)
	if len(items) != 2 || items[0]["Id"] != "p2" || items[1]["Id"] != "p3" {
		t.Fatalf("parts=%+v", got)
	}
	if _, ok := items[0]["PartCount"]; ok {
		t.Fatal("additional part must not recurse")
	}
	got, err = e.AdditionalParts(t.Context(), "p3", "")
	if err != nil {
		t.Fatal(err)
	}
	if got["TotalRecordCount"] != 0 {
		t.Fatalf("last part=%+v", got)
	}
}

func TestEmbyLibraryDisplayHideMergedEntranceKeepsCloudShadowed(t *testing.T) {
	e := newTestEmbyService(t)
	local := model.Library{Base: model.Base{ID: "local"}, Name: "日番", Type: "tv", Path: "/media/动漫/日番", Enabled: true}
	cloud := model.Library{Base: model.Base{ID: "cloud"}, Name: "OpenList · 日漫", Type: "anime", Path: BuildCloudLibraryPath("openlist", "/日漫", "/日漫"), Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := e.repo.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	before, err := e.DisplayLibraries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != 1 || before[0].ID != "local" {
		t.Fatalf("original entrance=%+v", before)
	}
	if err := e.repo.Setting.Set(t.Context(), EmbyLibraryDisplaySettingKey, `[{"id":"local","hidden":true}]`); err != nil {
		t.Fatal(err)
	}
	after, err := e.DisplayLibraries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("hidden merged entrance leaked shadow library: %+v", after)
	}
}
