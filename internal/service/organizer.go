// Package service — media file organizer (auto-rename + move).
//
// OrganizerService takes a source file (typically from a download
// completion) and moves/renames it into the configured library directory
// using a Jinja2-like template. The default templates:
//
//	movie: {Title} ({Year})/{Title} ({Year}).{Ext}
//	tv:    {Title}/Season {Season:02d}/{Title} - S{Season:02d}E{Episode:02d}.{Ext}
//
// Organizing moves the source file. On the same filesystem this is an
// os.Rename; across filesystems it falls back to streaming copy + source
// removal, so operators should keep downloads and media on the same volume
// when they want instant moves and no temporary duplicate disk usage.
package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// OrganizerService moves/renames files into library structures.
type OrganizerService struct {
	cfg                 *config.Config
	log                 *zap.Logger
	repo                *repository.Container
	probe               *FFprobeService // optional; used for 洗版 resolution comparison
	scraper             *ScraperService // optional; used to identify metadata before rename
	activeDownloadPaths func(context.Context) []string
}

// NewOrganizerService is the constructor.
func NewOrganizerService(cfg *config.Config, log *zap.Logger, repo *repository.Container) *OrganizerService {
	return &OrganizerService{cfg: cfg, log: log, repo: repo}
}

// SetProbe wires an FFprobe service so directory organize can compare real
// pixel dimensions when deciding whether to 洗版 (replace by higher resolution).
// Optional: when nil the organizer falls back to filename resolution tokens.
func (o *OrganizerService) SetProbe(p *FFprobeService) { o.probe = p }

// SetScraper wires the scraper so directory organize can resolve TMDb/Bangumi
// metadata before it decides the final folder and filename.
func (o *OrganizerService) SetScraper(s *ScraperService) { o.scraper = s }

// SetActiveDownloadPathProvider wires a live downloader snapshot. Directory
// organize must never move/copy files that still belong to an unfinished
// torrent, regardless of which UI switch triggered the organize operation.
func (o *OrganizerService) SetActiveDownloadPathProvider(provider func(context.Context) []string) {
	o.activeDownloadPaths = provider
}

func (o *OrganizerService) SetActiveDownloadProvider(provider func(context.Context) []QBitTorrent) {
	if provider == nil {
		o.activeDownloadPaths = nil
		return
	}
	o.activeDownloadPaths = func(ctx context.Context) []string {
		return activeDownloadPathCandidates(provider(ctx), nil)
	}
}

// OrganizeMedia moves a single media file into the target library directory.
// It auto-detects whether the media is a movie or TV episode based on the
// parsed season/episode numbers and builds the destination path accordingly.
// When smart classify is enabled, it adds a category subfolder (e.g., "华语电影").
func (o *OrganizerService) OrganizeMedia(ctx context.Context, mediaID string) (string, error) {
	return o.OrganizeMediaWithOptions(ctx, mediaID, OrganizeOptions{})
}

// OrganizeMediaWithOptions is OrganizeMedia with per-request overrides for the
// target path and transfer mode.
func (o *OrganizerService) OrganizeMediaWithOptions(ctx context.Context, mediaID string, opts OrganizeOptions) (string, error) {
	req, err := o.resolveOrganizeMediaRequest(ctx, mediaID, opts)
	if err != nil {
		return "", err
	}
	dst, err := o.buildOrganizeMediaDestination(ctx, req)
	if err != nil {
		return "", err
	}

	// Skip if already in place.
	if req.media.Path == dst.path {
		if !req.dryRun {
			if err := o.persistOrganizedMediaMetadata(ctx, req.media); err != nil {
				return "", err
			}
		}
		return dst.path, nil
	}
	if req.dryRun {
		return dst.path, nil
	}

	return o.applyOrganizeMedia(ctx, req, dst)
}

// SyncMediaPathWithMetadata renames an already-organized media file after
// metadata scraping has finalized the display title/year. It always moves the
// organized target itself, even when the ingest transfer mode was copy/link.
func (o *OrganizerService) SyncMediaPathWithMetadata(ctx context.Context, mediaID string, opts OrganizeOptions) (string, error) {
	req, err := o.resolveOrganizeMediaRequest(ctx, mediaID, opts)
	if err != nil {
		return "", err
	}
	dst, err := o.buildOrganizeMediaDestination(ctx, req)
	if err != nil {
		return "", err
	}
	if samePath(req.media.Path, dst.path) {
		if !req.dryRun {
			if err := o.persistOrganizedMediaMetadata(ctx, req.media); err != nil {
				return "", err
			}
		}
		return dst.path, nil
	}
	if req.dryRun {
		return dst.path, nil
	}
	req.transferMode = TransferMove
	before := req.media.Path
	after, err := o.applyOrganizeMedia(ctx, req, dst)
	if err == nil {
		cleanupEmptyOrganizeParents(before, req.baseRoot)
	}
	return after, err
}

// OrganizeLibrary organizes every media row in a library whose file is
// not already in the expected path structure.
func (o *OrganizerService) OrganizeLibrary(ctx context.Context, libraryID string) (*OrganizeResult, error) {
	return o.OrganizeLibraryWithOptions(ctx, libraryID, OrganizeOptions{})
}

// OrganizeLibraryWithOptions is OrganizeLibrary with per-request overrides for
// the target path and transfer mode.
func (o *OrganizerService) OrganizeLibraryWithOptions(ctx context.Context, libraryID string, opts OrganizeOptions) (*OrganizeResult, error) {
	lib, err := o.repo.Library.FindByID(ctx, libraryID)
	if err != nil || lib == nil {
		return nil, errors.New("library not found")
	}
	if _, ok := ParseCloudLibraryMount(lib.Path); ok {
		return nil, errors.New("local organize cannot use cloud libraries directly; use external storage scan/mount for cloud media or enable cloud transfer to write to cloud")
	}
	var rows []model.Media
	if err := o.repo.DB.WithContext(ctx).
		Where("library_id = ? AND deleted_at IS NULL", libraryID).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	// 源目录（待整理）：仅整理位于该目录下的媒体；留空 = 整个媒体库。
	sourceRoot := o.resolveSourceRoot(ctx, lib, opts.SourcePath)
	if _, ok := ParseCloudLibraryMount(sourceRoot); ok {
		return nil, errors.New("organize source must be a local directory; cloud libraries should be managed from external storage scan/mount")
	}
	// 目的地目录：已位于该根下的文件视为已整理；受 dest_path 覆盖与设置影响。
	baseRoot := normalizeMappedOrganizeDestinationRoot(o.resolveBaseRoot(ctx, lib, opts.DestPath))
	if _, ok := ParseCloudLibraryMount(baseRoot); ok {
		return nil, errors.New("organize destination must be a local writable media directory; enable cloud transfer in external storage when writing to cloud")
	}
	if !opts.DryRun {
		if err := ensureOrganizeDestinationWritable(baseRoot); err != nil {
			return nil, err
		}
	}
	res := &OrganizeResult{SourcePath: sourceRoot, DestPath: baseRoot, DryRun: opts.DryRun}
	for i := range rows {
		if changed, err := o.reclassifyScannedMedia(ctx, rows[i], *lib, "", opts, opts.DryRun, res); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", rows[i].Title, err.Error()))
			continue
		} else if changed {
			continue
		}
		// 不在源目录内的文件跳过（不属于本次「从源目录整理」的范围）。
		if !pathWithin(rows[i].Path, sourceRoot) {
			res.Skipped++
			continue
		}
		if pathWithin(rows[i].Path, baseRoot) {
			res.Skipped++
			continue
		}
		dst, err := o.OrganizeMediaWithOptions(ctx, rows[i].ID, opts)
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", rows[i].Title, err.Error()))
			continue
		}
		if dst == rows[i].Path {
			res.Skipped++
		} else {
			res.Organized++
		}
	}
	return res, nil
}

func (o *OrganizerService) refreshEpisodeIdentity(m *model.Media, lib *model.Library) error {
	season, episode := m.SeasonNum, m.EpisodeNum

	if parsedSeason, parsedEpisode := ParseEpisode(m.Path); parsedSeason > 0 || parsedEpisode > 0 {
		season = parsedSeason
		if parsedEpisode > 0 {
			episode = parsedEpisode
		}
	}

	if local, err := ReadLocalMetadata(m.Path, lib.Path, true); err == nil && local != nil {
		if local.SeasonNum > 0 || local.EpisodeNum > 0 {
			season = local.SeasonNum
		}
		if local.EpisodeNum > 0 {
			episode = local.EpisodeNum
		}
	} else if err != nil {
		o.log.Warn("organize read local metadata failed", zap.String("path", m.Path), zap.Error(err))
	}

	if season < 0 || episode <= 0 {
		return fmt.Errorf("cannot determine season/episode for %s", m.Path)
	}
	m.SeasonNum = season
	m.EpisodeNum = episode
	return nil
}
