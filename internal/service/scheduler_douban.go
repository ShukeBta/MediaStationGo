package service

import "context"

func (s *SchedulerService) SetDoubanScraper(scraper *ScraperService) { s.doubanScraper = scraper }
func (s *SchedulerService) jobDoubanEnrichment(ctx context.Context) (err error) {
	if s.doubanScraper == nil {
		return nil
	}
	if s.tasks != nil {
		task := s.tasks.Start(TaskKindScrape, "豆瓣评分与详情补齐", TaskUpdate{Stage: "enrich", Message: "正在补齐已绑定的豆瓣条目"})
		defer func() { task.Finish(err, TaskUpdate{Message: "豆瓣补齐结束"}) }()
	}
	return s.doubanScraper.runDoubanEnrichment(ctx)
}
