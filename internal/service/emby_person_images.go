package service

import (
	"context"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

const embyPersonCacheTTL = 30 * time.Second

var personMetadataVersion atomic.Uint64

func (e *EmbyService) personMetadataSnapshot(ctx context.Context) (map[string]model.Person, error) {
	if e == nil || e.repo == nil || e.repo.DB == nil {
		return map[string]model.Person{}, nil
	}
	now := time.Now()
	version := personMetadataVersion.Load()
	e.personMu.RLock()
	if e.personCache != nil && now.Before(e.personCacheExpires) && e.personCacheVersion == version {
		cached := e.personCache
		e.personMu.RUnlock()
		return cached, nil
	}
	e.personMu.RUnlock()

	e.personMu.Lock()
	defer e.personMu.Unlock()
	version = personMetadataVersion.Load()
	if e.personCache != nil && now.Before(e.personCacheExpires) && e.personCacheVersion == version {
		return e.personCache, nil
	}
	var rows []model.Person
	if err := e.repo.DB.WithContext(ctx).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	snapshot := make(map[string]model.Person, len(rows))
	for _, person := range rows {
		key := person.NameKey
		if key == "" {
			continue
		}
		snapshot[key] = person
	}
	e.personCache = snapshot
	e.personCacheExpires = now.Add(embyPersonCacheTTL)
	e.personCacheVersion = version
	if personMetadataVersion.Load() != version {
		e.personCacheExpires = now
	}
	return snapshot, nil
}

func embyPersonPrimaryImageTag(person model.Person) string {
	if strings.TrimSpace(person.ImageURL) == "" {
		return ""
	}
	return embyImageTag(embyStoredPersonID(person), "primary", person.ImageURL, person.UpdatedAt)
}

func (e *EmbyService) embyPeopleFromCSV(ctx context.Context, value string) []model.EmbyPerson {
	names := splitCSV(value)
	people := make([]model.EmbyPerson, 0, len(names))
	snapshot, err := e.personMetadataSnapshot(ctx)
	if err != nil && e != nil && e.log != nil {
		e.log.Warn("load person image metadata failed", zap.Error(err))
	}
	for _, name := range names {
		person := model.EmbyPerson{
			Id:   embyPersonID(name),
			Name: name,
			Type: "Actor",
		}
		if stored, ok := snapshot[normalizePersonNameKey(name)]; ok {
			person.PrimaryImageTag = embyPersonPrimaryImageTag(stored)
			person.ImageURL = stored.ImageURL
			if stored.TranslatedName != "" {
				person.Name = stored.TranslatedName
			}
		}
		people = append(people, person)
	}
	return people
}

func embyPeopleImageSignature(people []model.EmbyPerson) string {
	parts := make([]string, 0, len(people))
	for _, person := range people {
		parts = append(parts, strings.Join([]string{person.Id, person.Name, person.Type, person.Role, person.PrimaryImageTag}, ":"))
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
