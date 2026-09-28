package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/helper"
)

const (
	adultFD2PPVDefaultSort    = "release"
	adultFD2PPVSourcePageSize = 48
)

var (
	adultFD2CardStartPattern     = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bartist-card\b[^"']*["'][^>]*>`)
	adultFD2ListPhotosPattern    = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bwork-photos\b[^"']*["'][^>]*>(.*?)</div>`)
	adultFD2ListContentPattern   = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bartist-content\b[^"']*["'][^>]*>\s*(?:<a\b[^>]*>)?(.*?)(?:</a>)?\s*</div>`)
	adultFD2ListReleasePattern   = regexp.MustCompile(`(?is)<div\b[^>]*class=["'][^"']*\bartist-release\b[^"']*["'][^>]*>.*?<span\b[^>]*>(.*?)</span>`)
	adultFD2ArticleLinkPattern   = regexp.MustCompile(`(?is)<a\b([^>]*)href=["']([^"']*?/articles/(\d{5,10})(?:[/?#][^"']*)?)["'][^>]*>(.*?)</a>`)
	adultFD2PerformerLinkPattern = regexp.MustCompile(`(?is)<a\b([^>]*)href=["']([^"']*?/actresses/(\d+)(?:[/?#][^"']*)?)["'][^>]*>(.*?)</a>`)
	adultFD2AliasPattern         = regexp.MustCompile(`(?is)<span\b[^>]*class=["'][^"']*\balias-item\b[^"']*["'][^>]*>(.*?)</span>`)
	adultFD2ProfileNamePattern   = regexp.MustCompile(`(?is)<h1\b[^>]*class=["'][^"']*\bartist-detail-name\b[^"']*["'][^>]*>(.*?)</h1>`)
	adultFD2AvatarStylePattern   = regexp.MustCompile(`(?is)background-image\s*:\s*url\(\s*["']?([^"')]+)`)
	adultFD2PaginationPattern    = regexp.MustCompile(`(?is)<nav\b([^>]*)>`)
	adultFD2RawNumberPattern     = regexp.MustCompile(`^\d{5,8}$`)
)

func NormalizeFD2PPVDiscoverSort(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return adultFD2PPVDefaultSort, true
	}
	switch value {
	case "release", "views", "likes", "favorites", "comments":
		return value, true
	default:
		return "", false
	}
}

func (p *AdultProvider) FD2PPVEnabled() bool {
	return p != nil && strings.TrimSpace(p.flareSolverrURL) != ""
}

func (p *AdultProvider) DiscoverFD2PPVWindow(
	ctx context.Context,
	sortKey string,
	page int,
	pageSize int,
) ([]ExternalMediaResult, error) {
	if !p.FD2PPVEnabled() {
		return nil, errors.New("adult source fd2ppv requires FlareSolverr")
	}
	sortKey, ok := NormalizeFD2PPVDiscoverSort(sortKey)
	if !ok {
		return nil, fmt.Errorf("unsupported FD2PPV sort: %s", sortKey)
	}
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		return []ExternalMediaResult{}, nil
	}

	windowStart := (page - 1) * pageSize
	targetSize := windowStart + pageSize + 1
	out := make([]ExternalMediaResult, 0, targetSize)
	seen := make(map[string]struct{}, targetSize)
	for sourcePage := 1; len(out) < targetSize; sourcePage++ {
		targetURL := strings.TrimRight(fd2PPVBaseURL, "/") + "/articles/?sort=" +
			url.QueryEscape(sortKey) + "&size=" + strconv.Itoa(adultFD2PPVSourcePageSize) +
			"&page=" + strconv.Itoa(sourcePage)
		body, err := p.fetchFD2PPVText(ctx, targetURL)
		if err != nil {
			return nil, err
		}
		chunk := parseFD2PPVMovieList(body, fd2PPVBaseURL)
		if len(chunk) == 0 && len(out) == 0 {
			return nil, errors.New("fd2ppv returned no usable discovery items")
		}
		for _, item := range chunk {
			identity := strings.TrimSpace(item.ProviderID)
			if identity == "" {
				identity = strings.TrimSpace(item.ProviderURL)
			}
			if _, exists := seen[identity]; exists {
				continue
			}
			seen[identity] = struct{}{}
			out = append(out, item)
			if len(out) >= targetSize {
				break
			}
		}
		if !adultFD2PageHasNext(body, sourcePage) {
			break
		}
	}
	if windowStart >= len(out) {
		return []ExternalMediaResult{}, nil
	}
	windowEnd := windowStart + pageSize + 1
	if windowEnd > len(out) {
		windowEnd = len(out)
	}
	return out[windowStart:windowEnd], nil
}

func (p *AdultProvider) SearchFD2PPVMovies(ctx context.Context, query string) ([]ExternalMediaResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("fd2ppv movie query is empty")
	}
	if !p.FD2PPVEnabled() {
		return nil, errors.New("adult source fd2ppv requires FlareSolverr")
	}
	if number := adultFD2SearchNumber(query); number != "" {
		item, err := p.DiscoverMovieDetail(ctx, "fd2ppv", number, "FC2-PPV-"+number)
		if err != nil {
			return nil, err
		}
		return []ExternalMediaResult{item}, nil
	}

	targetURL := strings.TrimRight(fd2PPVBaseURL, "/") + "/articles/?keyword=" + url.QueryEscape(query) + "&size=24"
	body, err := p.fetchFD2PPVText(ctx, targetURL)
	if err != nil {
		return nil, err
	}
	items := filterAdultMovieSearchItems(parseFD2PPVMovieList(body, fd2PPVBaseURL), query)
	if len(items) == 0 {
		if item, ok := parseFD2PPVDetailResult(body, fd2PPVBaseURL); ok && fd2PPVMovieMatches(item, query) {
			items = append(items, item)
		}
	}
	return limitAdultDiscoveryItems(items, 20), nil
}

func (p *AdultProvider) SearchFD2PPVPerformers(ctx context.Context, query string) ([]ExternalMediaResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("fd2ppv performer query is empty")
	}
	if !p.FD2PPVEnabled() {
		return nil, errors.New("adult source fd2ppv requires FlareSolverr")
	}
	targetURL := strings.TrimRight(fd2PPVBaseURL, "/") + "/actresses/?keyword=" + url.QueryEscape(query) + "&size=24"
	body, err := p.fetchFD2PPVText(ctx, targetURL)
	if err != nil {
		return nil, err
	}
	items := parseFD2PPVPerformerList(body, fd2PPVBaseURL, query)
	if len(items) == 0 {
		if item, ok := parseFD2PPVPerformerProfile(body, fd2PPVBaseURL); ok && adultPerformerQueryMatches(item.Title, query) {
			items = append(items, item)
		}
	}
	return limitAdultDiscoveryItems(items, 30), nil
}

func (p *AdultProvider) fetchFD2PPVText(ctx context.Context, targetURL string) (string, error) {
	if !p.FD2PPVEnabled() {
		return "", errors.New("adult source fd2ppv requires FlareSolverr")
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	if p.apiConfig != nil {
		return p.fetchAuthenticatedFD2PPVText(ctx, targetURL)
	}
	solution, err := p.fetchWithFlareSolverrResultContext(
		ctx,
		targetURL,
		nil,
	)
	if err != nil {
		return "", err
	}
	if helper.IsCloudflareChallenge(solution.Response) {
		return "", errors.New("fd2ppv Cloudflare challenge was not solved")
	}
	return solution.Response, nil
}

func parseFD2PPVMovieList(body, base string) []ExternalMediaResult {
	out := make([]ExternalMediaResult, 0, adultFD2PPVSourcePageSize)
	seen := map[string]struct{}{}
	for _, block := range adultFD2CardBlocks(body) {
		link := adultFD2ArticleLinkPattern.FindStringSubmatch(block)
		if len(link) < 5 {
			continue
		}
		number := strings.TrimSpace(link[3])
		if !adultFD2RawNumberPattern.MatchString(number) {
			continue
		}
		if _, exists := seen[number]; exists {
			continue
		}
		seen[number] = struct{}{}
		code := "FC2-PPV-" + number
		title := adultFD2ListText(block, adultFD2ListContentPattern)
		photos := adultFD2ListPhotoURLs(block)
		detailURL := absolutizeURL(base, link[2])
		item := ExternalMediaResult{
			Source:           "fd2ppv",
			MediaType:        "adult",
			Title:            firstNonEmpty(title, code),
			OriginalName:     code,
			ReleaseDate:      adultFD2ListText(block, adultFD2ListReleasePattern),
			SubscribeKeyword: code,
			SubscribeAliases: compactUniqueStrings(code, title),
			Genres:           []string{"Adult", "fd2ppv"},
			NSFW:             true,
			ProviderURL:      detailURL,
			ProviderID:       number,
		}
		if len(photos) > 0 {
			item.PosterURL = photos[0]
			item.BackdropURL = photos[0]
		}
		if len(item.ReleaseDate) >= 4 {
			item.Year, _ = strconv.Atoi(item.ReleaseDate[:4])
		}
		out = append(out, item)
	}
	return out
}

func parseFD2PPVPerformerList(body, base, query string) []ExternalMediaResult {
	out := make([]ExternalMediaResult, 0, 30)
	seen := map[string]struct{}{}
	for _, block := range adultFD2CardBlocks(body) {
		link := adultFD2PerformerLinkPattern.FindStringSubmatch(block)
		if len(link) < 5 {
			continue
		}
		sourceID := strings.TrimSpace(link[3])
		if !adultPerformerIDPattern.MatchString(sourceID) {
			continue
		}
		name := strings.TrimSpace(stripAdultHTML(link[4]))
		if !validAdultActorName(name) || !fd2PPVPerformerCardMatches(block, name, query) {
			continue
		}
		if _, exists := seen[sourceID]; exists {
			continue
		}
		seen[sourceID] = struct{}{}
		profileURL := absolutizeURL(base, link[2])
		imageURL := adultFD2CardImageURL(block, base)
		person := PersonMetadata{
			Name:       name,
			ImageURL:   imageURL,
			ProfileURL: profileURL,
			Source:     "fd2ppv",
			SourceID:   sourceID,
		}
		out = append(out, adultPerformerItem(person))
	}
	return out
}

func parseFD2PPVPerformerProfile(body, base string) (ExternalMediaResult, bool) {
	nameMatch := adultFD2ProfileNamePattern.FindStringSubmatch(body)
	if len(nameMatch) < 2 {
		return ExternalMediaResult{}, false
	}
	name := strings.TrimSpace(stripAdultHTML(nameMatch[1]))
	if !validAdultActorName(name) {
		return ExternalMediaResult{}, false
	}
	link := adultFD2PerformerLinkPattern.FindStringSubmatch(body)
	if len(link) < 4 || !adultPerformerIDPattern.MatchString(strings.TrimSpace(link[3])) {
		return ExternalMediaResult{}, false
	}
	profileURL := absolutizeURL(base, link[2])
	person := PersonMetadata{
		Name:       name,
		ImageURL:   adultFD2CardImageURL(body, base),
		ProfileURL: profileURL,
		Source:     "fd2ppv",
		SourceID:   strings.TrimSpace(link[3]),
	}
	return adultPerformerItem(person), true
}

func parseFD2PPVDetailResult(body, base string) (ExternalMediaResult, bool) {
	workTitle := adultFD2TextMatch(body, adultFD2WorkTitlePattern)
	number := adultFD2NumberPattern.FindString(workTitle)
	if number == "" {
		return ExternalMediaResult{}, false
	}
	detailURL := strings.TrimRight(base, "/") + "/articles/" + number
	match := parseFD2PPVDetailHTML(body, "FC2-PPV-"+number, detailURL)
	if match == nil {
		return ExternalMediaResult{}, false
	}
	return externalAdultMovieResult("fd2ppv", number, detailURL, match), true
}

func adultFD2CardBlocks(body string) []string {
	indices := adultFD2CardStartPattern.FindAllStringIndex(body, -1)
	blocks := make([]string, 0, len(indices))
	for index, current := range indices {
		end := len(body)
		if index+1 < len(indices) {
			end = indices[index+1][0]
		}
		blocks = append(blocks, body[current[0]:end])
	}
	return blocks
}

func adultFD2ListText(block string, pattern *regexp.Regexp) string {
	match := pattern.FindStringSubmatch(block)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(stripAdultHTML(match[1]))
}

func adultFD2ListPhotoURLs(block string) []string {
	match := adultFD2ListPhotosPattern.FindStringSubmatch(block)
	if len(match) < 2 {
		return nil
	}
	return adultFD2PhotoURLs(stripAdultHTML(match[1]))
}

func adultFD2CardImageURL(block, base string) string {
	if image := firstAdultListImage(block); image != "" {
		return absolutizeURL(base, image)
	}
	if match := adultFD2AvatarStylePattern.FindStringSubmatch(block); len(match) > 1 {
		return absolutizeURL(base, strings.TrimSpace(match[1]))
	}
	return ""
}

func fd2PPVPerformerCardMatches(block, name, query string) bool {
	if adultPerformerQueryMatches(name, query) {
		return true
	}
	queryKey := normalizeAdultMovieSearchText(query)
	if queryKey == "" {
		return false
	}
	for _, match := range adultFD2AliasPattern.FindAllStringSubmatch(block, -1) {
		if len(match) > 1 && strings.Contains(normalizeAdultMovieSearchText(stripAdultHTML(match[1])), queryKey) {
			return true
		}
	}
	return false
}

func fd2PPVMovieMatches(item ExternalMediaResult, query string) bool {
	queryKey := normalizeAdultMovieSearchText(query)
	return queryKey != "" && (strings.Contains(normalizeAdultMovieSearchText(item.Title), queryKey) ||
		strings.Contains(normalizeAdultMovieSearchText(item.OriginalName), queryKey))
}

func adultFD2SearchNumber(query string) string {
	if number := adultFC2Number(query); number != "" {
		return number
	}
	query = strings.TrimSpace(query)
	if adultFD2RawNumberPattern.MatchString(query) {
		return query
	}
	return ""
}

func adultFD2PageHasNext(body string, page int) bool {
	if page < 1 {
		page = 1
	}
	for _, match := range adultFD2PaginationPattern.FindAllStringSubmatch(body, -1) {
		if len(match) < 2 {
			continue
		}
		attrs := adultAttrs(match[1])
		classes := " " + strings.ToLower(strings.Join(strings.Fields(attrs["class"]), " ")) + " "
		if !strings.Contains(classes, " pagination ") || attrs["data-param"] != "page" {
			continue
		}
		total, err := strconv.Atoi(strings.TrimSpace(attrs["data-total"]))
		return err == nil && total > page
	}
	return false
}
