package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type STRMRefreshResult struct {
	Requested       bool                `json:"requested"`
	Queued          bool                `json:"queued"`
	Reason          string              `json:"reason,omitempty"`
	ScrapeRequested bool                `json:"scrape_requested,omitempty"`
	ScrapeQueued    bool                `json:"scrape_queued,omitempty"`
	ScrapeReason    string              `json:"scrape_reason,omitempty"`
	Targets         []STRMRefreshTarget `json:"targets,omitempty"`
}

type STRMRefreshTarget struct {
	LibraryID string `json:"library_id"`
	RootID    string `json:"root_id,omitempty"`
	Name      string `json:"name"`
	Path      string `json:"path"`
}

func FindSTRMRefreshTargets(ctx context.Context, repo *repository.Container, outputDir string) ([]STRMRefreshTarget, error) {
	if repo == nil || repo.Library == nil {
		return nil, nil
	}
	outputDir = resolveMappedDestinationPath(strings.TrimSpace(outputDir))
	if outputDir == "" || outputDir == "." {
		return nil, nil
	}
	libraries, err := repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	targets := make([]STRMRefreshTarget, 0)
	seen := map[string]struct{}{}
	for i := range libraries {
		lib := libraries[i]
		if !lib.Enabled {
			continue
		}
		roots, err := repo.Library.ListRoots(ctx, lib.ID)
		if err != nil {
			return nil, err
		}
		if len(roots) == 0 && strings.TrimSpace(lib.Path) != "" {
			roots = []model.LibraryRoot{{LibraryID: lib.ID, Path: lib.Path, Enabled: lib.Enabled}}
		}
		for j := range roots {
			root := roots[j]
			if !root.Enabled || strings.TrimSpace(root.Path) == "" {
				continue
			}
			if _, ok := ParseCloudLibraryMount(root.Path); ok {
				continue
			}
			if !strmRefreshPathMatches(outputDir, root.Path) {
				continue
			}
			key := lib.ID + "\x00" + root.ID + "\x00" + strings.ToLower(filepath.Clean(root.Path))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			targets = append(targets, STRMRefreshTarget{
				LibraryID: lib.ID,
				RootID:    root.ID,
				Name:      lib.Name,
				Path:      filepath.Clean(root.Path),
			})
		}
	}
	return targets, nil
}

func strmRefreshPathMatches(outputDir, libraryRoot string) bool {
	outputDir = filepath.Clean(strings.TrimSpace(outputDir))
	libraryRoot = filepath.Clean(strings.TrimSpace(libraryRoot))
	if outputDir == "" || outputDir == "." || libraryRoot == "" || libraryRoot == "." {
		return false
	}
	return sameLibraryPath(outputDir, libraryRoot) || pathWithin(outputDir, libraryRoot) || pathWithin(libraryRoot, outputDir)
}
