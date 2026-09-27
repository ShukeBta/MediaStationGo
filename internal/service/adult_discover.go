package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var (
	adultListStrongPattern     = regexp.MustCompile(`(?is)<strong[^>]*>(.*?)</strong>`)
	adultListDatePattern       = regexp.MustCompile(`(?:19|20)\d{2}-\d{2}-\d{2}`)
	adultListScorePattern      = regexp.MustCompile(`(?i)([0-9](?:\.[0-9]+)?)\s*(?:分|points?)`)
	adultPerformerIDPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	adultMovieIDPattern        = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	adultDiscoveryCodePattern  = regexp.MustCompile(`^[A-Z]{2,10}-[0-9]{2,8}(?:[A-Z]{1,4}[0-9]{1,4}|-[0-9]{1,4})?$`)
	adultJavDBActorHeading     = regexp.MustCompile(`(?is)<h3\b[^>]*>(.*?)</h3>`)
	errAdultPerformerPageEmpty = errors.New("adult performer page returned no usable works")
)

const adultJavDBPerformerSectionCacheTTL = 5 * time.Minute

const (
	adultJavDBPerformerNew     = "new"
	adultJavDBPerformerMonthly = "monthly"
	adultJavDBPerformerFanza   = "fanza"
)

type adultJavDBPerformerSectionFlight struct {
	done     chan struct{}
	sections map[string][]ExternalMediaResult
	err      error
}

type adultJavDBPerformerSectionCache struct {
	mu        sync.Mutex
	sections  map[string][]ExternalMediaResult
	fetchedAt time.Time
	flight    *adultJavDBPerformerSectionFlight
}

func (p *AdultProvider) DiscoverJavDBPopular(ctx context.Context) ([]ExternalMediaResult, error) {
	items, err := p.discoverAdultList(ctx, "javdb", func(base string) string {
		return base + "/rankings/movies?p=daily&t=censored"
	}, parseJavDBMovieList)
	return items, err
}

func (p *AdultProvider) DiscoverJavDBPerformerSection(ctx context.Context, section string) ([]ExternalMediaResult, error) {
	section = strings.ToLower(strings.TrimSpace(section))
	switch section {
	case adultJavDBPerformerNew, adultJavDBPerformerMonthly, adultJavDBPerformerFanza:
	default:
		return nil, fmt.Errorf("unsupported JavDB performer section: %s", section)
	}
	sections, err := p.loadJavDBPerformerSections(ctx)
	if err != nil {
		return nil, err
	}
	items := sections[section]
	if len(items) == 0 {
		return nil, fmt.Errorf("JavDB performer section %s returned no usable items", section)
	}
	return limitAdultDiscoveryItems(items, 30), nil
}

func (p *AdultProvider) loadJavDBPerformerSections(ctx context.Context) (map[string][]ExternalMediaResult, error) {
	if p == nil {
		return nil, errors.New("adult provider is unavailable")
	}
	cache := &p.javDBPerformerSections
	cache.mu.Lock()
	if len(cache.sections) > 0 && time.Since(cache.fetchedAt) < adultJavDBPerformerSectionCacheTTL {
		sections := cloneAdultPerformerSections(cache.sections)
		cache.mu.Unlock()
		return sections, nil
	}
	if cache.flight != nil {
		flight := cache.flight
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			return cloneAdultPerformerSections(flight.sections), flight.err
		}
	}
	flight := &adultJavDBPerformerSectionFlight{done: make(chan struct{})}
	cache.flight = flight
	cache.mu.Unlock()

	sections, err := p.fetchJavDBPerformerSections(ctx)
	cache.mu.Lock()
	flight.sections = cloneAdultPerformerSections(sections)
	flight.err = err
	if err == nil {
		cache.sections = cloneAdultPerformerSections(sections)
		cache.fetchedAt = time.Now()
	}
	cache.flight = nil
	close(flight.done)
	cache.mu.Unlock()
	return cloneAdultPerformerSections(sections), err
}

func (p *AdultProvider) fetchJavDBPerformerSections(ctx context.Context) (map[string][]ExternalMediaResult, error) {
	var lastErr error
	foundSource := false
	for _, base := range p.resolveBases(ctx) {
		if adultSourceKind(base) != "javdb" {
			continue
		}
		foundSource = true
		base = strings.TrimRight(base, "/")
		body, err := p.fetchText(ctx, base+"/actors", base)
		if err != nil {
			lastErr = err
			continue
		}
		sections, err := parseJavDBPerformerSections(body, base)
		if err == nil {
			return sections, nil
		}
		lastErr = err
	}
	if !foundSource {
		return nil, errors.New("adult source javdb is not configured")
	}
	return nil, lastErr
}

func cloneAdultPerformerSections(source map[string][]ExternalMediaResult) map[string][]ExternalMediaResult {
	if source == nil {
		return nil
	}
	cloned := make(map[string][]ExternalMediaResult, len(source))
	for key, items := range source {
		cloned[key] = append([]ExternalMediaResult(nil), items...)
	}
	return cloned
}

func parseJavDBPerformerSections(body, base string) (map[string][]ExternalMediaResult, error) {
	headings := adultJavDBActorHeading.FindAllStringSubmatchIndex(body, -1)
	sections := make(map[string][]ExternalMediaResult, 3)
	for index, heading := range headings {
		if len(heading) < 4 {
			continue
		}
		section := javDBPerformerSectionKey(stripAdultHTML(body[heading[2]:heading[3]]))
		if section == "" {
			continue
		}
		end := len(body)
		if index+1 < len(headings) {
			end = headings[index+1][0]
		}
		sections[section] = parseJavDBPerformerList(body[heading[1]:end], base)
	}
	for _, section := range []string{adultJavDBPerformerNew, adultJavDBPerformerMonthly, adultJavDBPerformerFanza} {
		if len(sections[section]) == 0 {
			return nil, fmt.Errorf("JavDB actors page is missing performer section %s", section)
		}
	}
	return sections, nil
}

func javDBPerformerSectionKey(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	switch {
	case title == "新人":
		return adultJavDBPerformerNew
	case title == "月榜":
		return adultJavDBPerformerMonthly
	case strings.Contains(title, "fanza") || strings.Contains(title, "dmm"):
		return adultJavDBPerformerFanza
	default:
		return ""
	}
}

func FollowedAdultPerformerItems(follows []model.AdultPerformerFollow) []ExternalMediaResult {
	items := make([]ExternalMediaResult, 0, len(follows))
	for _, follow := range follows {
		person := PersonMetadata{
			Name:       follow.Name,
			ImageURL:   follow.ImageURL,
			ProfileURL: follow.ProfileURL,
			Source:     follow.Source,
			SourceID:   follow.SourceID,
		}
		item := adultPerformerItem(person)
		item.Followed = true
		items = append(items, item)
	}
	return items
}

func (p *AdultProvider) SearchPerformers(ctx context.Context, query string) ([]ExternalMediaResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("adult performer query is empty")
	}
	if p == nil {
		return nil, errors.New("adult provider is unavailable")
	}

	var lastErr error
	foundSource := false
	completedSource := false
	for _, base := range p.resolveBases(ctx) {
		if adultSourceKind(base) != "javdb" {
			continue
		}
		foundSource = true
		base = strings.TrimRight(base, "/")
		searchURL := base + "/search?q=" + url.QueryEscape(query) + "&f=actor"
		body, err := p.fetchText(ctx, searchURL, base)
		if err != nil {
			lastErr = err
			continue
		}
		if items := filterAdultPerformerItems(parseJavDBPerformerList(body, base), query); len(items) > 0 {
			return limitAdultDiscoveryItems(items, 30), nil
		}
		items, err := p.searchJavDBPerformerMovieDetails(ctx, base, query, parseJavDBMovieList(body, base))
		if err != nil {
			lastErr = err
			continue
		}
		if len(items) > 0 {
			return limitAdultDiscoveryItems(items, 30), nil
		}
		completedSource = true
	}
	if !foundSource {
		return nil, errors.New("adult source javdb is not configured")
	}
	if lastErr != nil && !completedSource {
		return nil, lastErr
	}
	return []ExternalMediaResult{}, nil
}

func (p *AdultProvider) SearchMovies(ctx context.Context, query string) ([]ExternalMediaResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("adult movie query is empty")
	}
	if p == nil {
		return nil, errors.New("adult provider is unavailable")
	}

	var lastErr error
	foundSource := false
	completedSource := false
	for _, base := range p.resolveBases(ctx) {
		if adultSourceKind(base) != "javdb" {
			continue
		}
		foundSource = true
		base = strings.TrimRight(base, "/")
		body, err := p.fetchText(ctx, base+"/search?q="+url.QueryEscape(query)+"&f=all", base)
		if err != nil {
			lastErr = err
			continue
		}
		completedSource = true
		if items := filterAdultMovieSearchItems(parseJavDBMovieList(body, base), query); len(items) > 0 {
			return limitAdultDiscoveryItems(items, 20), nil
		}
	}
	if !foundSource {
		return nil, errors.New("adult source javdb is not configured")
	}
	if lastErr != nil && !completedSource {
		return nil, lastErr
	}
	return []ExternalMediaResult{}, nil
}

func (p *AdultProvider) searchJavDBPerformerMovieDetails(
	ctx context.Context,
	base string,
	query string,
	movies []ExternalMediaResult,
) ([]ExternalMediaResult, error) {
	const movieLimit = 6
	out := make([]ExternalMediaResult, 0, 4)
	seen := map[string]struct{}{}
	var lastErr error
	attempted := 0
	failed := 0
	for _, movie := range movies {
		if attempted >= movieLimit {
			break
		}
		detailURL := strings.TrimSpace(movie.ProviderURL)
		if detailURL == "" {
			continue
		}
		attempted++
		body, err := p.fetchText(ctx, detailURL, base)
		if err != nil {
			failed++
			lastErr = err
			if p.log != nil {
				p.log.Debug("adult performer search detail failed",
					zap.String("url", detailURL), zap.Error(err))
			}
			continue
		}
		for _, person := range firstAdultPeople(body, "javdb", detailURL) {
			if !adultPerformerQueryMatches(person.Name, query) || person.SourceID == "" {
				continue
			}
			key := strings.ToLower(person.Source) + "\x00" + person.SourceID
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, adultPerformerItem(person))
		}
	}
	if len(out) == 0 && attempted > 0 && failed == attempted {
		return nil, lastErr
	}
	return out, nil
}

func (p *AdultProvider) DiscoverPerformerWorks(ctx context.Context, source, sourceID string, page int) ([]ExternalMediaResult, error) {
	items, _, err := p.DiscoverPerformerWorksPage(ctx, source, sourceID, page)
	return items, err
}

func (p *AdultProvider) DiscoverPerformerWorksPage(ctx context.Context, source, sourceID string, page int) ([]ExternalMediaResult, bool, error) {
	source = strings.ToLower(strings.TrimSpace(source))
	base, profileURL, ok := p.AdultPerformerProfile(ctx, source, sourceID)
	if !ok {
		return nil, false, errors.New("unsupported adult performer source")
	}
	if page < 1 {
		page = 1
	}
	target := profileURL
	if page > 1 {
		target += "?page=" + strconv.Itoa(page)
	}
	var (
		body string
		err  error
	)
	if source == "fd2ppv" {
		body, err = p.fetchFD2PPVText(ctx, target)
	} else {
		body, err = p.fetchText(ctx, target, base)
	}
	if err != nil {
		return nil, false, err
	}
	items := parseJavDBMovieList(body, base)
	hasNext := adultPageHasNext(body)
	if source == "fd2ppv" {
		items = parseFD2PPVMovieList(body, base)
		hasNext = adultFD2PageHasNext(body, page)
	}
	if len(items) == 0 {
		return nil, false, errAdultPerformerPageEmpty
	}
	return limitAdultDiscoveryItems(items, 40), hasNext, nil
}

func (p *AdultProvider) DiscoverMovieDetail(ctx context.Context, source, providerID, code string) (ExternalMediaResult, error) {
	if p == nil {
		return ExternalMediaResult{}, errors.New("adult provider is unavailable")
	}
	source = strings.ToLower(strings.TrimSpace(source))
	providerID = strings.TrimSpace(providerID)
	code = normalizeAdultDiscoveryCode(code)
	if source == "fd2ppv" {
		if !adultFD2RawNumberPattern.MatchString(providerID) {
			return ExternalMediaResult{}, errors.New("unsupported adult movie source")
		}
		if code == "" {
			code = "FC2-PPV-" + providerID
		}
		detailURL := strings.TrimRight(fd2PPVBaseURL, "/") + "/articles/" + url.PathEscape(providerID)
		body, err := p.fetchFD2PPVText(ctx, detailURL)
		if err != nil {
			return ExternalMediaResult{}, err
		}
		match := parseFD2PPVDetailHTML(body, code, detailURL)
		if match == nil {
			return ExternalMediaResult{}, errors.New("adult movie detail returned no usable metadata")
		}
		return externalAdultMovieResult(source, providerID, detailURL, match), nil
	}
	if source != "javdb" || !adultMovieIDPattern.MatchString(providerID) {
		return ExternalMediaResult{}, errors.New("unsupported adult movie source")
	}
	if code == "" {
		return ExternalMediaResult{}, errors.New("adult movie code is empty")
	}

	var lastErr error
	foundSource := false
	for _, base := range p.resolveBases(ctx) {
		if adultSourceKind(base) != source {
			continue
		}
		foundSource = true
		base = strings.TrimRight(base, "/")
		detailURL := base + "/v/" + url.PathEscape(providerID)
		body, err := p.fetchText(ctx, detailURL, base)
		if err != nil {
			lastErr = err
			continue
		}
		match := parseAdultDetailHTML(body, code, source, detailURL)
		if match == nil {
			lastErr = errors.New("adult movie detail returned no usable metadata")
			continue
		}
		return externalAdultMovieResult(source, providerID, detailURL, match), nil
	}
	if !foundSource {
		return ExternalMediaResult{}, errors.New("adult source javdb is not configured")
	}
	if lastErr != nil {
		return ExternalMediaResult{}, lastErr
	}
	return ExternalMediaResult{}, errors.New("adult movie detail is unavailable")
}

func externalAdultMovieResult(source, providerID, detailURL string, match *Match) ExternalMediaResult {
	if match == nil {
		return ExternalMediaResult{}
	}
	code := normalizeAdultDiscoveryCode(match.OriginalName)
	return ExternalMediaResult{
		Source:           source,
		MediaType:        "adult",
		Title:            match.Title,
		OriginalName:     code,
		Overview:         match.Overview,
		PosterURL:        match.PosterURL,
		BackdropURL:      match.BackdropURL,
		PreviewImages:    match.PreviewImages,
		Year:             match.Year,
		ReleaseDate:      match.ReleaseDate,
		Rating:           match.Rating,
		DurationMinutes:  match.DurationMinutes,
		Maker:            match.Maker,
		SubscribeKeyword: code,
		SubscribeAliases: compactUniqueStrings(code, match.Title),
		Genres:           match.Genres,
		Actors:           match.Actors,
		People:           match.People,
		NSFW:             true,
		ProviderURL:      detailURL,
		ProviderID:       providerID,
	}
}

func (p *AdultProvider) DiscoverFollowedPerformerWorks(ctx context.Context, follows []model.AdultPerformerFollow, page int) ([]ExternalMediaResult, error) {
	items, _, err := p.discoverFollowedPerformerWorksPage(ctx, follows, page)
	return items, err
}

func (p *AdultProvider) discoverFollowedPerformerWorksPage(
	ctx context.Context,
	follows []model.AdultPerformerFollow,
	page int,
) ([]ExternalMediaResult, bool, error) {
	if len(follows) == 0 {
		return []ExternalMediaResult{}, false, nil
	}
	type result struct {
		follow  model.AdultPerformerFollow
		items   []ExternalMediaResult
		hasNext bool
		err     error
	}
	results := make(chan result, len(follows))
	semaphore := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for _, follow := range follows {
		follow := follow
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results <- result{follow: follow, err: ctx.Err()}
				return
			}
			items, hasNext, err := p.DiscoverPerformerWorksPage(ctx, follow.Source, follow.SourceID, page)
			results <- result{follow: follow, items: items, hasNext: hasNext, err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	out := make([]ExternalMediaResult, 0, len(follows)*8)
	seen := map[string]struct{}{}
	errs := make([]error, 0)
	hasNext := false
	for fetched := range results {
		if fetched.err != nil {
			if errors.Is(fetched.err, errAdultPerformerPageEmpty) {
				continue
			}
			errs = append(errs, fmt.Errorf("%s: %w", fetched.follow.Name, fetched.err))
			continue
		}
		hasNext = hasNext || fetched.hasNext
		person := PersonMetadata{
			Name:       fetched.follow.Name,
			ImageURL:   fetched.follow.ImageURL,
			ProfileURL: fetched.follow.ProfileURL,
			Source:     fetched.follow.Source,
			SourceID:   fetched.follow.SourceID,
		}
		for _, item := range fetched.items {
			key := strings.ToLower(strings.TrimSpace(item.Source + "\x00" + item.OriginalName))
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(item.Source + "\x00" + item.Title))
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			item.Actors = []string{fetched.follow.Name}
			item.People = []PersonMetadata{person}
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].ReleaseDate > out[j].ReleaseDate
	})
	if len(out) > 40 {
		out = out[:40]
		hasNext = true
	}
	if len(out) == 0 && len(errs) > 0 {
		return nil, false, errors.Join(errs...)
	}
	return out, hasNext, nil
}

// DiscoverFollowedPerformerWorksWindow maps the 40-item aggregate pages used
// by the source scraper into continuous logical pages. It returns pageSize
// items plus one extra item so the handler can expose an accurate next button.
func (p *AdultProvider) DiscoverFollowedPerformerWorksWindow(
	ctx context.Context,
	follows []model.AdultPerformerFollow,
	page int,
	pageSize int,
) ([]ExternalMediaResult, error) {
	if pageSize <= 0 || len(follows) == 0 {
		return []ExternalMediaResult{}, nil
	}
	const sourcePageSize = 40
	sourcePage, sourceOffset := discoverWindowStart(page, pageSize, sourcePageSize)
	targetSize := pageSize + 1
	out := make([]ExternalMediaResult, 0, targetSize)
	for len(out) < targetSize {
		chunk, chunkHasNext, err := p.discoverFollowedPerformerWorksPage(ctx, follows, sourcePage)
		if err != nil {
			return nil, err
		}
		if sourceOffset < len(chunk) {
			remaining := targetSize - len(out)
			available := chunk[sourceOffset:]
			if len(available) > remaining {
				available = available[:remaining]
			}
			out = append(out, available...)
		}
		if !chunkHasNext && len(chunk) < sourcePageSize {
			break
		}
		sourcePage++
		sourceOffset = 0
	}
	return out, nil
}

func limitAdultDiscoveryItems(items []ExternalMediaResult, limit int) []ExternalMediaResult {
	if limit <= 0 || len(items) <= limit {
		return items
	}
	return items[:limit]
}

func (p *AdultProvider) AdultPerformerProfile(ctx context.Context, source, sourceID string) (string, string, bool) {
	source = strings.ToLower(strings.TrimSpace(source))
	sourceID = strings.TrimSpace(sourceID)
	if !adultPerformerIDPattern.MatchString(sourceID) {
		return "", "", false
	}
	if source == "fd2ppv" {
		if !p.FD2PPVEnabled() {
			return "", "", false
		}
		base := strings.TrimRight(fd2PPVBaseURL, "/")
		return base, base + "/actresses/" + url.PathEscape(sourceID), true
	}
	if source != "javdb" {
		return "", "", false
	}
	for _, base := range p.resolveBases(ctx) {
		if adultSourceKind(base) != source {
			continue
		}
		base = strings.TrimRight(base, "/")
		return base, base + "/actors/" + url.PathEscape(sourceID), true
	}
	return "", "", false
}

func NormalizeAdultPerformerImageURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") {
		return "", false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if !adultPerformerImageHostAllowed(host) {
		return "", false
	}
	return u.String(), true
}

func adultPerformerImageHostAllowed(host string) bool {
	if host == "jdbstatic.com" || strings.HasSuffix(host, ".jdbstatic.com") {
		return true
	}
	if host == "fd2ppv.cc" || strings.HasSuffix(host, ".fd2ppv.cc") ||
		host == "xximgs.cc" || strings.HasSuffix(host, ".xximgs.cc") ||
		host == "ppvdatabank.com" || strings.HasSuffix(host, ".ppvdatabank.com") {
		return true
	}
	return host == "contents.fc2.com" || strings.HasSuffix(host, ".contents.fc2.com") ||
		(strings.HasPrefix(host, "contents-thumbnail") && strings.HasSuffix(host, ".fc2.com"))
}

func (p *AdultProvider) discoverAdultList(
	ctx context.Context,
	source string,
	targetURL func(base string) string,
	parse func(body, base string) []ExternalMediaResult,
) ([]ExternalMediaResult, error) {
	if p == nil {
		return nil, errors.New("adult provider is unavailable")
	}
	var lastErr error
	foundSource := false
	for _, base := range p.resolveBases(ctx) {
		if adultSourceKind(base) != source {
			continue
		}
		foundSource = true
		base = strings.TrimRight(base, "/")
		body, err := p.fetchText(ctx, targetURL(base), base)
		if err != nil {
			lastErr = err
			continue
		}
		items := parse(body, base)
		if len(items) > 0 {
			return items, nil
		}
		lastErr = fmt.Errorf("adult source %s returned no usable discovery items", base)
	}
	if !foundSource {
		return nil, fmt.Errorf("adult source %s is not configured", source)
	}
	return nil, lastErr
}

func parseJavDBMovieList(body, base string) []ExternalMediaResult {
	return parseAdultMovieList(body, base, "javdb", "box", "/v/")
}

func parseAdultMovieList(body, base, source, className, hrefNeedle string) []ExternalMediaResult {
	out := make([]ExternalMediaResult, 0, 40)
	seen := map[string]struct{}{}
	for _, found := range adultAnchorPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 3 {
			continue
		}
		attrs := adultAttrs(found[1])
		classes := " " + strings.ToLower(strings.TrimSpace(attrs["class"])) + " "
		href := strings.TrimSpace(attrs["href"])
		if href == "" || (hrefNeedle != "" && !strings.Contains(href, hrefNeedle)) {
			continue
		}
		if !strings.Contains(classes, " "+className+" ") && !(source == "javdb" && strings.Contains(href, "/v/")) {
			continue
		}
		inner := found[2]
		code := adultListCode(inner, href)
		if code == "" {
			continue
		}
		key := strings.ToUpper(code)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		detailURL := absolutizeURL(base, href)
		title := adultListTitle(attrs, inner, code)
		item := ExternalMediaResult{
			Source:           source,
			MediaType:        "adult",
			Title:            firstNonEmpty(title, code),
			OriginalName:     code,
			PosterURL:        absolutizeURL(base, firstAdultListImage(inner)),
			ReleaseDate:      adultListReleaseDate(inner),
			Rating:           adultListRating(inner),
			SubscribeKeyword: code,
			SubscribeAliases: compactUniqueStrings(code, title),
			Genres:           []string{"Adult", source},
			NSFW:             true,
			ProviderURL:      detailURL,
			ProviderID:       adultProviderItemID(detailURL),
		}
		if len(item.ReleaseDate) >= 4 {
			item.Year, _ = strconv.Atoi(item.ReleaseDate[:4])
		}
		out = append(out, item)
	}
	return out
}

func parseJavDBPerformerList(body, base string) []ExternalMediaResult {
	out := make([]ExternalMediaResult, 0, 40)
	seen := map[string]struct{}{}
	for _, found := range adultAnchorPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 3 {
			continue
		}
		attrs := adultAttrs(found[1])
		href := strings.TrimSpace(attrs["href"])
		sourceID := adultPerformerSourceID(href)
		if sourceID == "" {
			continue
		}
		if _, exists := seen[sourceID]; exists {
			continue
		}
		name := stripAdultHTML(found[2])
		if m := adultListStrongPattern.FindStringSubmatch(found[2]); len(m) > 1 {
			name = stripAdultHTML(m[1])
		}
		if name == "" {
			name = strings.TrimSpace(strings.Split(attrs["title"], ",")[0])
		}
		if !validAdultActorName(name) {
			continue
		}
		seen[sourceID] = struct{}{}
		profileURL := absolutizeURL(base, href)
		out = append(out, ExternalMediaResult{
			Source:      "javdb",
			MediaType:   "person",
			Title:       name,
			PosterURL:   absolutizeURL(base, firstAdultListImage(found[2])),
			NSFW:        true,
			ProviderURL: profileURL,
			ProviderID:  sourceID,
			People: []PersonMetadata{{
				Name:       name,
				ImageURL:   absolutizeURL(base, firstAdultListImage(found[2])),
				ProfileURL: profileURL,
				Source:     "javdb",
				SourceID:   sourceID,
			}},
		})
	}
	return out
}

func filterAdultPerformerItems(items []ExternalMediaResult, query string) []ExternalMediaResult {
	out := make([]ExternalMediaResult, 0, len(items))
	for _, item := range items {
		if adultPerformerQueryMatches(item.Title, query) {
			out = append(out, item)
		}
	}
	return out
}

func filterAdultMovieSearchItems(items []ExternalMediaResult, query string) []ExternalMediaResult {
	out := make([]ExternalMediaResult, 0, len(items))
	if code := normalizeAdultDiscoveryCode(query); code != "" {
		for _, item := range items {
			if normalizeAdultDiscoveryCode(item.OriginalName) == code || normalizeAdultDiscoveryCode(item.SubscribeKeyword) == code {
				out = append(out, item)
			}
		}
		return out
	}

	queryKey := normalizeAdultMovieSearchText(query)
	if queryKey == "" {
		return out
	}
	for _, item := range items {
		if strings.Contains(normalizeAdultMovieSearchText(item.Title), queryKey) ||
			strings.Contains(normalizeAdultMovieSearchText(item.OriginalName), queryKey) {
			out = append(out, item)
		}
	}
	return out
}

func normalizeAdultMovieSearchText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func adultPerformerQueryMatches(name, query string) bool {
	nameKey := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
	queryKey := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(query)), " "))
	if nameKey == "" || queryKey == "" {
		return false
	}
	return strings.Contains(nameKey, queryKey) || strings.Contains(queryKey, nameKey)
}

func adultPerformerItem(person PersonMetadata) ExternalMediaResult {
	source := strings.ToLower(strings.TrimSpace(person.Source))
	if source == "" {
		source = "javdb"
	}
	return ExternalMediaResult{
		Source:      source,
		MediaType:   "person",
		Title:       strings.TrimSpace(person.Name),
		PosterURL:   strings.TrimSpace(person.ImageURL),
		NSFW:        true,
		ProviderURL: strings.TrimSpace(person.ProfileURL),
		ProviderID:  strings.TrimSpace(person.SourceID),
		People:      []PersonMetadata{person},
	}
}

func adultListCode(inner, href string) string {
	if m := adultListStrongPattern.FindStringSubmatch(inner); len(m) > 1 {
		if code := normalizeAdultDiscoveryCode(stripAdultHTML(m[1])); code != "" {
			return code
		}
	}
	for _, raw := range regexp.MustCompile(`(?is)<date[^>]*>(.*?)</date>`).FindAllStringSubmatch(inner, -1) {
		if len(raw) > 1 {
			if code := normalizeAdultDiscoveryCode(stripAdultHTML(raw[1])); code != "" {
				return code
			}
		}
	}
	return normalizeAdultDiscoveryCode(href)
}

func normalizeAdultDiscoveryCode(input string) string {
	exact := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(input), "_", "-"))
	if adultDiscoveryCodePattern.MatchString(exact) {
		return exact
	}
	return normalizeAdultCode(input)
}

func adultPageHasNext(body string) bool {
	for _, found := range adultAnchorPattern.FindAllStringSubmatch(body, -1) {
		if len(found) < 2 {
			continue
		}
		attrs := adultAttrs(found[1])
		for _, rel := range strings.Fields(strings.ToLower(attrs["rel"])) {
			if rel == "next" && strings.TrimSpace(attrs["href"]) != "" {
				return true
			}
		}
	}
	return false
}

func adultListTitle(attrs map[string]string, inner, code string) string {
	if title := strings.TrimSpace(attrs["title"]); title != "" {
		return strings.TrimSpace(strings.TrimPrefix(title, code))
	}
	if image := adultImagePattern.FindStringSubmatch(inner); len(image) > 1 {
		imageAttrs := adultAttrs(image[1])
		if title := firstText(imageAttrs["title"], imageAttrs["alt"]); title != "" {
			return strings.TrimSpace(strings.TrimPrefix(title, code))
		}
	}
	return ""
}

func firstAdultListImage(inner string) string {
	image := adultImagePattern.FindStringSubmatch(inner)
	if len(image) < 2 {
		return ""
	}
	attrs := adultAttrs(image[1])
	return firstText(attrs["data-src"], attrs["data-original"], attrs["data-lazy-src"], attrs["src"])
}

func adultListReleaseDate(inner string) string {
	return adultListDatePattern.FindString(inner)
}

func adultListRating(inner string) float32 {
	m := adultListScorePattern.FindStringSubmatch(stripAdultHTML(inner))
	if len(m) < 2 {
		return 0
	}
	value, _ := strconv.ParseFloat(m[1], 32)
	return float32(value)
}

func adultProviderItemID(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(u.Path[strings.LastIndex(u.Path, "/")+1:]), "/")
}

func adultPerformerSourceID(href string) string {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "actors" || !adultPerformerIDPattern.MatchString(parts[1]) {
		return ""
	}
	switch strings.ToLower(parts[1]) {
	case "censored", "uncensored", "western", "male":
		return ""
	default:
		return parts[1]
	}
}
