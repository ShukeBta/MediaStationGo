package service

import "strings"

// Item-level task status. A single BackgroundTask (one organize → scan →
// scrape run) is broken down into per-file / per-library TaskItemRecord rows so
// the operator can watch exactly which file is being organized, renamed, ingested
// or scraped, and retry only the failed ones.
const (
	ItemStatusPending   = "pending"   // 待进行
	ItemStatusRunning   = "running"   // 进行中
	ItemStatusSucceeded = "succeeded" // 成功
	ItemStatusFailed    = "failed"    // 失败
)

// Item kind / phase. 整理与重命名 share the "organize" phase (per file);
// scan（入库）与 scrape（刮削）are reported per library.
const (
	ItemKindOrganize = "organize"
	ItemKindScan     = "scan"
	ItemKindScrape   = "scrape"
)

// TaskItemRecord is a single row on the live tasks board. Each record maps to
// exactly one file (organize) or one library (ingest / scrape).
type TaskItemRecord struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`              // organize / scan / scrape
	Status    string `json:"status"`            // pending / running / succeeded / failed
	Name      string `json:"name"`              // display name (file base name or library name)
	Source    string `json:"source,omitempty"`  // source file path (organize)
	DestPath  string `json:"dest_path,omitempty"`
	LibraryID string `json:"library_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// organizeItemsFromResult converts the final organize result items into
// per-file task records. The organize result is only complete after the whole
// directory walk finishes, so this is called once at stage boundaries rather
// than per file. Each item is keyed by its source path for stable identity.
func organizeItemsFromResult(res *OrganizeResult) []TaskItemRecord {
	if res == nil || len(res.Items) == 0 {
		return nil
	}
	out := make([]TaskItemRecord, 0, len(res.Items))
	for _, item := range res.Items {
		rec := TaskItemRecord{
			ID:       "organize:" + item.Source,
			Kind:     ItemKindOrganize,
			Name:     itemBaseName(item.Source, item.Title),
			Source:   item.Source,
			DestPath: item.Target,
		}
		switch item.Action {
		case "error":
			rec.Status = ItemStatusFailed
			rec.Error = strings.TrimSpace(item.Reason)
		default:
			// organize / replace / reclassify / cleanup / skip all count as done.
			rec.Status = ItemStatusSucceeded
		}
		out = append(out, rec)
	}
	return out
}

// scanItemsFromResult converts per-library scan summaries into task records.
func scanItemsFromResult(res *OrganizeResult) []TaskItemRecord {
	if res == nil || len(res.Scans) == 0 {
		return nil
	}
	out := make([]TaskItemRecord, 0, len(res.Scans))
	for _, scan := range res.Scans {
		rec := TaskItemRecord{
			ID:        "scan:" + scan.LibraryID,
			Kind:      ItemKindScan,
			Name:      scan.Name,
			LibraryID: scan.LibraryID,
			Status:    ItemStatusSucceeded,
		}
		if scan.Error != "" {
			rec.Status = ItemStatusFailed
			rec.Error = scan.Error
		}
		out = append(out, rec)
	}
	return out
}

// scrapeItemsFromResult converts per-library scrape summaries into task records.
func scrapeItemsFromResult(res *OrganizeResult) []TaskItemRecord {
	if res == nil || len(res.Scrapes) == 0 {
		return nil
	}
	out := make([]TaskItemRecord, 0, len(res.Scrapes))
	for _, scrape := range res.Scrapes {
		rec := TaskItemRecord{
			ID:        "scrape:" + scrape.LibraryID,
			Kind:      ItemKindScrape,
			Name:      scrape.Name,
			LibraryID: scrape.LibraryID,
			Status:    ItemStatusSucceeded,
		}
		if scrape.Error != "" {
			rec.Status = ItemStatusFailed
			rec.Error = scrape.Error
			out = append(out, rec)
			continue
		}
		if scrape.Skipped {
			rec.Status = ItemStatusSucceeded
		}
		out = append(out, rec)
	}
	return out
}

// combineOrganizeItems merges organize + scan + scrape item rows into one flat
// list ordered by phase, deduping by ID (scan/scrape share the library ID and
// there is at most one of each per run).
func combineOrganizeItems(res *OrganizeResult) []TaskItemRecord {
	items := organizeItemsFromResult(res)
	items = append(items, scanItemsFromResult(res)...)
	items = append(items, scrapeItemsFromResult(res)...)
	return items
}

func itemBaseName(source, title string) string {
	if strings.TrimSpace(title) != "" && !strings.Contains(title, ".") {
		return strings.TrimSpace(title)
	}
	if source == "" {
		return ""
	}
	base := source
	if idx := strings.LastIndexAny(base, "/\\"); idx >= 0 {
		base = base[idx+1:]
	}
	if idx := strings.LastIndex(base, "."); idx > 0 {
		base = base[:idx]
	}
	if base == "" {
		return source
	}
	return base
}
