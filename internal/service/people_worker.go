package service

import (
	"context"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

// startPeopleWorker resumes unfinished provider profiles and translations from
// database state after restart. Work is opt-in, bounded, and shutdown-aware.
func (c *Container) startPeopleWorker(ctx context.Context) {
	go func() {
		timer := time.NewTicker(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				enabled, err := c.Repo.Setting.Get(ctx, PeopleTranslationSetting)
				if err != nil || enabled != "true" || c.AI == nil || !c.AI.EnabledFor(ctx) {
					continue
				}
				c.runPeopleEnrichment(ctx)
			}
		}
	}()
}

func (c *Container) runPeopleEnrichment(ctx context.Context) {
	var people []model.Person
	if err := c.Repo.DB.WithContext(ctx).Where("source = ? AND source_id <> '' AND (details_refreshed_at IS NULL OR translated_name = '')", "tmdb").Order("updated_at, id").Limit(10).Find(&people).Error; err != nil {
		return
	}
	for _, person := range people {
		if ctx.Err() != nil {
			return
		}
		workCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		var err error
		if person.DetailsRefreshedAt == nil {
			err = c.RefreshPerson(workCtx, embyStoredPersonID(person))
		}
		if err == nil {
			err = c.TranslatePerson(workCtx, embyStoredPersonID(person))
		}
		cancel()
		if err != nil && c.Log != nil {
			c.Log.Warn("人物资料补全失败", zap.String("person", person.Name), zap.Error(err))
		}
		// Fairness for unknown names and transient failures; every pending person
		// gets a turn instead of the first ten starving the rest of the queue.
		_ = c.Repo.DB.WithContext(ctx).Model(&model.Person{}).Where("id = ?", person.ID).UpdateColumn("updated_at", time.Now()).Error
	}
	var mediaIDs []string
	if err := c.Repo.DB.WithContext(ctx).Model(&model.PersonCredit{}).Select("media_id").Where("original_role <> '' AND role = original_role").Group("media_id").Order("MIN(updated_at)").Limit(10).Pluck("media_id", &mediaIDs).Error; err != nil {
		return
	}
	for _, id := range mediaIDs {
		if ctx.Err() != nil {
			return
		}
		workCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		_, err := c.TranslateMediaPeople(workCtx, id)
		cancel()
		if err != nil && c.Log != nil {
			c.Log.Warn("人物角色翻译失败", zap.String("media_id", id), zap.Error(err))
		}
		_ = c.Repo.DB.WithContext(ctx).Model(&model.PersonCredit{}).Where("media_id = ?", id).UpdateColumn("updated_at", time.Now()).Error
	}
}
