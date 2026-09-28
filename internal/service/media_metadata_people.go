package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Explicit actor edits replace cast membership while preserving the provider
// identity and translated roles of retained actors, as well as all crew credits.
func syncEditedMediaActors(ctx context.Context, tx *gorm.DB, mediaID, actors string) error {
	if !tx.Migrator().HasTable(&model.PersonCredit{}) {
		return nil
	}
	names := splitCSV(actors)
	wanted := make(map[string]int, len(names))
	for index, name := range names {
		wanted[normalizePersonNameKey(name)] = index
	}
	var credits []model.PersonCredit
	if err := tx.WithContext(ctx).Where("media_id = ? AND type = ?", mediaID, "Actor").Preload("Person").Find(&credits).Error; err != nil {
		return err
	}
	retained := map[int]bool{}
	keep := make([]string, 0, len(credits))
	for _, credit := range credits {
		person := credit.Person
		aliases := append([]string{person.Name, person.OriginalName, person.TranslatedName}, splitCSV(person.Aliases)...)
		for _, name := range aliases {
			index, ok := wanted[normalizePersonNameKey(name)]
			if !ok {
				continue
			}
			keep = append(keep, credit.ID)
			retained[index] = true
			if err := tx.Model(&credit).Update("sort_order", index).Error; err != nil {
				return err
			}
			break
		}
	}
	stale := tx.Unscoped().Where("media_id = ? AND type = ?", mediaID, "Actor")
	if len(keep) > 0 {
		stale = stale.Where("id NOT IN ?", keep)
	}
	if err := stale.Delete(&model.PersonCredit{}).Error; err != nil {
		return err
	}
	for index, name := range names {
		if retained[index] {
			continue
		}
		name = strings.TrimSpace(name)
		person := model.Person{Name: name, NameKey: normalizePersonNameKey(name)}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "name_key"}}, DoNothing: true}).Create(&person).Error; err != nil {
			return err
		}
		var stored model.Person
		if err := tx.Unscoped().Where("name_key = ?", person.NameKey).First(&stored).Error; err != nil {
			return err
		}
		if stored.DeletedAt.Valid {
			if err := tx.Unscoped().Model(&stored).Update("deleted_at", nil).Error; err != nil {
				return err
			}
		}
		credit := model.PersonCredit{CreditKey: personTextKey(mediaID, stored.ID, "Actor", ""), MediaID: mediaID, PersonID: stored.ID, Type: "Actor", SortOrder: index}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "credit_key"}}, DoUpdates: clause.Assignments(map[string]any{"deleted_at": nil, "sort_order": index})}).Create(&credit).Error; err != nil {
			return err
		}
	}
	return nil
}
