package service

import (
	"context"
	"encoding/base64"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const embyPersonPrefix = "msgo-person-"

func embyPersonID(name string) string {
	return embyPersonPrefix + base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(name)))
}

func embyPersonName(id string) (string, bool) {
	if !strings.HasPrefix(id, embyPersonPrefix) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(id, embyPersonPrefix))
	if err != nil {
		return "", false
	}
	name := strings.TrimSpace(string(raw))
	return name, name != ""
}

type embyPersonCount struct {
	Name   string
	Count  int
	Person model.Person
}

func (e *EmbyService) Persons(ctx context.Context, p ItemsParams) (map[string]any, error) {
	if p.Limit <= 0 || p.Limit > 500 {
		p.Limit = 50
	}
	if p.StartIndex < 0 {
		p.StartIndex = 0
	}
	people, err := e.visiblePersonCounts(ctx, p.UserID, p.ParentID)
	if err != nil {
		return nil, err
	}
	search := strings.ToLower(strings.TrimSpace(p.SearchTerm))
	startsWith := strings.ToLower(strings.TrimSpace(p.NameStartsWith))
	rows := make([]embyPersonCount, 0, len(people))
	for _, person := range people {
		lower := strings.ToLower(person.Name)
		if search != "" && !strings.Contains(lower, search) {
			continue
		}
		if startsWith != "" && !strings.HasPrefix(lower, startsWith) {
			continue
		}
		rows = append(rows, person)
	}
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].Name) < strings.ToLower(rows[j].Name)
	})
	total := len(rows)
	paged := pageSlice(rows, p.StartIndex, p.Limit)
	items := make([]map[string]any, 0, len(paged))
	for _, person := range paged {
		items = append(items, embyPersonPayload(person))
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) personItem(ctx context.Context, userID, name string) (map[string]any, error) {
	people, err := e.visiblePersonCounts(ctx, userID, "")
	if err != nil {
		return nil, err
	}
	person, ok := people[normalizePersonNameKey(name)]
	if !ok {
		return nil, nil
	}
	return embyPersonPayload(person), nil
}

func (e *EmbyService) visiblePersonCounts(ctx context.Context, userID, parentID string) (map[string]embyPersonCount, error) {
	query := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("actors").
		Where("actors IS NOT NULL AND actors <> ''")
	query = e.applyUserMediaVisibility(ctx, query, userID)
	if strings.TrimSpace(parentID) != "" {
		query = query.Where("library_id IN ?", e.mergedLibraryIDs(ctx, parentID))
	}
	var rows []struct {
		Actors string
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	people := make(map[string]embyPersonCount)
	for _, row := range rows {
		for _, name := range splitCSV(row.Actors) {
			key := normalizePersonNameKey(name)
			person := people[key]
			if person.Name == "" {
				person.Name = name
			}
			person.Count++
			people[key] = person
		}
	}
	snapshot, err := e.personMetadataSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	for key, person := range people {
		if stored, ok := snapshot[key]; ok {
			person.Person = stored
			people[key] = person
		}
	}
	return people, nil
}

func embyPersonPayload(person embyPersonCount) map[string]any {
	payload := map[string]any{
		"Id":                 embyPersonID(person.Name),
		"Name":               person.Name,
		"ServerId":           embyServerID,
		"Type":               "Person",
		"IsFolder":           false,
		"RecursiveItemCount": person.Count,
	}
	if tag := embyPersonPrimaryImageTag(person.Person); tag != "" {
		payload["ImageTags"] = map[string]string{"Primary": tag}
	}
	return payload
}
