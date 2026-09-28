package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func personProviderNameKey(person PersonMetadata) string {
	return "provider:" + strings.ToLower(strings.TrimSpace(person.Source)) + ":" + strings.TrimSpace(person.SourceID)
}

func personIdentityQuery(db *gorm.DB, person PersonMetadata) *gorm.DB {
	if person.Source != "" && person.SourceID != "" {
		return db.Where("source = ? AND source_id = ?", person.Source, person.SourceID)
	}
	return db.Where("name_key = ?", normalizePersonNameKey(person.Name))
}

func personProviderConflict(existing model.Person, incoming PersonMetadata) bool {
	return existing.SourceID != "" && incoming.SourceID != "" && (existing.SourceID != incoming.SourceID || existing.Source != incoming.Source)
}

func embyStoredPersonID(person model.Person) string {
	if strings.HasPrefix(person.NameKey, "provider:") {
		return embyPersonID(person.NameKey)
	}
	return embyPersonID(person.Name)
}

func embyCountPersonID(person embyPersonCount) string {
	if person.Person.ID != "" {
		return embyStoredPersonID(person.Person)
	}
	return embyPersonID(person.Name)
}
