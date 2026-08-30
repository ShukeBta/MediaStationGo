// Package service — playback history / favourites / playlists.
//
// These three concerns are intentionally co-located: they all sit between
// "the user" and "a media item" and share the same join-table flavour. A
// dedicated PlaybackService keeps the wiring simple and lets handlers
// dispatch by feature instead of by repository.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// PlaybackService bundles history / favourite / playlist business logic.
type PlaybackService struct {
	log          *zap.Logger
	repo         *repository.Container
	resolvedMu   sync.Mutex
	resolvedPlay map[string]time.Time
	resolvedGCAt time.Time
}

var ErrCloudPlaybackNotResolved = errors.New("cloud playback direct link was not resolved successfully")

const (
	resolvedCloudPlaybackTTL      = 6 * time.Hour
	resolvedCloudPlaybackGCPeriod = 30 * time.Minute
)

// NewPlaybackService is the constructor.
func NewPlaybackService(log *zap.Logger, repo *repository.Container) *PlaybackService {
	return &PlaybackService{log: log, repo: repo, resolvedPlay: make(map[string]time.Time)}
}

// ─── History ────────────────────────────────────────────────────────────────

// RecordProgress upserts the resume position for a (user, media) pair. A
// position within 30 seconds of the duration auto-flags the item as
// completed so the home page can hide it from "Continue Watching".
func (p *PlaybackService) RecordProgress(ctx context.Context, userID, mediaID string, position, duration int64) error {
	if userID == "" || mediaID == "" {
		return errors.New("missing user or media")
	}
	if err := p.ValidateProgressWrite(ctx, userID, mediaID); err != nil {
		return err
	}
	completed := duration > 0 && position >= duration-30_000
	h := &model.PlaybackHistory{
		UserID:     userID,
		MediaID:    mediaID,
		PositionMs: position,
		DurationMs: duration,
		WatchedAt:  time.Now(),
		Completed:  completed,
	}
	if err := p.repo.History.Upsert(ctx, h); err != nil {
		return err
	}
	// 标准行为：被移出继续观看的条目再次观看时自动恢复。
	return p.repo.MediaPlaybackPreference.ClearHiddenFromResume(ctx, userID, mediaID)
}

func (p *PlaybackService) AuthorizeResolvedCloudPlayback(userID, mediaID string) {
	if p == nil {
		return
	}
	key := resolvedCloudPlaybackKey(userID, mediaID)
	if key == "" {
		return
	}
	now := time.Now()
	p.resolvedMu.Lock()
	if p.resolvedPlay == nil {
		p.resolvedPlay = make(map[string]time.Time)
	}
	if !p.resolvedGCAt.After(now) {
		for existing, expiresAt := range p.resolvedPlay {
			if !expiresAt.After(now) {
				delete(p.resolvedPlay, existing)
			}
		}
		p.resolvedGCAt = now.Add(resolvedCloudPlaybackGCPeriod)
	}
	p.resolvedPlay[key] = now.Add(resolvedCloudPlaybackTTL)
	p.resolvedMu.Unlock()
}

func (p *PlaybackService) ValidateProgressWrite(ctx context.Context, userID, mediaID string) error {
	if p == nil || p.repo == nil || p.repo.Media == nil {
		return errors.New("playback service unavailable")
	}
	if resolvedCloudPlaybackKey(userID, mediaID) == "" {
		return errors.New("missing user or media")
	}
	if p.hasResolvedCloudPlayback(userID, mediaID) {
		return nil
	}
	m, err := p.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return err
	}
	if m == nil {
		return errors.New("media not found")
	}
	if !cloudBackedPlaybackMedia(m) {
		return nil
	}
	return ErrCloudPlaybackNotResolved
}

func (p *PlaybackService) hasResolvedCloudPlayback(userID, mediaID string) bool {
	key := resolvedCloudPlaybackKey(userID, mediaID)
	if key == "" {
		return false
	}
	now := time.Now()
	p.resolvedMu.Lock()
	defer p.resolvedMu.Unlock()
	expiresAt, ok := p.resolvedPlay[key]
	if !ok {
		return false
	}
	if !expiresAt.After(now) {
		delete(p.resolvedPlay, key)
		return false
	}
	return true
}

func resolvedCloudPlaybackKey(userID, mediaID string) string {
	userID = strings.TrimSpace(userID)
	mediaID = strings.TrimSpace(mediaID)
	if userID == "" || mediaID == "" {
		return ""
	}
	return userID + "\x00" + mediaID
}

func cloudBackedPlaybackMedia(m *model.Media) bool {
	if m == nil {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://") {
		return true
	}
	_, _, ok := parseCloudMediaPlaybackURL(m.STRMURL)
	return ok
}

// HistoryItem joins the playback row with its media so the API consumer
// gets a fully-populated card without a second round-trip.
type HistoryItem struct {
	model.PlaybackHistory
	Media *model.Media `json:"media,omitempty"`
}

// RecentHistory returns the most recently-watched items for a user. We
// fetch the history rows first then attach each Media row in a single
// follow-up query.
func (p *PlaybackService) RecentHistory(ctx context.Context, userID string, limit int) ([]HistoryItem, error) {
	rows, err := p.repo.History.ListByUser(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	return p.hydrateHistoryItems(ctx, rows)
}

// RecentHistoryPage returns one visibility-aware page and its exact total.
func (p *PlaybackService) RecentHistoryPage(
	ctx context.Context,
	userID string,
	page int,
	pageSize int,
	visibility MediaVisibility,
) ([]HistoryItem, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := p.repo.DB.WithContext(ctx).Model(&model.PlaybackHistory{}).
		Joins("JOIN media ON media.id = playback_histories.media_id AND media.deleted_at IS NULL").
		Where("playback_histories.user_id = ?", userID)
	if !visibility.IncludeNSFW {
		query = query.Where("media.nsfw = ?", false)
	}
	if len(visibility.HiddenLibraryIDs) > 0 {
		query = query.Where("media.library_id NOT IN ?", visibility.HiddenLibraryIDs)
	}
	if len(visibility.AllowedLibraryIDs) > 0 {
		query = query.Where("media.library_id IN ?", visibility.AllowedLibraryIDs)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.PlaybackHistory
	if err := query.Select("playback_histories.*").
		Order("playback_histories.watched_at DESC, playback_histories.updated_at DESC, playback_histories.id DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	items, err := p.hydrateHistoryItems(ctx, rows)
	return items, total, err
}

func (p *PlaybackService) hydrateHistoryItems(ctx context.Context, rows []model.PlaybackHistory) ([]HistoryItem, error) {
	mediaIDs := make([]string, 0, len(rows))
	for i := range rows {
		if rows[i].MediaID != "" {
			mediaIDs = append(mediaIDs, rows[i].MediaID)
		}
	}
	mediaByID := map[string]model.Media{}
	if len(mediaIDs) > 0 {
		var mediaRows []model.Media
		if err := p.repo.DB.WithContext(ctx).Where("id IN ?", mediaIDs).Find(&mediaRows).Error; err == nil {
			for _, media := range mediaRows {
				mediaByID[media.ID] = media
			}
		}
	}
	items := make([]HistoryItem, 0, len(rows))
	for i := range rows {
		if m, ok := mediaByID[rows[i].MediaID]; ok {
			media := m
			items = append(items, HistoryItem{PlaybackHistory: rows[i], Media: &media})
		} else {
			items = append(items, HistoryItem{PlaybackHistory: rows[i]})
		}
	}
	return items, nil
}

// ─── Favourites ─────────────────────────────────────────────────────────────

// ToggleFavourite flips the favourite flag and reports the new state.
func (p *PlaybackService) ToggleFavourite(ctx context.Context, userID, mediaID string) (bool, error) {
	return p.repo.Favorite.Toggle(ctx, userID, mediaID)
}

// ListFavourites returns every favourited media for a user.
func (p *PlaybackService) ListFavourites(ctx context.Context, userID string) ([]model.Media, error) {
	favs, err := p.repo.Favorite.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(favs) == 0 {
		return nil, nil
	}
	ids := make([]string, len(favs))
	for i, f := range favs {
		ids[i] = f.MediaID
	}
	var out []model.Media
	err = p.repo.DB.Where("id IN ?", ids).
		Order("created_at desc").Find(&out).Error
	return out, err
}

// ─── Playlists ──────────────────────────────────────────────────────────────

// CreatePlaylist persists a new playlist owned by userID.
func (p *PlaybackService) CreatePlaylist(ctx context.Context, userID, name string, isPublic bool) (*model.Playlist, error) {
	if name == "" {
		return nil, errors.New("name required")
	}
	pl := &model.Playlist{UserID: userID, Name: name, IsPublic: isPublic}
	if err := p.repo.Playlist.Create(ctx, pl); err != nil {
		return nil, err
	}
	return pl, nil
}

// ListPlaylists returns every playlist owned by userID.
func (p *PlaybackService) ListPlaylists(ctx context.Context, userID string) ([]model.Playlist, error) {
	return p.repo.Playlist.ListByUser(ctx, userID)
}

// PlaylistDetail returns the playlist together with its ordered media items.
type PlaylistDetail struct {
	Playlist model.Playlist `json:"playlist"`
	Items    []model.Media  `json:"items"`
}

// GetPlaylist returns the playlist + its ordered media. Visibility is
// enforced at the handler level; the service trusts callers.
func (p *PlaybackService) GetPlaylist(ctx context.Context, playlistID string) (*PlaylistDetail, error) {
	var pl model.Playlist
	if err := p.repo.DB.Where("id = ?", playlistID).First(&pl).Error; err != nil {
		return nil, err
	}
	var rows []model.PlaylistItem
	if err := p.repo.DB.
		Where("playlist_id = ?", playlistID).
		Order("position asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return &PlaylistDetail{Playlist: pl}, nil
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.MediaID
	}
	var media []model.Media
	if err := p.repo.DB.Where("id IN ?", ids).Find(&media).Error; err != nil {
		return nil, err
	}
	// Preserve playlist order.
	byID := make(map[string]model.Media, len(media))
	for _, m := range media {
		byID[m.ID] = m
	}
	ordered := make([]model.Media, 0, len(rows))
	for _, r := range rows {
		if m, ok := byID[r.MediaID]; ok {
			ordered = append(ordered, m)
		}
	}
	return &PlaylistDetail{Playlist: pl, Items: ordered}, nil
}

// AddToPlaylist appends a media item to the end of a playlist.
func (p *PlaybackService) AddToPlaylist(ctx context.Context, playlistID, mediaID string) error {
	var count int64
	if err := p.repo.DB.Model(&model.PlaylistItem{}).
		Where("playlist_id = ?", playlistID).Count(&count).Error; err != nil {
		return err
	}
	item := &model.PlaylistItem{
		PlaylistID: playlistID,
		MediaID:    mediaID,
		Position:   int(count) + 1,
	}
	return p.repo.DB.Create(item).Error
}

// RemoveFromPlaylist removes a media item from a playlist (idempotent).
func (p *PlaybackService) RemoveFromPlaylist(ctx context.Context, playlistID, mediaID string) error {
	return p.repo.DB.
		Where("playlist_id = ? AND media_id = ?", playlistID, mediaID).
		Delete(&model.PlaylistItem{}).Error
}

// DeletePlaylist removes a playlist and all of its items.
func (p *PlaybackService) DeletePlaylist(ctx context.Context, playlistID string) error {
	if err := p.repo.DB.Where("playlist_id = ?", playlistID).
		Delete(&model.PlaylistItem{}).Error; err != nil {
		return err
	}
	return p.repo.DB.Where("id = ?", playlistID).Delete(&model.Playlist{}).Error
}
