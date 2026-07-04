package service

import (
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	nexusPHPFreeLabelRE = regexp.MustCompile(`(?i)(class="[^"]*(?:free|free2|twoupfree|free_download)[^"]*"|促销|免费)`)
	nexusPHPRiskLabelRE = regexp.MustCompile(`(?i)(?:class|title|alt)=["'][^"']*\bhr\b[^"']*["']`)
)

// parseNexusPHPHTML 解析 NexusPHP 种子列表 HTML。
func parseNexusPHPHTML(html, siteName, baseURL string) (*SiteSearchResult, error) {
	result := &SiteSearchResult{
		SiteName: siteName,
		Items:    []TorrentItem{},
		Page:     1,
	}

	for _, row := range nexusPHPTorrentRows(html) {
		item := parseNexusPHPRow(row, baseURL)
		if item.ID != "" {
			result.Items = append(result.Items, item)
		}
	}

	result.Total = len(result.Items)
	return result, nil
}

func nexusPHPPageLooksLogin(pageHTML string) bool {
	lower := strings.ToLower(pageHTML)
	if strings.Contains(lower, "details.php") || strings.Contains(lower, "download.php") {
		return false
	}
	for _, marker := range []string{
		"takelogin.php",
		"id=\"loginform\"",
		"id='loginform'",
		"name=\"loginform\"",
		"name='loginform'",
		"type=\"password\"",
		"type='password'",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// parseNexusPHPRow 解析单行种子条目。
func parseNexusPHPRow(row, baseURL string) TorrentItem {
	item := TorrentItem{}
	if link := firstNexusPHPLink(row, "details.php"); link != nil {
		item.ID = link.query.Get("id")
		item.Title = nexusPHPTitleFromLink(*link)
		item.Subtitle = nexusPHPSubtitle(row)
		item.Labels = nexusPHPRowLabels(row)
		item.DetailURL = resolveSiteURL(baseURL, link.href)
	}
	if link := firstNexusPHPLink(row, "download.php"); link != nil {
		item.DownloadURL = resolveSiteURL(baseURL, link.href)
	}
	if sizeMatches := regexp.MustCompile(`(?i)(\d+\.?\d*)\s*(GiB|MiB|TiB|KiB|GB|MB|TB|KB)`).FindStringSubmatch(row); len(sizeMatches) >= 3 {
		item.Size = parseSizeString(sizeMatches[1], sizeMatches[2])
	}
	if value, ok := nexusPHPIntByClass(row, "seeders"); ok {
		item.Seeders = value
	}
	if value, ok := nexusPHPIntByClass(row, "leechers"); ok {
		item.Leechers = value
	}
	if value, ok := nexusPHPIntByClass(row, "snatched"); ok {
		item.Snatched = value
	}
	if item.Snatched == 0 {
		if m := regexp.MustCompile(`snatched[^"]*"[^>]*>(\d+)`).FindStringSubmatch(row); len(m) >= 2 {
			item.Snatched, _ = strconv.Atoi(m[1])
		}
	}
	item.Free = regexp.MustCompile(`(?i)(class="free|free2|twoupfree|free_download|促销|免费)`).MatchString(row)
	if m := regexp.MustCompile(`(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2})`).FindStringSubmatch(row); len(m) >= 2 {
		if t, err := time.Parse("2006-01-02 15:04", m[1]); err == nil {
			item.UploadTime = t
		}
	}
	if m := regexp.MustCompile(`cat=(\d+)[^"]*"[^>]*title="([^"]+)"`).FindStringSubmatch(row); len(m) >= 3 {
		item.Category = strings.TrimSpace(m[2])
	}
	return item
}

type nexusPHPLink struct {
	href  string
	attrs string
	text  string
	query url.Values
}

func nexusPHPTorrentRows(pageHTML string) []string {
	rows := regexp.MustCompile(`(?is)<tr\b[^>]*>.*?</tr>`).FindAllString(pageHTML, -1)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row), "details.php") {
			out = append(out, row)
		}
	}
	return out
}

func firstNexusPHPLink(row, path string) *nexusPHPLink {
	pattern := regexp.MustCompile(`(?is)<a\b([^>]*href\s*=\s*["']([^"']*)["'][^>]*)>(.*?)</a>`)
	for _, match := range pattern.FindAllStringSubmatch(row, -1) {
		if len(match) < 4 {
			continue
		}
		href := html.UnescapeString(strings.TrimSpace(match[2]))
		parsed, err := url.Parse(href)
		if err != nil || !nexusPHPLinkPathMatches(parsed, path) {
			continue
		}
		return &nexusPHPLink{
			href:  href,
			attrs: match[1],
			text:  cleanNexusPHPText(match[3]),
			query: parsed.Query(),
		}
	}
	return nil
}

func nexusPHPLinkPathMatches(parsed *url.URL, want string) bool {
	if parsed == nil {
		return false
	}
	path := strings.TrimSpace(parsed.Path)
	if path == "" {
		path = strings.TrimSpace(parsed.Opaque)
	}
	path = strings.Trim(strings.ToLower(path), "/")
	want = strings.Trim(strings.ToLower(strings.TrimSpace(want)), "/")
	if path == "" || want == "" {
		return false
	}
	return path == want || strings.HasSuffix(path, "/"+want)
}

func nexusPHPTitleFromLink(link nexusPHPLink) string {
	for _, attr := range []string{"title", "data-title"} {
		if value := htmlAttr(link.attrs, attr); value != "" {
			return value
		}
	}
	return link.text
}

func nexusPHPSubtitle(row string) string {
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?is)<span\b[^>]*(?:class|id)\s*=\s*["'][^"']*(?:subtitle|small_descr|descr|sub)[^"']*["'][^>]*>(.*?)</span>`),
		regexp.MustCompile(`(?is)<font\b[^>]*(?:class|id)\s*=\s*["'][^"']*(?:subtitle|small_descr|descr|sub)[^"']*["'][^>]*>(.*?)</font>`),
	} {
		if match := pattern.FindStringSubmatch(row); len(match) >= 2 {
			return cleanNexusPHPText(match[1])
		}
	}
	return ""
}

func nexusPHPRowLabels(row string) string {
	labels := make([]string, 0, 4)
	lower := strings.ToLower(row)
	add := func(label string) {
		for _, existing := range labels {
			if existing == label {
				return
			}
		}
		labels = append(labels, label)
	}
	if nexusPHPFreeLabelRE.MatchString(row) {
		add("free")
	}
	if strings.Contains(lower, "hit and run") || strings.Contains(lower, "hit&run") || strings.Contains(lower, "h&r") ||
		nexusPHPRiskLabelRE.MatchString(row) ||
		strings.Contains(row, "禁转") || strings.Contains(row, "禁止转载") || strings.Contains(row, "禁下") || strings.Contains(row, "禁止下载") {
		add("HR")
	}
	return strings.Join(labels, " ")
}

func nexusPHPIntByClass(row, className string) (int, bool) {
	pattern := regexp.MustCompile(`(?is)<td\b[^>]*(?:class|id)\s*=\s*["'][^"']*` + regexp.QuoteMeta(className) + `[^"']*["'][^>]*>(.*?)</td>`)
	if match := pattern.FindStringSubmatch(row); len(match) >= 2 {
		text := cleanNexusPHPText(match[1])
		valueMatch := regexp.MustCompile(`\d+`).FindString(text)
		if valueMatch != "" {
			value, _ := strconv.Atoi(valueMatch)
			return value, true
		}
	}
	return 0, false
}

func htmlAttr(attrs, name string) string {
	pattern := regexp.MustCompile(`(?is)\b` + regexp.QuoteMeta(name) + `\s*=\s*["']([^"']*)["']`)
	if match := pattern.FindStringSubmatch(attrs); len(match) >= 2 {
		return cleanNexusPHPText(match[1])
	}
	return ""
}

func cleanNexusPHPText(value string) string {
	return strings.Join(strings.Fields(html.UnescapeString(stripHTML(value))), " ")
}

func resolveSiteURL(baseURL, href string) string {
	base, err := url.Parse(strings.TrimRight(baseURL, "/") + "/")
	if err != nil {
		return strings.TrimSpace(href)
	}
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return strings.TrimSpace(href)
	}
	return base.ResolveReference(ref).String()
}
