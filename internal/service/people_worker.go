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
				if c.StartupStatus().State != "ready" {
					continue
				}
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
	failQueue := func(err error) {
		task := c.Tasks.Start(TaskKindPeople, "后台人物资料与翻译", TaskUpdate{Stage: "loading", Message: "读取待处理人物"})
		task.Finish(err, TaskUpdate{Stage: "finished", Message: "读取人物待办失败"})
	}
	var people []model.Person
	if err := c.Repo.DB.WithContext(ctx).Where("source = ? AND source_id <> '' AND (details_refreshed_at IS NULL OR translated_name = '')", "tmdb").Order("updated_at, id").Limit(10).Find(&people).Error; err != nil {
		failQueue(err)
		return
	}
	var mediaIDs []string
	if err := c.Repo.DB.WithContext(ctx).Model(&model.PersonCredit{}).Select("media_id").Where("original_role <> '' AND role = original_role").Group("media_id").Order("MIN(updated_at)").Limit(10).Pluck("media_id", &mediaIDs).Error; err != nil {
		failQueue(err)
		return
	}
	if len(people) == 0 && len(mediaIDs) == 0 {
		return
	}
	var task *TaskHandle
	if c.Tasks != nil {
		var started bool
		task, started = c.Tasks.StartUnique(TaskKindPeople, "后台人物资料与翻译", TaskUpdate{Stage: "running", Message: "处理待补全人物和角色"})
		if !started {
			return
		}
	}
	metrics := map[string]int64{"candidates": int64(len(people) + len(mediaIDs))}
	var batchErr error
	defer func() {
		if ctx.Err() != nil {
			task.Cancel(TaskUpdate{Stage: "canceled", Message: "人物后台批次已中断", Metrics: metrics})
			return
		}
		task.Finish(batchErr, TaskUpdate{Stage: "finished", Message: "人物后台批次结束", Metrics: metrics})
	}()
	recordResult := func(err error) {
		metrics["processed"]++
		if err != nil {
			metrics["errors"]++
			if batchErr == nil {
				batchErr = err
			}
		}
		task.Update(TaskUpdate{Metrics: metrics})
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
		recordResult(err)
		if err != nil && c.Log != nil {
			c.Log.Warn("人物资料补全失败", zap.String("person", person.Name), zap.Error(err))
		}
		// Fairness for unknown names and transient failures; every pending person
		// gets a turn instead of the first ten starving the rest of the queue.
		_ = c.Repo.DB.WithContext(ctx).Model(&model.Person{}).Where("id = ?", person.ID).UpdateColumn("updated_at", time.Now()).Error
	}
	for _, id := range mediaIDs {
		if ctx.Err() != nil {
			return
		}
		workCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		count, err := c.TranslateMediaPeople(workCtx, id)
		cancel()
		metrics["translated"] += int64(count)
		recordResult(err)
		if err != nil && c.Log != nil {
			c.Log.Warn("人物角色翻译失败", zap.String("media_id", id), zap.Error(err))
		}
		_ = c.Repo.DB.WithContext(ctx).Model(&model.PersonCredit{}).Where("media_id = ?", id).UpdateColumn("updated_at", time.Now()).Error
	}
}
