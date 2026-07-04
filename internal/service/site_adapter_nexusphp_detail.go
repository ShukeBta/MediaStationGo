package service

import (
	"regexp"
	"strconv"
	"strings"
)

// parseNexusPHPDetailHTML 解析种子详情页。
func parseNexusPHPDetailHTML(html, id, baseURL string) (*TorrentDetail, error) {
	detail := &TorrentDetail{
		ID:        id,
		DetailURL: baseURL + "/details.php?id=" + id,
	}
	if m := regexp.MustCompile(`<h1[^>]*>([^<]+)</h1>`).FindStringSubmatch(html); len(m) >= 2 {
		detail.Title = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`<span[^>]*class="[^"]*sub[^"]*"[^>]*>([^<]+)</span>`).FindStringSubmatch(html); len(m) >= 2 {
		detail.Subtitle = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?i)info_hash[^<]*</td>\s*<td[^>]*>([^<]+)</td>`).FindStringSubmatch(html); len(m) >= 2 {
		detail.InfoHash = strings.TrimSpace(m[1])
	}
	if m := regexp.MustCompile(`(?i)imdb[^<]*</td>\s*<td[^>]*>[^<]*(tt\d+)`).FindStringSubmatch(html); len(m) >= 2 {
		detail.ImdbID = m[1]
	}
	if m := regexp.MustCompile(`(?i)size[^<]*</td>\s*<td[^>]*>(\d+\.?\d*)\s*(GB|MB|TB|KB)`).FindStringSubmatch(html); len(m) >= 3 {
		detail.Size = parseSizeString(m[1], m[2])
	}
	if m := regexp.MustCompile(`seeders[^<]*</td>\s*<td[^>]*>(\d+)</td>\s*<td[^>]*>\s*</td>\s*<td[^>]*>\s*</td>\s*<td[^>]*>leechers[^<]*</td>\s*<td[^>]*>(\d+)`).FindStringSubmatch(html); len(m) >= 3 {
		detail.Seeders, _ = strconv.Atoi(m[1])
		detail.Leechers, _ = strconv.Atoi(m[2])
	}
	if m := regexp.MustCompile(`(?i)times completed[^<]*</td>\s*<td[^>]*>(\d+)`).FindStringSubmatch(html); len(m) >= 2 {
		detail.Snatched, _ = strconv.Atoi(m[1])
	}
	if m := regexp.MustCompile(`(?i)<div[^>]*id="kdescr"[^>]*>(.*?)</div>`).FindStringSubmatch(html); len(m) >= 2 {
		detail.Description = stripHTML(m[1])
	}
	detail.DownloadURL = baseURL + "/download.php?id=" + id
	detail.Free = strings.Contains(html, "free") || strings.Contains(html, "免费")
	return detail, nil
}
