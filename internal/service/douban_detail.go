package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

var doubanISODurationPattern = regexp.MustCompile(`^PT(?:(\d+)H)?(?:(\d+)M)?$`)

type doubanAbstractSubject struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Rate          string   `json:"rate"`
	Subtype       string   `json:"subtype"`
	IsTV          bool     `json:"is_tv"`
	Directors     []string `json:"directors"`
	Actors        []string `json:"actors"`
	Duration      string   `json:"duration"`
	Region        string   `json:"region"`
	Types         []string `json:"types"`
	ReleaseYear   string   `json:"release_year"`
	EpisodesCount string   `json:"episodes_count"`
}

type doubanSchemaPerson struct {
	Name string `json:"name"`
}

type doubanSubjectSchema struct {
	Name            string               `json:"name"`
	Image           string               `json:"image"`
	Description     string               `json:"description"`
	DatePublished   string               `json:"datePublished"`
	Genre           []string             `json:"genre"`
	Duration        string               `json:"duration"`
	Director        []doubanSchemaPerson `json:"director"`
	Author          []doubanSchemaPerson `json:"author"`
	Actor           []doubanSchemaPerson `json:"actor"`
	AggregateRating struct {
		RatingValue string `json:"ratingValue"`
	} `json:"aggregateRating"`
}

type doubanSubjectPage struct {
	Schema    doubanSubjectSchema
	Overview  string
	Countries []string
	Languages []string
	Aliases   []string
}

// GetDiscoverDetailByID loads one subject from Douban's own abstract and page
// representations. The page supplies fields absent from the compact abstract.
func (d *DoubanProvider) GetDiscoverDetailByID(ctx context.Context, doubanID string) (*Match, error) {
	doubanID = strings.TrimSpace(doubanID)
	if doubanID == "" {
		return nil, nil
	}

	abstract, err := d.getDiscoverAbstract(ctx, doubanID)
	if err != nil {
		return nil, err
	}
	page, err := d.getDiscoverSubjectPage(ctx, doubanID)
	if err != nil {
		return nil, err
	}

	mediaType := "movie"
	if abstract.IsTV || strings.EqualFold(abstract.Subtype, "TV") {
		mediaType = "tv"
	}
	title := strings.TrimSpace(page.Schema.Name)
	if title == "" {
		title = stripDoubanTitleYear(abstract.Title)
	}
	rating, _ := strconv.ParseFloat(strings.TrimSpace(abstract.Rate), 32)
	if rating <= 0 {
		rating, _ = strconv.ParseFloat(strings.TrimSpace(page.Schema.AggregateRating.RatingValue), 32)
	}
	year, _ := strconv.Atoi(strings.TrimSpace(abstract.ReleaseYear))
	releaseDate := normalizeReleaseDate(page.Schema.DatePublished)
	if year == 0 {
		year = yearFromDate(releaseDate)
	}
	duration := doubanDurationMinutes(page.Schema.Duration)
	if duration == 0 {
		duration = doubanDurationMinutes(abstract.Duration)
	}
	actors := deduplicate(abstract.Actors)
	if len(actors) == 0 {
		actors = deduplicate(schemaPersonNames(page.Schema.Actor))
	}
	directors := deduplicate(abstract.Directors)
	if len(directors) == 0 {
		directors = deduplicate(schemaPersonNames(page.Schema.Director))
	}
	genres := deduplicate(append(page.Schema.Genre, abstract.Types...))
	countries := deduplicate(append(page.Countries, splitDoubanValues(abstract.Region)...))

	return &Match{
		DoubanID:        doubanID,
		MediaType:       mediaType,
		Title:           title,
		Overview:        strings.TrimSpace(page.Overview),
		PosterURL:       strings.TrimSpace(page.Schema.Image),
		Year:            year,
		ReleaseDate:     releaseDate,
		Rating:          float32(rating),
		DurationMinutes: duration,
		Languages:       deduplicate(page.Languages),
		Countries:       countries,
		Genres:          genres,
		Actors:          actors,
		Directors:       directors,
		Writers:         deduplicate(schemaPersonNames(page.Schema.Author)),
		Aliases:         deduplicate(page.Aliases),
		People:          peopleFromNames(actors),
	}, nil
}

func (d *DoubanProvider) getDiscoverAbstract(ctx context.Context, doubanID string) (doubanAbstractSubject, error) {
	var result struct {
		Code    int                   `json:"r"`
		Subject doubanAbstractSubject `json:"subject"`
	}
	u := "https://movie.douban.com/j/subject_abstract?subject_id=" + url.QueryEscape(doubanID)
	if err := d.getDiscoverJSON(ctx, u, &result); err != nil {
		return doubanAbstractSubject{}, err
	}
	if result.Code != 0 || strings.TrimSpace(result.Subject.ID) == "" {
		return doubanAbstractSubject{}, errors.New("douban detail abstract returned no subject")
	}
	return result.Subject, nil
}

func (d *DoubanProvider) getDiscoverJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	d.setHeaders(req)
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("douban detail: %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(target)
}

func (d *DoubanProvider) getDiscoverSubjectPage(ctx context.Context, doubanID string) (doubanSubjectPage, error) {
	u := "https://movie.douban.com/subject/" + url.PathEscape(doubanID) + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return doubanSubjectPage{}, err
	}
	d.setHeaders(req)
	resp, err := d.client.Do(req)
	if err != nil {
		return doubanSubjectPage{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return doubanSubjectPage{}, fmt.Errorf("douban subject page: %d", resp.StatusCode)
	}
	return parseDoubanSubjectPage(resp.Body)
}

func parseDoubanSubjectPage(reader io.Reader) (doubanSubjectPage, error) {
	doc, err := html.Parse(reader)
	if err != nil {
		return doubanSubjectPage{}, err
	}
	var page doubanSubjectPage
	schemaNode := findHTMLNode(doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && node.Data == "script" && htmlAttribute(node, "type") == "application/ld+json"
	})
	if schemaNode == nil || schemaNode.FirstChild == nil {
		return doubanSubjectPage{}, errors.New("douban subject page has no structured metadata")
	}
	if err := json.Unmarshal([]byte(schemaNode.FirstChild.Data), &page.Schema); err != nil {
		return doubanSubjectPage{}, fmt.Errorf("parse douban subject metadata: %w", err)
	}
	if strings.TrimSpace(page.Schema.Name) == "" {
		return doubanSubjectPage{}, errors.New("douban subject page has no title")
	}

	if summary := findHTMLNode(doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && htmlAttribute(node, "property") == "v:summary"
	}); summary != nil {
		page.Overview = normalizeDoubanText(htmlNodeText(summary))
	}
	if info := findHTMLNode(doc, func(node *html.Node) bool {
		return node.Type == html.ElementNode && htmlAttribute(node, "id") == "info"
	}); info != nil {
		fields := doubanInfoFields(info)
		page.Countries = splitDoubanValues(fields["制片国家/地区"])
		page.Languages = splitDoubanValues(fields["语言"])
		page.Aliases = splitDoubanValues(fields["又名"])
	}
	if page.Overview == "" {
		page.Overview = strings.TrimSpace(page.Schema.Description)
	}
	return page, nil
}

func doubanInfoFields(info *html.Node) map[string]string {
	fields := map[string]string{}
	for child := info.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || child.Data != "span" || !strings.Contains(htmlAttribute(child, "class"), "pl") {
			continue
		}
		label := strings.TrimSuffix(normalizeDoubanText(htmlNodeText(child)), ":")
		if label == "" {
			continue
		}
		var parts []string
		for sibling := child.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			if sibling.Type == html.ElementNode && sibling.Data == "br" {
				break
			}
			if sibling.Type == html.ElementNode && sibling.Data == "span" && strings.Contains(htmlAttribute(sibling, "class"), "pl") {
				break
			}
			parts = append(parts, htmlNodeText(sibling))
		}
		fields[label] = normalizeDoubanText(strings.Join(parts, " "))
	}
	return fields
}

func findHTMLNode(node *html.Node, match func(*html.Node) bool) *html.Node {
	if node == nil {
		return nil
	}
	if match(node) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLNode(child, match); found != nil {
			return found
		}
	}
	return nil
}

func htmlAttribute(node *html.Node, key string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == key {
			return strings.TrimSpace(attribute.Val)
		}
	}
	return ""
}

func htmlNodeText(node *html.Node) string {
	if node == nil {
		return ""
	}
	if node.Type == html.TextNode {
		return node.Data
	}
	var builder strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		builder.WriteString(htmlNodeText(child))
	}
	return builder.String()
}

func normalizeDoubanText(value string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(value, "\u00a0", " ")), " ")
}

func splitDoubanValues(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '／' })
	return nonEmptyStrings(parts...)
}

func schemaPersonNames(people []doubanSchemaPerson) []string {
	names := make([]string, 0, len(people))
	for _, person := range people {
		if name := strings.TrimSpace(person.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
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
	if matches := doubanISODurationPattern.FindStringSubmatch(value); len(matches) == 3 {
		hours, _ := strconv.Atoi(matches[1])
		minutes, _ := strconv.Atoi(matches[2])
		return hours*60 + minutes
	}
	var minutes int
	if _, err := fmt.Sscanf(value, "%d分钟", &minutes); err == nil && minutes > 0 {
		return minutes
	}
	return 0
}

func stripDoubanTitleYear(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.LastIndex(value, " ("); index > 0 && strings.HasSuffix(value, ")") {
		return strings.TrimSpace(value[:index])
	}
	return value
}
