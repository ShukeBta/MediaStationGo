// Package service — Douban discovery rails.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Discover returns public Douban movie/TV rails. Douban does not require a
// formal API key here; these are the same public web endpoints the site uses.
func (d *DoubanProvider) Discover(ctx context.Context, key string, pages ...int) ([]ExternalMediaResult, error) {
	doubanType := "movie"
	tag := "热门"
	switch key {
	case "douban_hot_movie":
		doubanType = "movie"
		tag = "热门"
	case "douban_top_movie":
		doubanType = "movie"
		tag = "高分"
	case "douban_hot_tv":
		doubanType = "tv"
		tag = "热门"
	default:
		return []ExternalMediaResult{}, nil
	}
	q := url.Values{}
	q.Set("type", doubanType)
	q.Set("tag", tag)
	q.Set("sort", "recommend")
	q.Set("page_limit", "24")
	pageNumber := 1
	if len(pages) > 0 && pages[0] > 0 {
		pageNumber = pages[0]
	}
	q.Set("page_start", strconv.Itoa((pageNumber-1)*24))
	u := "https://movie.douban.com/j/search_subjects?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	d.setHeaders(req)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("douban discover: %d", resp.StatusCode)
	}
	var page struct {
		Subjects []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Rate  string `json:"rate"`
			Cover string `json:"cover"`
			URL   string `json:"url"`
		} `json:"subjects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, err
	}
	out := make([]ExternalMediaResult, 0, len(page.Subjects))
	mediaType := "movie"
	if doubanType == "tv" {
		mediaType = "tv"
	}
	for _, subject := range page.Subjects {
		if strings.TrimSpace(subject.Title) == "" {
			continue
		}
		rating, _ := strconv.ParseFloat(subject.Rate, 32)
		out = append(out, ExternalMediaResult{
			Source:           "douban",
			MediaType:        mediaType,
			Title:            subject.Title,
			PosterURL:        subject.Cover,
			Rating:           float32(rating),
			DoubanID:         subject.ID,
			SubscribeKeyword: subject.Title,
			SubscribeAliases: buildSubscribeAliases(subject.Title, "", 0),
		})
	}
	return out, nil
}
