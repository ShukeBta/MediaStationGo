package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type organizeDirectoryLayout struct {
	MediaType string
	Category  string
}

type organizeSourceFileRequest struct {
	Source                string
	SourceRoot            string
	DestRoot              string
	Mode                  TransferMode
	MediaTypeOverride     string
	MediaCategoryOverride string
	DryRun                bool
	AllowReplaceExisting  bool
	MetadataCache         map[string]*Match
	Result                *OrganizeResult
}

type organizeSourceIdentity struct {
	Ext         string
	Title       string
	ParsedTitle string
	Year        int
	Season      int
	Episode     int
	SourceMedia *model.Media
}

type organizeSourceFilePlan struct {
	Identity        organizeSourceIdentity
	Layout          organizeDirectoryLayout
	LayoutRoot      string
	TargetLibraryID string
	MetadataMatch   *Match
	Target          organizeTargetPath
}

type organizeExistingSourceVersions struct {
	External []string
	Identity []string
	Folder   []string
	All      []string
}

// organizeSourceFile organizes a single video file from the source directory
// into destRoot, applying dedup + 洗版.
func (o *OrganizerService) organizeSourceFile(ctx context.Context, req organizeSourceFileRequest) error {
	plan, err := o.buildOrganizeSourceFilePlan(ctx, req)
	if err != nil {
		return err
	}
	if filepath.Clean(req.Source) == filepath.Clean(plan.Target.Path) {
		skipAlreadyOrganizedSource(req, plan)
		return nil
	}
	existing := o.collectExistingSourceVersions(ctx, req, plan)
	if len(existing.All) > 0 {
		return o.handleExistingSourceVersions(ctx, req, plan, existing)
	}
	return o.writeOrganizedSourceFile(ctx, req, plan)
}

func organizeTitleFromParentFolder(src, sourceRoot string, seriesLike bool) (string, int) {
	if !seriesLike {
		return "", 0
	}
	raw := seriesFolderTitle(src, sourceRoot)
	if strings.TrimSpace(raw) == "" {
		return "", 0
	}
	title, year := CleanQuery(raw)
	if title == "" {
		title = strings.TrimSpace(raw)
	}
	return title, year
}

func shouldSkipOrganizeSourceVideo(path, sourceRoot string) (bool, string) {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(sourceRoot)
	if rel, err := filepath.Rel(cleanRoot, cleanPath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		dir := filepath.Dir(rel)
		if dir != "." {
			for _, part := range strings.Split(dir, string(os.PathSeparator)) {
				switch normalizeOrganizeCategoryKey(part) {
				case "sample", "samples", "trailer", "trailers", "preview", "previews", "teaser", "teasers":
					return true, organizeSkipSampleClip
				}
			}
		}
	}
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(cleanPath), filepath.Ext(cleanPath)))
	normalized := strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(base)
	fields := strings.Fields(normalized)
	if len(fields) == 0 {
		return false, ""
	}
	if len(fields) == 1 && strings.HasPrefix(fields[0], "sample") {
		return true, organizeSkipSampleClip
	}
	last := fields[len(fields)-1]
	switch last {
	case "sample", "trailer", "preview", "teaser":
		return true, organizeSkipSampleClip
	}
	return false, ""
}

func normalizeOrganizeMediaType(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie", "film":
		return "movie"
	case "tv", "series", "show", "drama":
		return "tv"
	case "anime", "animation":
		return "anime"
	case "variety":
		return "variety"
	case "adult", "nsfw":
		return "adult"
	default:
		return ""
	}
}

func organizeWeakFileTitle(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" {
		return true
	}
	fields := strings.Fields(strings.ToLower(title))
	if len(fields) == 0 {
		return true
	}
	meaningful := 0
	for _, field := range fields {
		if _, ok := noiseTokenSet[field]; ok {
			continue
		}
		if _, ok := releaseBoundaryTokenSet[field]; ok {
			continue
		}
		if len(field) == 4 && strings.HasPrefix(field, "20") {
			continue
		}
		meaningful++
	}
	return meaningful == 0
}

func (o *OrganizerService) inferMediaTypeForSourceFile(src, title string, season, episode int) string {
	if season > 0 || episode > 0 {
		return "tv"
	}
	return normalizeMediaType("", title, src)
}
