package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func personTextKey(parts ...string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}

// persistMediaPeople updates the credit set only after a successful metadata
// commit. The transaction prevents readers observing half a replacement set.
func (s *ScraperService) persistMediaPeople(ctx context.Context, mediaID string, match *Match) error {
	if match == nil {
		return nil
	}
	if err := s.persistMatchPeople(ctx, match); err != nil {
		return err
	}
	if !s.repo.DB.Migrator().HasTable(&model.PersonCredit{}) {
		return nil
	}
	people := append([]PersonMetadata(nil), match.People...)
	known := map[string]bool{}
	for _, person := range people {
		known[normalizePersonNameKey(person.Name)] = true
	}
	for _, name := range match.Actors {
		if !known[normalizePersonNameKey(name)] {
			people = append(people, PersonMetadata{Name: name, Type: "Actor"})
		}
	}
	err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		keys := make([]string, 0, len(people))
		for index, person := range people {
			var stored model.Person
			if strings.TrimSpace(person.Name) == "" {
				continue
			}
			if err := personIdentityQuery(tx, person).First(&stored).Error; err != nil {
				return err
			}
			typ := person.Type
			if typ == "" {
				typ = "Actor"
			}
			originalRole := person.OriginalRole
			if originalRole == "" {
				originalRole = person.Role
			}
			key := personTextKey(mediaID, stored.ID, typ, originalRole)
			keys = append(keys, key)
			credit := model.PersonCredit{CreditKey: key, MediaID: mediaID, PersonID: stored.ID, Type: typ, OriginalRole: originalRole, Role: localizedCrewRole(typ, person.Role), SortOrder: index}
			// Existing translations survive provider refresh when the source role is unchanged.
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "credit_key"}}, DoUpdates: clause.Assignments(map[string]any{"sort_order": index, "deleted_at": nil})}).Create(&credit).Error; err != nil {
				return err
			}
		}
		stale := tx.Unscoped().Where("media_id = ?", mediaID)
		if len(keys) > 0 {
			stale = stale.Where("credit_key NOT IN ?", keys)
		}
		return stale.Delete(&model.PersonCredit{}).Error
	})
	if err == nil {
		personMetadataVersion.Add(1)
	}
	return err
}

type personCreditsContextKey struct{}

func (e *EmbyService) withPersonCredits(ctx context.Context, media []model.Media) context.Context {
	snapshot := make(map[string][]model.PersonCredit, len(media))
	if e.repo.DB.Migrator().HasTable(&model.PersonCredit{}) {
		for start := 0; start < len(media); start += 100 {
			ids := make([]string, 0, 100)
			for _, row := range media[start:min(start+100, len(media))] {
				ids = append(ids, row.ID)
			}
			var credits []model.PersonCredit
			if err := e.repo.DB.WithContext(ctx).Where("media_id IN ?", ids).Order("sort_order, id").Preload("Person").Find(&credits).Error; err != nil {
				return ctx
			}
			for _, credit := range credits {
				snapshot[credit.MediaID] = append(snapshot[credit.MediaID], credit)
			}
		}
	}
	return context.WithValue(ctx, personCreditsContextKey{}, snapshot)
}

func (e *EmbyService) creditsForMedia(ctx context.Context, media model.Media) []model.PersonCredit {
	if snapshot, ok := ctx.Value(personCreditsContextKey{}).(map[string][]model.PersonCredit); ok {
		return snapshot[media.ID]
	}
	var rows []model.PersonCredit
	if e.repo.DB.Migrator().HasTable(&model.PersonCredit{}) {
		_ = e.repo.DB.WithContext(ctx).Where("media_id = ?", media.ID).Order("sort_order, id").Preload("Person").Find(&rows).Error
	}
	return rows
}

func (e *EmbyService) embyPeopleForMedia(ctx context.Context, media *model.Media) []model.EmbyPerson {
	credits := e.creditsForMedia(ctx, *media)
	if len(credits) == 0 {
		return e.embyPeopleFromCSV(ctx, media.Actors)
	}
	people := make([]model.EmbyPerson, 0, len(credits))
	for _, credit := range credits {
		if credit.Person.ID == "" {
			continue
		}
		name := credit.Person.Name
		if credit.Person.TranslatedName != "" {
			name = credit.Person.TranslatedName
		}
		people = append(people, model.EmbyPerson{Id: embyStoredPersonID(credit.Person), Name: name, ImageURL: credit.Person.ImageURL, Type: credit.Type, Role: localizedCrewRole(credit.Type, credit.Role), PrimaryImageTag: embyPersonPrimaryImageTag(credit.Person)})
	}
	return people
}

func (e *EmbyService) filterMediaRowsByPeople(ctx context.Context, rows []model.Media, ids []string) ([]model.Media, error) {
	if len(ids) == 0 {
		return rows, nil
	}
	matched := map[string]bool{}
	for start := 0; start < len(rows); start += 500 {
		mediaIDs := make([]string, 0, 500)
		for _, row := range rows[start:min(start+500, len(rows))] {
			mediaIDs = append(mediaIDs, row.ID)
		}
		var found []string
		q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("media.id IN ?", mediaIDs)
		if err := applyEmbyPersonFilter(q, ids).Pluck("media.id", &found).Error; err != nil {
			return nil, err
		}
		for _, id := range found {
			matched[id] = true
		}
	}
	filtered := rows[:0]
	for _, row := range rows {
		if matched[row.ID] {
			filtered = append(filtered, row)
		}
	}
	return filtered, nil
}

func (e *EmbyService) seriesPayloadWithPeople(ctx context.Context, group embySeriesGroup) map[string]any {
	item := e.seriesPayload(group)
	if len(group.Episodes) > 0 {
		item["People"] = e.embyPeopleForMedia(ctx, &group.Episodes[0])
	}
	return item
}

func (e *EmbyService) seasonPayloadWithPeople(ctx context.Context, group embySeasonGroup) map[string]any {
	item := e.seasonPayload(group)
	if len(group.Episodes) > 0 {
		item["People"] = e.embyPeopleForMedia(ctx, &group.Episodes[0])
	}
	return item
}
