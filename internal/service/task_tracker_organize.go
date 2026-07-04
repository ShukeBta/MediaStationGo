package service

import "strings"

func OrganizeTaskMetrics(res *OrganizeResult) map[string]int64 {
	if res == nil {
		return nil
	}
	metrics := map[string]int64{
		"organized":    int64(res.Organized),
		"replaced":     int64(res.Replaced),
		"reclassified": int64(res.Reclassified),
		"skipped":      int64(res.Skipped),
		"errors":       int64(len(res.Errors)),
	}
	var scanVisited, scanAdded, scanUpdated, scanRemoved int64
	for _, scan := range res.Scans {
		scanVisited += int64(scan.Visited)
		scanAdded += int64(scan.Added)
		scanUpdated += int64(scan.Updated)
		scanRemoved += scan.Removed
		if scan.Error != "" {
			metrics["scan_errors"]++
		}
	}
	if len(res.Scans) > 0 {
		metrics["scans"] = int64(len(res.Scans))
		metrics["scan_visited"] = scanVisited
		metrics["scan_added"] = scanAdded
		metrics["scan_updated"] = scanUpdated
		metrics["scan_removed"] = scanRemoved
	}
	for reason, count := range OrganizeSkipReasonCounts(res) {
		metrics["skip_"+organizeMetricKey(reason)] = int64(count)
	}
	var scrapeMatched, scrapeProcessed int64
	for _, scrape := range res.Scrapes {
		scrapeMatched += int64(scrape.Matched)
		scrapeProcessed += int64(scrape.Processed)
		if scrape.Error != "" {
			metrics["scrape_errors"]++
		}
		if scrape.Skipped {
			metrics["scrape_skipped"]++
		}
	}
	if len(res.Scrapes) > 0 {
		metrics["scrapes"] = int64(len(res.Scrapes))
		metrics["scrape_matched"] = scrapeMatched
		metrics["scrape_processed"] = scrapeProcessed
	}
	return metrics
}

func OrganizeTaskDetails(res *OrganizeResult, limit int) []string {
	if res == nil || limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, line := range res.Errors {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, "错误: "+line)
		if len(out) >= limit {
			return out
		}
	}
	for _, item := range res.Items {
		if item.Action != "error" && item.Action != "skip" && item.Action != "reclassify" && item.Action != "cleanup" {
			continue
		}
		line := strings.TrimSpace(item.Source)
		if item.Reason != "" {
			line += ": " + strings.TrimSpace(item.Reason)
		}
		if line == "" {
			continue
		}
		out = append(out, item.Action+": "+line)
		if len(out) >= limit {
			return out
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func OrganizeSkipReasonCounts(res *OrganizeResult) map[string]int {
	if res == nil || len(res.Items) == 0 {
		return nil
	}
	counts := map[string]int{}
	for _, item := range res.Items {
		if item.Action != "skip" {
			continue
		}
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			reason = "unknown"
		}
		counts[reason]++
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func organizeMetricKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_", "/", "_", "\\", "_")
	value = replacer.Replace(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unknown"
	}
	return out
}
