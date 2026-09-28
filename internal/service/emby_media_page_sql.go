package service

import (
	"context"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

type embyCollapsedMediaPageRow struct {
	model.Media `gorm:"embedded"`
	TotalGroups int64 `gorm:"column:total_groups"`
}

// collapsedMediaPageSQL selects logical Emby media groups and paginates them
// before loading complete media rows. The former path loaded every matching
// physical row into Go and only then collapsed versions, so a small page was
// still linear in the whole library size.
func (e *EmbyService) collapsedMediaPageSQL(
	ctx context.Context,
	scope *gorm.DB,
	p ItemsParams,
	requestedOrder string,
	resumeFilter bool,
	descending bool,
) ([]model.Media, int64, error) {
	if e == nil || e.repo == nil || e.repo.DB == nil || scope == nil {
		return []model.Media{}, 0, nil
	}
	if p.StartIndex < 0 {
		p.StartIndex = 0
	}
	if p.Limit <= 0 {
		p.Limit = 50
	}

	libraryGroupSQL := "TRIM(media.library_id)"
	var libraryGroupArgs []any
	if strings.TrimSpace(p.ParentID) == "" {
		libraryGroupSQL, libraryGroupArgs = e.embyLibraryGroupSQL(ctx)
	}
	datePlayedSelect := "NULL AS emby_date_played"
	if resumeFilter {
		datePlayedSelect = "resume.watched_at AS emby_date_played"
	}
	const candidateColumns = `media.id, media.library_id, media.emby_version_key, media.part_group_key, media.part_index,
media.strm_url, media.path, media.width, media.size_bytes, media.created_at, media.updated_at,
media.release_date, media.year, media.title, media.rating`
	scoped := scope.Session(&gorm.Session{}).Select(
		candidateColumns+", ("+libraryGroupSQL+") AS library_group, "+datePlayedSelect,
		libraryGroupArgs...,
	)

	groupKeySQL := "candidate.emby_version_key"
	if strings.TrimSpace(p.ParentID) == "" {
		groupKeySQL = "candidate.library_group || ':' || candidate.emby_version_key"
	}
	keyed := e.repo.DB.WithContext(ctx).Table("(?) AS candidate", scoped).
		Select("candidate.*, " + groupKeySQL + " AS version_group")

	candidateOrder := strings.ReplaceAll(requestedOrder, "media.", "candidate.")
	candidateOrder = strings.ReplaceAll(candidateOrder, "resume.watched_at", "candidate.emby_date_played")
	if strings.TrimSpace(candidateOrder) == "" {
		candidateOrder = "candidate.release_date DESC, candidate.year DESC, candidate.created_at DESC, candidate.id DESC"
	}
	representativeOrder := `
CASE WHEN TRIM(COALESCE(candidate.part_group_key, '')) <> '' AND candidate.part_index > 0 THEN 0 ELSE 1 END ASC,
CASE WHEN TRIM(COALESCE(candidate.part_group_key, '')) <> '' AND candidate.part_index > 0 THEN candidate.part_index ELSE 2147483647 END ASC,
CASE WHEN TRIM(COALESCE(candidate.strm_url, '')) <> '' OR LOWER(TRIM(COALESCE(candidate.path, ''))) LIKE 'cloud://%%' THEN 0 ELSE 1 END DESC,
candidate.width DESC, candidate.size_bytes DESC, candidate.created_at DESC, ` + candidateOrder
	isDateCreated := primarySupportedEmbySort(p.SortBy, resumeFilter) == "datecreated"
	if isDateCreated && e.repo.DB.Dialector.Name() == "postgres" {
		representatives := e.repo.DB.WithContext(ctx).Table("(?) AS candidate", keyed).
			Select("DISTINCT ON (candidate.version_group) candidate.id, candidate.created_at").
			Order("candidate.version_group, " + representativeOrder)
		selected := e.repo.DB.WithContext(ctx).Table("(?) AS representatives", representatives).
			Select("representatives.*, COUNT(*) OVER () AS total_groups").
			Order("representatives.created_at " + embySortDirection(descending) + ", representatives.id " + embySortDirection(descending)).
			Offset(p.StartIndex).Limit(p.Limit)
		return e.loadCollapsedMediaPage(ctx, selected, "selected.created_at "+embySortDirection(descending)+", selected.id "+embySortDirection(descending), p.StartIndex, representatives)
	}

	rankedSelect := `candidate.id, candidate.created_at, candidate.version_group,
ROW_NUMBER() OVER (PARTITION BY candidate.version_group ORDER BY ` + representativeOrder + `) AS representative_rank`
	groupedSelect := `ranked.version_group,
MAX(CASE WHEN ranked.representative_rank = 1 THEN ranked.id END) AS representative_id,
MAX(CASE WHEN ranked.representative_rank = 1 THEN ranked.created_at END) AS representative_created_at`
	if !isDateCreated {
		rankedSelect += ", ROW_NUMBER() OVER (ORDER BY " + candidateOrder + ") AS global_sort_rank"
		groupedSelect += ", MIN(ranked.global_sort_rank) AS first_sort_rank"
	}
	ranked := e.repo.DB.WithContext(ctx).Table("(?) AS candidate", keyed).Select(rankedSelect)
	grouped := e.repo.DB.WithContext(ctx).Table("(?) AS ranked", ranked).Select(groupedSelect).Group("ranked.version_group")

	selectedOrder := "selected.first_sort_rank ASC"
	groupedOrder := "grouped.first_sort_rank ASC"
	if isDateCreated {
		direction := "ASC"
		if descending {
			direction = "DESC"
		}
		selectedOrder = "selected.representative_created_at " + direction + ", selected.representative_id " + direction
		groupedOrder = "grouped.representative_created_at " + direction + ", grouped.representative_id " + direction
	}
	selected := e.repo.DB.WithContext(ctx).Table("(?) AS grouped", grouped).
		Select("grouped.representative_id AS id, grouped.*, COUNT(*) OVER () AS total_groups").
		Order(groupedOrder).Offset(p.StartIndex).Limit(p.Limit)

	return e.loadCollapsedMediaPage(ctx, selected, selectedOrder, p.StartIndex, grouped)
}

func (e *EmbyService) loadCollapsedMediaPage(ctx context.Context, selected *gorm.DB, selectedOrder string, startIndex int, grouped *gorm.DB) ([]model.Media, int64, error) {
	var page []embyCollapsedMediaPageRow
	if err := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Joins("JOIN (?) AS selected ON selected.id = media.id", selected).
		Select("media.*, selected.total_groups").Order(selectedOrder).Scan(&page).Error; err != nil {
		return nil, 0, err
	}
	total := int64(0)
	if len(page) > 0 {
		total = page[0].TotalGroups
	} else if startIndex > 0 {
		if err := e.repo.DB.WithContext(ctx).Table("(?) AS grouped", grouped).Count(&total).Error; err != nil {
			return nil, 0, err
		}
	}
	rows := make([]model.Media, len(page))
	for i := range page {
		rows[i] = page[i].Media
	}
	return rows, total, nil
}

func embySortDirection(descending bool) string {
	if descending {
		return "DESC"
	}
	return "ASC"
}

// embyLibraryGroupSQL reproduces mediaVersionKey's sorted merged-library
// identity for SQL grouping. It is request-local and comes from the same
// library snapshot used by the payload path, so no second grouping source is
// introduced.
func (e *EmbyService) embyLibraryGroupSQL(ctx context.Context) (string, []any) {
	snapshot, ok := embyLibrarySnapshotFromContext(ctx)
	if !ok || len(snapshot.libraries) == 0 {
		return "TRIM(media.library_id)", nil
	}
	libraries := append([]model.Library(nil), snapshot.libraries...)
	sort.Slice(libraries, func(i, j int) bool { return libraries[i].ID < libraries[j].ID })
	parts := make([]string, 0, len(libraries)+2)
	args := make([]any, 0, len(libraries)*2)
	parts = append(parts, "CASE media.library_id")
	for _, library := range libraries {
		id := strings.TrimSpace(library.ID)
		if id == "" {
			continue
		}
		ids := MergedLibraryIDs(snapshot.libraries, library)
		sort.Strings(ids)
		group := strings.Join(ids, ",")
		if group == "" {
			group = id
		}
		parts = append(parts, "WHEN ? THEN ?")
		args = append(args, id, group)
	}
	parts = append(parts, "ELSE TRIM(media.library_id) END")
	return strings.Join(parts, " "), args
}
