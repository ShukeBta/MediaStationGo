package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// OrganizeScanSummary reports a library scan triggered after directory
// organize. It is intentionally compact for the tools page toast.
type OrganizeScanSummary struct {
	LibraryID string `json:"library_id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Visited   int    `json:"visited"`
	Added     int    `json:"added"`
	Updated   int    `json:"updated"`
	Removed   int64  `json:"removed"`
	Error     string `json:"error,omitempty"`
}

// OrganizeScrapeSummary reports metadata enrichment triggered after organize.
type OrganizeScrapeSummary struct {
	LibraryID string `json:"library_id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Matched   int    `json:"matched"`
	Processed int    `json:"processed"`
	Skipped   bool   `json:"skipped,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Error     string `json:"error,omitempty"`
}

// OrganizeScrapeAfterEnabled decides whether organize workflows should run
// metadata scraping after scanning organized files into libraries.
func OrganizeScrapeAfterEnabled(ctx context.Context, repo *repository.Container) bool {
	if repo == nil || repo.Setting == nil {
		return true
	}
	if value, err := repo.Setting.Get(ctx, "organize.scrape_after"); err == nil && strings.TrimSpace(value) != "" {
		return parseBoolSetting(value, true)
	}
	if value, err := repo.Setting.Get(ctx, "scrape.auto_on_scan"); err == nil && strings.TrimSpace(value) != "" {
		return parseBoolSetting(value, true)
	}
	return true
}

// OrganizeResultHasChanges reports whether an organize run actually changed
// files in the destination library. Skipped duplicates are intentionally not a
// change: scanning after a no-op organize can turn a harmless restart into a
// full library ffprobe sweep.
func OrganizeResultHasChanges(res *OrganizeResult) bool {
	return res != nil && (res.Organized > 0 || res.Replaced > 0 || res.Reclassified > 0)
}

// OrganizeResultNeedsVisibilitySync reports whether a just-finished organize
// should make sure destination files exist in the DB. New/replaced files always
// need it. Skips only need it when the target file exists on disk but was not
// proven to be an already scanned library duplicate; otherwise an automatic
// organize loop with only seeding duplicates would rescan libraries forever.
func OrganizeResultNeedsVisibilitySync(res *OrganizeResult) bool {
	if OrganizeResultHasChanges(res) {
		return true
	}
	if res == nil {
		return false
	}
	for _, item := range res.Items {
		if item.Action != "skip" {
			continue
		}
		switch item.Reason {
		case organizeSkipAlreadyOrganized, organizeSkipTargetExists, "duplicate exists", "target exists":
			return true
		}
	}
	return false
}

// ScanLibrariesForPath recursively scans libraries affected by an organize
// destination. If preferredLibraryID is set, only that library is scanned.
// Otherwise every enabled library whose path intersects destRoot is scanned;
// if no path can be matched, we fall back to all enabled libraries to preserve
// the old "scan all after ingest" UI behavior.
func (s *ScannerService) ScanLibrariesForPath(ctx context.Context, destRoot, preferredLibraryID string) []OrganizeScanSummary {
	scans, _ := s.ScanAndScrapeLibrariesForPath(ctx, destRoot, preferredLibraryID, false)
	return scans
}

// ScanAndScrapeLibrariesForPath scans the affected organize target libraries,
// then optionally runs the scraper synchronously so manual/automatic organize
// can provide deterministic "整理 + 入库 + 刮削" behavior instead of relying on
// a background scan hook that may or may not be enabled.
func (s *ScannerService) ScanAndScrapeLibrariesForPath(ctx context.Context, destRoot, preferredLibraryID string, scrapeAfter bool) ([]OrganizeScanSummary, []OrganizeScrapeSummary) {
	if s == nil || s.repo == nil || s.repo.Library == nil {
		return nil, nil
	}
	libraries, err := s.repo.Library.List(ctx)
	if err != nil {
		return []OrganizeScanSummary{{Error: err.Error()}}, nil
	}
	targets := selectOrganizeScanTargets(libraries, destRoot, preferredLibraryID)
	out := make([]OrganizeScanSummary, 0, len(targets))
	for _, lib := range targets {
		summary := OrganizeScanSummary{
			LibraryID: lib.ID,
			Name:      lib.Name,
			Path:      lib.Path,
		}
		res, err := s.scanLibrary(ctx, lib.ID, !scrapeAfter)
		if err != nil {
			summary.Error = err.Error()
			out = append(out, summary)
			continue
		}
		summary.Visited = res.Visited
		summary.Added = res.Added
		summary.Updated = res.Updated
		summary.Removed = res.Removed
		out = append(out, summary)
	}
	return out, s.scrapeOrganizeTargets(ctx, targets, scrapeAfter)
}

func (s *ScannerService) scrapeOrganizeTargets(ctx context.Context, targets []model.Library, scrapeAfter bool) []OrganizeScrapeSummary {
	if len(targets) == 0 || !scrapeAfter {
		return nil
	}
	out := make([]OrganizeScrapeSummary, 0, len(targets))
	if s.scraper == nil {
		for _, lib := range targets {
			out = append(out, OrganizeScrapeSummary{
				LibraryID: lib.ID,
				Name:      lib.Name,
				Path:      lib.Path,
				Skipped:   true,
				Reason:    "scraper unavailable",
			})
		}
		return out
	}
	if !s.scraper.AnyEnabled() {
		for _, lib := range targets {
			out = append(out, OrganizeScrapeSummary{
				LibraryID: lib.ID,
				Name:      lib.Name,
				Path:      lib.Path,
				Skipped:   true,
				Reason:    "no scraper provider enabled",
			})
		}
		return out
	}
	for _, lib := range targets {
		summary := OrganizeScrapeSummary{
			LibraryID: lib.ID,
			Name:      lib.Name,
			Path:      lib.Path,
		}
		// Organize is an explicit ingest workflow: after rename/classification,
		// previously failed no_match rows should be retried so the operator does
		// not need to run a separate manual scrape.
		options := skipEpisodeArtworkOptions(true)
		options.RefreshWeakMatched = true
		result, err := s.scraper.EnrichLibraryDetailedWithOptions(ctx, lib.ID, options)
		if err != nil {
			summary.Error = err.Error()
		} else {
			summary.Matched = result.Matched
			summary.Processed = result.Processed
		}
		out = append(out, summary)
	}
	return out
}

func selectOrganizeScanTargets(libraries []model.Library, destRoot, preferredLibraryID string) []model.Library {
	preferredLibraryID = strings.TrimSpace(preferredLibraryID)
	enabled := make([]model.Library, 0, len(libraries))
	for _, lib := range libraries {
		if !lib.Enabled {
			continue
		}
		if preferredLibraryID != "" {
			if lib.ID == preferredLibraryID {
				return []model.Library{lib}
			}
			continue
		}
		enabled = append(enabled, lib)
	}
	if preferredLibraryID != "" {
		return nil
	}
	destRoot = strings.TrimSpace(destRoot)
	if destRoot == "" {
		return enabled
	}
	matched := make([]model.Library, 0, len(enabled))
	for _, lib := range enabled {
		if pathWithin(lib.Path, destRoot) || pathWithin(destRoot, lib.Path) {
			matched = append(matched, lib)
		}
	}
	if len(matched) > 0 {
		return dedupeOrganizeScanTargets(matched)
	}
	return dedupeOrganizeScanTargets(enabled)
}

func dedupeOrganizeScanTargets(libraries []model.Library) []model.Library {
	out := make([]model.Library, 0, len(libraries))
	byPath := map[string]int{}
	for _, lib := range libraries {
		key := strings.ToLower(filepath.Clean(lib.Path))
		if existingIndex, ok := byPath[key]; ok {
			if organizeScanTargetScore(lib) > organizeScanTargetScore(out[existingIndex]) {
				out[existingIndex] = lib
			}
			continue
		}
		byPath[key] = len(out)
		out = append(out, lib)
	}
	return out
}

func organizeScanTargetScore(lib model.Library) int {
	libraryType := normalizeOrganizeMediaType(lib.Type)
	inferred := normalizeMediaType("", lib.Name, lib.Path)
	score := 0
	if libraryType != "" && libraryType == inferred {
		score += 10
	}
	if librarySupportsSeasons(&lib) {
		score += 2
	}
	if libraryType != "" && libraryType != "movie" {
		score++
	}
	return score
}
