// Package service — Douban discovery rails.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Discover returns public Douban movie/TV rails. Douban does not require a
// formal API key here; these are the same public web endpoints the site uses.
func (d *DoubanProvider) Discover(ctx context.Context, key string, pages ...int) ([]ExternalMediaResult, error) {
	pageNumber := 1
	if len(pages) > 0 && pages[0] > 0 {
		pageNumber = pages[0]
	}
	return d.discoverRange(ctx, key, (pageNumber-1)*24, 24)
}

// DiscoverWindow returns one logical Discover page plus one item used to
// determine whether a following page exists. Douban accepts an exact offset
// and limit, so no results are skipped when the UI uses 18-item pages.
func (d *DoubanProvider) DiscoverWindow(ctx context.Context, key string, page, pageSize int) ([]ExternalMediaResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		return []ExternalMediaResult{}, nil
	}
	return d.discoverRange(ctx, key, (page-1)*pageSize, pageSize+1)
}

func (d *DoubanProvider) discoverRange(ctx context.Context, key string, offset, limit int) ([]ExternalMediaResult, error) {
	ctx, cancel := context.WithTimeout(ctx, doubanRequestTimeout)
	defer cancel()
	collection := map[string]string{"douban_hot_movie": "movie_hot_gaia", "douban_hot_tv": "tv_hot"}[key]
	webBudget := doubanRequestTimeout
	if collection != "" {
		webBudget = 8 * time.Second
	}
	webCtx, webCancel := context.WithTimeout(ctx, webBudget)
	items, err := d.discoverWebRange(webCtx, key, offset, limit)
	webCancel()
	if err == nil || ctx.Err() != nil {
		return items, err
	}
	if collection == "" {
		return nil, err
	}
	items, mobileErr := d.discoverCollectionRange(ctx, collection, offset, limit)
	if mobileErr != nil {
		return nil, errors.Join(err, mobileErr)
	}
	return items, nil
}

func (d *DoubanProvider) discoverWebRange(ctx context.Context, key string, offset, limit int) ([]ExternalMediaResult, error) {
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
	q.Set("page_limit", strconv.Itoa(limit))
	q.Set("page_start", strconv.Itoa(offset))
	u := "https://movie.douban.com/j/search_subjects?" + q.Encode()
	raw, status, err := d.requestPublicJSON(ctx, u, "https://movie.douban.com/")
	if status >= 400 {
		return nil, fmt.Errorf("douban discover: %d", status)
	}
	if err != nil {
		return nil, err
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
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, err
	}
	if page.Subjects == nil {
		return nil, errors.New("douban discover: missing subjects in upstream response")
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
			ProviderURL:      subject.URL,
		})
	}
	return out, nil
}
