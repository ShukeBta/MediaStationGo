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
	if len(p.Filters) > 0 {
		return emptyItemsEnvelope(p.StartIndex), nil
	}
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
		lower := strings.ToLower(strings.Join([]string{person.Name, person.Person.OriginalName, person.Person.TranslatedName, person.Person.Aliases}, " "))
		if len(p.IDs) > 0 && !personIDIncluded(p.IDs, embyCountPersonID(person)) {
			continue
		}
		if search != "" && !strings.Contains(lower, search) {
			continue
		}
		if startsWith != "" && !strings.HasPrefix(lower, startsWith) {
			continue
		}
		rows = append(rows, person)
	}
	sort.Slice(rows, func(i, j int) bool {
		if strings.EqualFold(rows[i].Name, rows[j].Name) {
			return embyCountPersonID(rows[i]) < embyCountPersonID(rows[j])
		}
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
		Select("id, actors").
		Where("actors IS NOT NULL AND actors <> ''")
	query = e.applyUserMediaVisibility(ctx, query, userID)
	if strings.TrimSpace(parentID) != "" {
		query = query.Where("library_id IN ?", e.mergedLibraryIDs(ctx, parentID))
	}
	var rows []struct {
		ID     string
		Actors string
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	people := make(map[string]embyPersonCount)
	seen := make(map[string]bool)
	creditedNames := make(map[string]bool)
	if e.repo.DB.Migrator().HasTable(&model.PersonCredit{}) {
		visible := e.applyUserMediaVisibility(ctx, e.repo.DB.Model(&model.Media{}).Select("media.id"), userID)
		if strings.TrimSpace(parentID) != "" {
			visible = visible.Where("library_id IN ?", e.mergedLibraryIDs(ctx, parentID))
		}
		var credits []model.PersonCredit
		if err := e.repo.DB.WithContext(ctx).Where("media_id IN (?)", visible).Preload("Person").Find(&credits).Error; err != nil {
			return nil, err
		}
		for _, credit := range credits {
			if credit.Person.ID == "" {
				continue
			}
			key := credit.Person.NameKey
			if seen[credit.MediaID+"\x00"+key] {
				continue
			}
			aliases := append([]string{credit.Person.Name, credit.Person.OriginalName, credit.Person.TranslatedName}, splitCSV(credit.Person.Aliases)...)
			for _, alias := range aliases {
				creditedNames[credit.MediaID+"\x00"+normalizePersonNameKey(alias)] = true
			}
			person := people[key]
			person.Name, person.Person = credit.Person.Name, credit.Person
			person.Count++
			people[key], seen[credit.MediaID+"\x00"+key] = person, true
		}
	}
	for _, row := range rows {
		for _, name := range splitCSV(row.Actors) {
			key := normalizePersonNameKey(name)
			if creditedNames[row.ID+"\x00"+key] {
				continue
			}
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
	name := person.Name
	if person.Person.TranslatedName != "" {
		name = person.Person.TranslatedName
	}
	payload := map[string]any{
		"Id":                  embyCountPersonID(person),
		"Name":                name,
		"OriginalTitle":       person.Person.OriginalName,
		"Overview":            person.Person.Overview,
		"PremiereDate":        person.Person.Birthday,
		"EndDate":             person.Person.Deathday,
		"ProductionLocations": []string{person.Person.Birthplace},
		"ImageURL":            person.Person.ImageURL,
		"Department":          localizedPersonDepartment(person.Person.Department),
		"Aliases":             splitCSV(person.Person.Aliases),
		"ProviderIds":         map[string]string{person.Person.Source: person.Person.SourceID},
		"ServerId":            embyServerID,
		"Type":                "Person",
		"IsFolder":            false,
		"RecursiveItemCount":  person.Count,
	}
	if tag := embyPersonPrimaryImageTag(person.Person); tag != "" {
		payload["ImageTags"] = map[string]string{"Primary": tag}
	}
	return payload
}

func personIDIncluded(ids []string, id string) bool {
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}
