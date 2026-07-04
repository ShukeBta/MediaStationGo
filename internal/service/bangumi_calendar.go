// Package service — Bangumi discovery calendar.
package service

import (
	"context"
	"strconv"
	"strings"
)

// Calendar returns Bangumi's public on-air anime calendar as a recommendation
// rail. It needs no token, but NewBangumiProvider still attaches one when set.
func (b *BangumiProvider) Calendar(ctx context.Context) ([]ExternalMediaResult, error) {
	type subject struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		NameCN  string `json:"name_cn"`
		Summary string `json:"summary"`
		AirDate string `json:"air_date"`
		Images  struct {
			Large  string `json:"large"`
			Common string `json:"common"`
		} `json:"images"`
		Rating struct {
			Score float32 `json:"score"`
		} `json:"rating"`
	}
	type day struct {
		Items []subject `json:"items"`
	}
	var days []day
	if err := b.getJSON(ctx, b.base+"/calendar", &days); err != nil {
		return nil, err
	}
	out := make([]ExternalMediaResult, 0, 24)
	seen := map[int]struct{}{}
	for _, day := range days {
		for _, item := range day.Items {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
			title := strings.TrimSpace(item.NameCN)
			if title == "" {
				title = strings.TrimSpace(item.Name)
			}
			if title == "" {
				continue
			}
			poster := item.Images.Large
			if poster == "" {
				poster = item.Images.Common
			}
			poster = normalizeBangumiImageURL(poster)
			year := 0
			if len(item.AirDate) >= 4 {
				year, _ = strconv.Atoi(item.AirDate[:4])
			}
			out = append(out, ExternalMediaResult{
				Source:           "bangumi",
				MediaType:        "anime",
				Title:            title,
				OriginalName:     item.Name,
				Overview:         item.Summary,
				PosterURL:        poster,
				Year:             year,
				Rating:           item.Rating.Score,
				BangumiID:        item.ID,
				SubscribeKeyword: buildSubscribeKeyword(title, year),
				SubscribeAliases: buildSubscribeAliases(title, item.Name, year),
			})
			if len(out) >= 24 {
				return out, nil
			}
		}
	}
	return out, nil
}
