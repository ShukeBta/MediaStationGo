// Package service — duplicate-file finder.
//
// DuplicateService finds duplicate media by two signals:
//
//   - external identity: same TMDb / Bangumi / Douban / TheTVDB id and, for
//     episodes, same season+episode;
//   - sparse file hash: same head + middle + tail SHA-256 and same size.
//
// The first row (preferring scraped + larger files) is kept as the primary;
// the rest get is_duplicate = true and duplicate_of pointing at the primary.
//
// Why sparse: a full hash on a 50 GB Blu-ray remux takes minutes; the
// 3-window 3 MiB sample is enough to differentiate real-world copies
// while finishing per-file in well under a second.
package service

import (
	"context"
	"os"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// DuplicateService is the entry point for the duplicate finder.
type DuplicateService struct {
	log  *zap.Logger
	repo *repository.Container
	hub  *Hub
}

// NewDuplicateService is the constructor.
func NewDuplicateService(log *zap.Logger, repo *repository.Container, hub *Hub) *DuplicateService {
	return &DuplicateService{log: log, repo: repo, hub: hub}
}

// Group describes one set of duplicates returned by Detect.
type Group struct {
	Hash       string        `json:"hash"`
	Primary    model.Media   `json:"primary"`
	Duplicates []model.Media `json:"duplicates"`
}

// Report is the summary the React UI displays.
type Report struct {
	TotalScanned   int     `json:"total_scanned"`
	GroupsFound    int     `json:"groups_found"`
	ItemsMarked    int     `json:"items_marked"`
	MissingRemoved int64   `json:"missing_removed"`
	Groups         []Group `json:"groups"`
}

// Detect walks every media row in the given library (or all libraries
// when libraryID is empty), computes a hash for the ones missing it,
// then groups by hash and marks duplicates in the DB.
func (d *DuplicateService) Detect(ctx context.Context, libraryID string) (*Report, error) {
	var rows []model.Media
	q := d.repo.DB.WithContext(ctx).Model(&model.Media{})
	if libraryID != "" {
		q = q.Where("library_id = ?", libraryID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}

	rep := &Report{Groups: []Group{}}
	rows = d.removeMissingRows(ctx, rows, rep)
	rep.TotalScanned = len(rows)
	totalToHash := 0
	for i := range rows {
		if rows[i].FileHash == "" && rows[i].Path != "" {
			totalToHash++
		}
	}

	hashed := 0
	for i := range rows {
		select {
		case <-ctx.Done():
			return rep, ctx.Err()
		default:
		}
		if rows[i].FileHash != "" || rows[i].Path == "" {
			continue
		}
		h, err := SparseFileHash(rows[i].Path)
		if err != nil {
			d.log.Debug("hash failed", zap.String("path", rows[i].Path), zap.Error(err))
			continue
		}
		rows[i].FileHash = h
		if err := d.repo.DB.WithContext(ctx).
			Model(&model.Media{}).
			Where("id = ?", rows[i].ID).
			Update("file_hash", h).Error; err != nil {
			d.log.Warn("hash persist failed", zap.Error(err))
		}
		hashed++
		if d.hub != nil && totalToHash > 0 {
			d.hub.Publish("duplicate", map[string]any{
				"hashed":  hashed,
				"total":   totalToHash,
				"current": rows[i].Title,
			})
		}
	}

	// Group rows by file_hash.
	groups := make(map[string][]model.Media)
	for _, r := range rows {
		if r.FileHash == "" {
			continue
		}
		groups[r.FileHash] = append(groups[r.FileHash], r)
	}

	markedIDs := map[string]struct{}{}
	for hash, group := range groups {
		if len(group) < 2 {
			continue
		}
		d.markDuplicateGroup(ctx, rep, hash, group, markedIDs)
	}

	for key, group := range groupByExternalIdentity(rows) {
		if len(group) < 2 {
			continue
		}
		d.markDuplicateGroup(ctx, rep, key, group, markedIDs)
	}
	rep.GroupsFound = len(rep.Groups)
	if d.hub != nil {
		d.hub.Publish("duplicate", map[string]any{
			"finished": true,
			"groups":   rep.GroupsFound,
			"marked":   rep.ItemsMarked,
		})
	}
	return rep, nil
}

// Current returns duplicate groups already marked in the database. It keeps
// the UI useful after a prior scan and avoids requiring POST on page load.
func (d *DuplicateService) Current(ctx context.Context, libraryID string) (*Report, error) {
	var rows []model.Media
	q := d.repo.DB.WithContext(ctx).Where("is_duplicate = ? OR duplicate_of <> ''", true)
	if libraryID != "" {
		q = q.Where("library_id = ?", libraryID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	rep := &Report{TotalScanned: len(rows), Groups: []Group{}}
	byPrimary := make(map[string][]model.Media)
	for _, row := range rows {
		if row.DuplicateOf == "" {
			continue
		}
		byPrimary[row.DuplicateOf] = append(byPrimary[row.DuplicateOf], row)
	}
	for primaryID, dupes := range byPrimary {
		primary, err := d.repo.Media.FindByID(ctx, primaryID)
		if err != nil || primary == nil {
			continue
		}
		hash := primary.FileHash
		if hash == "" && len(dupes) > 0 {
			hash = dupes[0].FileHash
		}
		rep.Groups = append(rep.Groups, Group{
			Hash:       hash,
			Primary:    *primary,
			Duplicates: dupes,
		})
	}
	rep.GroupsFound = len(rep.Groups)
	return rep, nil
}

func (d *DuplicateService) removeMissingRows(ctx context.Context, rows []model.Media, rep *Report) []model.Media {
	kept := make([]model.Media, 0, len(rows))
	for _, row := range rows {
		if row.Path == "" {
			kept = append(kept, row)
			continue
		}
		if _, err := os.Stat(row.Path); err == nil {
			kept = append(kept, row)
			continue
		} else if !os.IsNotExist(err) {
			kept = append(kept, row)
			continue
		}
		res := d.repo.DB.WithContext(ctx).Where("id = ?", row.ID).Delete(&model.Media{})
		if res.Error != nil {
			d.log.Warn("remove missing duplicate candidate failed", zap.String("media", row.ID), zap.Error(res.Error))
			continue
		}
		rep.MissingRemoved += res.RowsAffected
	}
	return kept
}

// Unmark clears the is_duplicate flag for every row in the given library
// (or all when libraryID is empty). Useful when the operator deletes the
// physical duplicates manually.
func (d *DuplicateService) Unmark(ctx context.Context, libraryID string) (int64, error) {
	q := d.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("is_duplicate = ?", true)
	if libraryID != "" {
		q = q.Where("library_id = ?", libraryID)
	}
	res := q.Updates(map[string]any{"is_duplicate": false, "duplicate_of": ""})
	return res.RowsAffected, res.Error
}
