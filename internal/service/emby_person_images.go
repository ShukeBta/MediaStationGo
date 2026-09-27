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
		Where("image_url IS NOT NULL AND image_url <> ''").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	snapshot := make(map[string]model.Person, len(rows))
	for _, person := range rows {
		key := normalizePersonNameKey(person.Name)
		if key == "" || strings.TrimSpace(person.ImageURL) == "" {
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
	return embyImageTag(embyPersonID(person.Name), "primary", person.ImageURL, person.UpdatedAt)
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
		}
		people = append(people, person)
	}
	return people
}

func embyPeopleImageSignature(people []model.EmbyPerson) string {
	parts := make([]string, 0, len(people))
	for _, person := range people {
		if person.PrimaryImageTag != "" {
			parts = append(parts, person.Id+":"+person.PrimaryImageTag)
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}
