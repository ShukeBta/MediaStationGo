package service

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func normalizePersonNameKey(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

func normalizePersonRemoteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	return parsed.String()
}

func peopleForPersistence(people []PersonMetadata, actorNames []string) []PersonMetadata {
	merged := append([]PersonMetadata(nil), people...)
	for _, name := range actorNames {
		merged = append(merged, PersonMetadata{Name: name})
	}
	return deduplicatePersonMetadata(merged)
}

func (s *ScraperService) persistMatchPeople(ctx context.Context, match *Match) error {
	if match == nil {
		return nil
	}
	return s.persistPeople(ctx, match.People, match.Actors)
}

func (s *ScraperService) persistPeople(ctx context.Context, people []PersonMetadata, actorNames []string) error {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return errors.New("person metadata store unavailable")
	}
	changed := false
	defer func() {
		if changed {
			personMetadataVersion.Add(1)
		}
	}()
	for _, person := range peopleForPersistence(people, actorNames) {
		name := strings.TrimSpace(person.Name)
		nameKey := normalizePersonNameKey(name)
		if nameKey == "" {
			continue
		}
		imageURL := normalizePersonRemoteURL(person.ImageURL)
		profileURL := normalizePersonRemoteURL(person.ProfileURL)
		db := s.repo.DB.WithContext(ctx)
		var identified model.Person
		if person.Source != "" && person.SourceID != "" {
			err := personIdentityQuery(db.Unscoped(), person).First(&identified).Error
			if err == nil {
				nameKey = identified.NameKey
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		candidate := model.Person{
			Name:         name,
			NameKey:      nameKey,
			ImageURL:     imageURL,
			ProfileURL:   profileURL,
			Source:       strings.TrimSpace(person.Source),
			SourceID:     strings.TrimSpace(person.SourceID),
			OriginalName: strings.TrimSpace(person.OriginalName),
		}
		if err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "name_key"}},
			DoNothing: true,
		}).Create(&candidate).Error; err != nil {
			return err
		}
		var existing model.Person
		if err := db.Unscoped().Where("name_key = ?", nameKey).First(&existing).Error; err != nil {
			return err
		}
		if personProviderConflict(existing, person) {
			candidate.Base = model.Base{}
			candidate.NameKey = personProviderNameKey(person)
			if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name_key"}}, DoNothing: true}).Create(&candidate).Error; err != nil {
				return err
			}
			existing = model.Person{}
			if err := db.Unscoped().Where("name_key = ?", candidate.NameKey).First(&existing).Error; err != nil {
				return err
			}
		}
		// Canonical names also form legacy public IDs; preserve them across
		// provider spelling/localization changes and retain the new spelling as an alias.
		updates := map[string]any{"deleted_at": nil}
		if existing.Name != name {
			updates["aliases"] = strings.Join(deduplicate(append(splitCSV(existing.Aliases), name)), ",")
		}
		if imageURL != "" {
			updates["image_url"] = imageURL
		}
		if profileURL != "" {
			updates["profile_url"] = profileURL
		}
		if source := strings.TrimSpace(person.Source); source != "" {
			updates["source"] = source
		}
		if sourceID := strings.TrimSpace(person.SourceID); sourceID != "" {
			updates["source_id"] = sourceID
		}
		if originalName := strings.TrimSpace(person.OriginalName); originalName != "" {
			updates["original_name"] = originalName
		}
		if err := db.Unscoped().Model(&model.Person{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return err
		}
		changed = true
	}
	return nil
}
