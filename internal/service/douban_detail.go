package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type doubanRexxarPerson struct {
	Name string `json:"name"`
}

type doubanRexxarSubject struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	Intro         string `json:"intro"`
	CoverURL      string `json:"cover_url"`
	Pic           struct {
		Large  string `json:"large"`
		Normal string `json:"normal"`
	} `json:"pic"`
	Year        string               `json:"year"`
	Pubdate     []string             `json:"pubdate"`
	Durations   []string             `json:"durations"`
	Genres      []string             `json:"genres"`
	Countries   []string             `json:"countries"`
	Languages   []string             `json:"languages"`
	Directors   []doubanRexxarPerson `json:"directors"`
	Actors      []doubanRexxarPerson `json:"actors"`
	Aliases     []string             `json:"aka"`
	IsTV        bool                 `json:"is_tv"`
	Type        string               `json:"type"`
	ProviderURL string               `json:"url"`
	Rating      struct {
		Value float32 `json:"value"`
	} `json:"rating"`
}

type doubanRexxarCredit struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

type doubanRexxarCredits struct {
	Items []doubanRexxarCredit `json:"items"`
}

// GetDiscoverDetailByID loads one subject from Douban's own mobile JSON API.
// The subject response contains the display metadata, while credits supplies
// explicit writer roles that are not included in the compact subject payload.
func (d *DoubanProvider) GetDiscoverDetailByID(ctx context.Context, doubanID string) (*Match, error) {
	doubanID = strings.TrimSpace(doubanID)
	if doubanID == "" {
		return nil, nil
	}

	subject, err := d.getDiscoverRexxarSubject(ctx, doubanID)
	if err != nil {
		return nil, err
	}
	credits, err := d.getDiscoverRexxarCredits(ctx, doubanID)
	if err != nil {
		return nil, err
	}

	mediaType := "movie"
	if subject.IsTV || strings.EqualFold(strings.TrimSpace(subject.Type), "tv") {
		mediaType = "tv"
	}
	releaseDate := firstDoubanReleaseDate(subject.Pubdate)
	year, _ := strconv.Atoi(strings.TrimSpace(subject.Year))
	if year == 0 {
		year = yearFromDate(releaseDate)
	}

	return &Match{
		DoubanID:        doubanID,
		MediaType:       mediaType,
		Title:           firstText(subject.Title, subject.OriginalTitle),
		OriginalName:    strings.TrimSpace(subject.OriginalTitle),
		Overview:        strings.TrimSpace(subject.Intro),
		PosterURL:       firstText(subject.CoverURL, subject.Pic.Large, subject.Pic.Normal),
		Year:            year,
		ReleaseDate:     releaseDate,
		Rating:          subject.Rating.Value,
		DurationMinutes: firstDoubanDuration(subject.Durations),
		Languages:       deduplicate(nonEmptyStrings(subject.Languages...)),
		Countries:       deduplicate(nonEmptyStrings(subject.Countries...)),
		Genres:          deduplicate(nonEmptyStrings(subject.Genres...)),
		Actors:          deduplicate(doubanRexxarPersonNames(subject.Actors)),
		Directors:       deduplicate(doubanRexxarPersonNames(subject.Directors)),
		Writers:         doubanRexxarWriters(credits.Items),
		Aliases:         deduplicate(nonEmptyStrings(subject.Aliases...)),
		People:          peopleFromNames(doubanRexxarPersonNames(subject.Actors)),
	}, nil
}

func (d *DoubanProvider) getDiscoverRexxarSubject(ctx context.Context, doubanID string) (doubanRexxarSubject, error) {
	var subject doubanRexxarSubject
	endpoint := "https://m.douban.com/rexxar/api/v2/subject/" + url.PathEscape(doubanID)
	if err := d.getDiscoverRexxarJSON(ctx, endpoint, &subject); err != nil {
		return doubanRexxarSubject{}, err
	}
	if strings.TrimSpace(subject.ID) != doubanID || strings.TrimSpace(subject.Title) == "" {
		return doubanRexxarSubject{}, errors.New("douban detail returned no matching subject")
	}
	return subject, nil
}

func (d *DoubanProvider) getDiscoverRexxarCredits(ctx context.Context, doubanID string) (doubanRexxarCredits, error) {
	var credits doubanRexxarCredits
	endpoint := "https://m.douban.com/rexxar/api/v2/subject/" + url.PathEscape(doubanID) + "/credits"
	if err := d.getDiscoverRexxarJSON(ctx, endpoint, &credits); err != nil {
		return doubanRexxarCredits{}, err
	}
	return credits, nil
}

func (d *DoubanProvider) getDiscoverRexxarJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	d.setHeaders(req)
	req.Header.Set("Referer", "https://m.douban.com/")
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("douban detail: %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode douban detail: %w", err)
	}
	return nil
}

func firstDoubanReleaseDate(values []string) string {
	for _, value := range values {
		if normalized := normalizeReleaseDate(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func firstDoubanDuration(values []string) int {
	for _, value := range values {
		if minutes := doubanDurationMinutes(value); minutes > 0 {
			return minutes
		}
	}
	return 0
}

func doubanRexxarPersonNames(people []doubanRexxarPerson) []string {
	names := make([]string, 0, len(people))
	for _, person := range people {
		if name := strings.TrimSpace(person.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func doubanRexxarWriters(credits []doubanRexxarCredit) []string {
	writers := make([]string, 0)
	for _, credit := range credits {
		category := strings.TrimSpace(credit.Category)
		if category != "编剧" && !strings.EqualFold(category, "writer") {
			continue
		}
		if name := strings.TrimSpace(credit.Name); name != "" {
			writers = append(writers, name)
		}
	}
	return deduplicate(writers)
}

func peopleFromNames(names []string) []PersonMetadata {
	people := make([]PersonMetadata, 0, len(names))
	for _, name := range names {
		people = append(people, PersonMetadata{Name: name})
	}
	return people
}

func doubanDurationMinutes(value string) int {
	value = strings.TrimSpace(value)
	var minutes int
	if _, err := fmt.Sscanf(value, "%d分钟", &minutes); err == nil && minutes > 0 {
		return minutes
	}
	return 0
}
