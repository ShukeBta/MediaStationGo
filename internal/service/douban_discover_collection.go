package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// The mobile site's equivalent hot lists keep Douban identities when the
// desktop JSON endpoint is unavailable. Use the same logical page window.
func (d *DoubanProvider) discoverCollectionRange(ctx context.Context, collection string, offset, limit int) ([]ExternalMediaResult, error) {
	query := url.Values{"start": {strconv.Itoa(offset)}, "count": {strconv.Itoa(limit)}}
	referer := "https://m.douban.com/subject_collection/" + collection
	raw, status, err := d.requestPublicJSON(ctx, "https://m.douban.com/rexxar/api/v2/subject_collection/"+collection+"/items?"+query.Encode(), referer)
	if status >= 400 {
		return nil, fmt.Errorf("douban collection: %d", status)
	}
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			Year          string `json:"year"`
			OriginalTitle string `json:"original_title"`
			Cover         struct {
				URL string `json:"url"`
			} `json:"cover"`
			Rating struct {
				Value float32 `json:"value"`
			} `json:"rating"`
		} `json:"subject_collection_items"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	if page.Items == nil {
		return nil, fmt.Errorf("douban collection: missing items in upstream response")
	}
	mediaType := "movie"
	if collection == "tv_hot" {
		mediaType = "tv"
	}
	items := make([]ExternalMediaResult, 0, len(page.Items))
	for _, subject := range page.Items {
		id, title := strings.TrimSpace(subject.ID), strings.TrimSpace(subject.Title)
		if title == "" || id == "" || strings.Trim(id, "0123456789") != "" {
			continue
		}
		year, _ := strconv.Atoi(subject.Year)
		items = append(items, ExternalMediaResult{
			Source: "douban", MediaType: mediaType, DoubanID: id, Title: title,
			OriginalName: subject.OriginalTitle, Year: year, PosterURL: subject.Cover.URL,
			Rating: subject.Rating.Value, ProviderURL: "https://movie.douban.com/subject/" + id + "/",
			SubscribeKeyword: buildSubscribeKeyword(title, year), SubscribeAliases: buildSubscribeAliases(title, subject.OriginalTitle, year),
		})
	}
	return items, nil
}
