package service

import "fmt"

func configureManualEpisodeMapping(req ManualScrapeRequest, options *ScrapeOptions) error {
	if req.EpisodeEndNum == nil && req.EpisodePartNum == nil {
		return nil
	}
	if req.SeasonNum == nil || req.EpisodeNum == nil || *req.SeasonNum < 0 || *req.EpisodeNum < 1 {
		return fmt.Errorf("合并集或分段映射必须同时指定季号和起始集号")
	}
	if req.EpisodeEndNum != nil {
		end := *req.EpisodeEndNum
		if end != 0 && (end < *req.EpisodeNum || end-*req.EpisodeNum > 30) {
			return fmt.Errorf("结束集号必须不小于起始集号，最多覆盖 31 集")
		}
		if end > *req.EpisodeNum {
			options.manualEpisodeEnd = end
		}
	}
	if req.EpisodePartNum != nil {
		if *req.EpisodePartNum < 0 || *req.EpisodePartNum > 99 {
			return fmt.Errorf("分段序号必须在 0 到 99 之间")
		}
		options.manualEpisodePart = *req.EpisodePartNum
	}
	return nil
}
