package service

import "fmt"

// ManualEpisodeMapping identifies one media row inside a batch manual scrape.
// The map key in ManualScrapeRequest.EpisodeMappings is the media ID, which
// lets one show/season request validate heterogeneous filenames without
// repeating TMDB lookups for every episode.
type ManualEpisodeMapping struct {
	SeasonNum      int `json:"season_num"`
	EpisodeNum     int `json:"episode_num"`
	EpisodeEndNum  int `json:"episode_end_num,omitempty"`
	EpisodePartNum int `json:"episode_part_num,omitempty"`
}

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

func validateManualEpisodeMappings(req ManualScrapeRequest, mediaIDs []string) error {
	if len(req.EpisodeMappings) == 0 {
		return nil
	}
	if req.SeasonNum != nil || req.EpisodeNum != nil || req.EpisodeEndNum != nil || req.EpisodePartNum != nil {
		return fmt.Errorf("批量季集映射不能与单条季集字段同时使用")
	}
	selected := make(map[string]struct{}, len(mediaIDs))
	for _, mediaID := range mediaIDs {
		selected[mediaID] = struct{}{}
	}
	if len(req.EpisodeMappings) != len(selected) {
		return fmt.Errorf("批量季集映射必须覆盖每一条媒体")
	}
	for mediaID, mapping := range req.EpisodeMappings {
		if _, ok := selected[mediaID]; !ok {
			return fmt.Errorf("批量季集映射包含未选择的媒体 %s", mediaID)
		}
		if err := validateManualEpisodeOverride(mapping); err != nil {
			return fmt.Errorf("媒体 %s 的季集映射无效: %w", mediaID, err)
		}
	}
	return nil
}

func validateManualEpisodeOverride(mapping ManualEpisodeMapping) error {
	if mapping.SeasonNum < 0 || mapping.EpisodeNum < 1 {
		return fmt.Errorf("季号需大于等于 0，集号需大于 0")
	}
	if mapping.EpisodeEndNum != 0 && (mapping.EpisodeEndNum < mapping.EpisodeNum || mapping.EpisodeEndNum-mapping.EpisodeNum > 30) {
		return fmt.Errorf("结束集号必须不小于起始集号，最多覆盖 31 集")
	}
	if mapping.EpisodePartNum < 0 || mapping.EpisodePartNum > 99 {
		return fmt.Errorf("分段序号必须在 0 到 99 之间")
	}
	return nil
}

func configureManualEpisodeOverride(mapping ManualEpisodeMapping, options *ScrapeOptions) {
	options.manualEpisodeIdentity = &episodeRef{Season: mapping.SeasonNum, Episode: mapping.EpisodeNum}
	options.manualEpisodeEnd = 0
	options.manualEpisodePart = mapping.EpisodePartNum
	if mapping.EpisodeEndNum > mapping.EpisodeNum {
		options.manualEpisodeEnd = mapping.EpisodeEndNum
	}
}
