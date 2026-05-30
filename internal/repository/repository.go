// Package repository 实现基于 GORM 的数据访问层。
// 每个方法接受 context.Context 以便后续插入取消/追踪。
//
// Repository 故意保持精简：它们只负责持久化数据，不处理业务逻辑。
// 业务逻辑位于 internal/service。
package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// Container 是所有 repositories 的注册表，注入到 services 中。
type Container struct {
	DB             *gorm.DB
	User           *UserRepository
	Library        *LibraryRepository
	Media          *MediaRepository
	Series         *SeriesRepository
	History        *HistoryRepository
	Favorite       *FavoriteRepository
	Playlist       *PlaylistRepository
	Download       *DownloadRepository
	Subscription   *SubscriptionRepository
	Setting        *SettingRepository
	Log            *AccessLogRepository
	Permission     *PermissionRepository
	RefreshToken   *RefreshTokenRepository
	ApiConfig      *ApiConfigRepository
	DownloadClient *DownloadClientRepository
	NotifyChannel  *NotifyChannelRepository
	Site           *SiteRepository
	STRM           *STRMRepository
	PlayProfile    *PlayProfileRepository
	StorageConfig  *StorageConfigRepository
	Assistant      *AssistantRepository
}

// New 将每个 repository 连接到单个 *gorm.DB。
func New(db *gorm.DB) *Container {
	return &Container{
		DB:             db,
		User:           &UserRepository{db: db},
		Library:        &LibraryRepository{db: db},
		Media:          &MediaRepository{db: db},
		Series:         &SeriesRepository{db: db},
		History:        &HistoryRepository{db: db},
		Favorite:       &FavoriteRepository{db: db},
		Playlist:       &PlaylistRepository{db: db},
		Download:       &DownloadRepository{db: db},
		Subscription:   &SubscriptionRepository{db: db},
		Setting:        &SettingRepository{db: db},
		Log:            &AccessLogRepository{db: db},
		Permission:     &PermissionRepository{db: db},
		RefreshToken:   &RefreshTokenRepository{db: db},
		ApiConfig:      &ApiConfigRepository{db: db},
		DownloadClient: &DownloadClientRepository{db: db},
		NotifyChannel:  &NotifyChannelRepository{db: db},
		Site:           &SiteRepository{db: db},
		STRM:           &STRMRepository{db: db},
		PlayProfile:    &PlayProfileRepository{db: db},
		StorageConfig:  &StorageConfigRepository{db: db},
		Assistant:      &AssistantRepository{db: db},
	}
}

// ─── User ────────────────────────────────────────────────────────────────────

// UserRepository persists model.User records.
type UserRepository struct{ db *gorm.DB }

// Create inserts a new user. Caller must pre-hash the password.
func (r *UserRepository) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

// ReleaseDeletedUsername renames soft-deleted rows that still hold a unique
// username so the same account name can be created again.
func (r *UserRepository) ReleaseDeletedUsername(ctx context.Context, username string) error {
	if username == "" {
		return nil
	}
	released := username + "__deleted__" + time.Now().Format("20060102150405.000000000")
	if len(released) > 64 {
		sum := sha256.Sum256([]byte(released))
		released = username
		if len(released) > 43 {
			released = released[:43]
		}
		released += "__deleted__" + hex.EncodeToString(sum[:])[:10]
	}
	return r.db.WithContext(ctx).Unscoped().
		Model(&model.User{}).
		Where("username = ? AND deleted_at IS NOT NULL", username).
		Update("username", released).Error
}

// FindByUsername returns the user matching username, or (nil, nil) when absent.
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByID returns the user with the matching primary key, or (nil, nil).
func (r *UserRepository) FindByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Count returns the total number of non-deleted users.
func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Count(&n).Error
	return n, err
}

// CountAdmins returns the number of users that hold the admin role.
func (r *UserRepository) CountAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.User{}).
		Where("role = ?", "admin").Count(&n).Error
	return n, err
}

// FirstAdmin returns the earliest admin user. This row represents the protected
// built-in/default administrator even if its username is later changed.
func (r *UserRepository) FirstAdmin(ctx context.Context) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("role = ?", "admin").Order("created_at asc").First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns all users ordered by creation time desc.
func (r *UserRepository) List(ctx context.Context) ([]model.User, error) {
	var users []model.User
	err := r.db.WithContext(ctx).Order("created_at desc").Find(&users).Error
	return users, err
}

// UpdateFields applies a narrow set of user field updates.
func (r *UserRepository) UpdateFields(ctx context.Context, id string, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Updates(updates).Error
}

// UpdatePassword sets a new password hash and clears ForcePasswordReset.
func (r *UserRepository) UpdatePassword(ctx context.Context, id, hash string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Updates(map[string]any{"password_hash": hash, "force_password_reset": false}).Error
}

// TouchLogin updates the last login timestamp.
func (r *UserRepository) TouchLogin(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).
		Update("last_login_at", &now).Error
}

// Delete removes a user (soft-delete via gorm.DeletedAt), releases the unique
// username, and drops Telegram bindings so future re-created users bind cleanly.
func (r *UserRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Where("id = ?", id).First(&user).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("user_id = ?", id).Delete(&model.TelegramBinding{}).Error; err != nil {
			return err
		}
		released := user.Username + "__deleted__" + time.Now().Format("20060102150405.000000000")
		if len(released) > 64 {
			sum := sha256.Sum256([]byte(user.ID + user.Username))
			base := user.Username
			if len(base) > 43 {
				base = base[:43]
			}
			released = base + "__deleted__" + hex.EncodeToString(sum[:])[:10]
		}
		if err := tx.Model(&model.User{}).Where("id = ?", id).Update("username", released).Error; err != nil {
			return err
		}
		return tx.Delete(&model.User{}, "id = ?", id).Error
	})
}

// ─── Library ─────────────────────────────────────────────────────────────────

// LibraryRepository persists model.Library records.
type LibraryRepository struct{ db *gorm.DB }

// Create persists a new library row.
func (r *LibraryRepository) Create(ctx context.Context, l *model.Library) error {
	return r.db.WithContext(ctx).Create(l).Error
}

// List returns all enabled+disabled libraries.
func (r *LibraryRepository) List(ctx context.Context) ([]model.Library, error) {
	var ls []model.Library
	err := r.db.WithContext(ctx).Order("created_at asc").Find(&ls).Error
	return ls, err
}

// FindByID returns the library, or (nil, nil) when missing.
func (r *LibraryRepository) FindByID(ctx context.Context, id string) (*model.Library, error) {
	var l model.Library
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&l).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// Delete removes a library and (soft) cascades to its media via repository
// callers — we do not run CASCADE here to keep this method narrow.
func (r *LibraryRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Library{}, "id = ?", id).Error
}

// ─── Media ───────────────────────────────────────────────────────────────────

// MediaRepository persists model.Media records.
type MediaRepository struct{ db *gorm.DB }

// MediaQueryFilter is applied to user-facing media queries so NSFW items and
// profile-restricted libraries are filtered in SQL instead of only in React.
type MediaQueryFilter struct {
	IncludeNSFW       bool
	AllowedLibraryIDs []string
	HiddenLibraryIDs  []string
}

func applyMediaQueryFilter(q *gorm.DB, filter MediaQueryFilter) *gorm.DB {
	if !filter.IncludeNSFW {
		q = q.Where("nsfw = ?", false)
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		q = q.Where("library_id NOT IN ?", filter.HiddenLibraryIDs)
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		q = q.Where("library_id IN ?", filter.AllowedLibraryIDs)
	}
	return q
}

// Upsert inserts or updates a media row keyed by Path (unique index).
//
// 重要：当一条行已经存在时，scanner 重扫只应该刷新文件级元数据
// （时长、宽高、编码、容器、大小），不能把刮削器维护的字段（标题改写、
// 海报、TMDb/Bangumi ID、scrape_status 等）覆盖回零值。
//
// 之前用 Assign(*m).FirstOrCreate(m) 会把整张零值结构体写回，导致：
//  1. scrape_status 从 'matched' / 'no_match' 被清空成 ”；
//  2. 新建行使 GORM `default:pending` 也得不到应用（因为 zero value 被
//     显式写入）。这两个问题都让 EnrichLibrary(WHERE scrape_status='pending')
//     永远捞不到数据。
func (r *MediaRepository) Upsert(ctx context.Context, m *model.Media) error {
	var existing model.Media
	err := r.db.WithContext(ctx).Unscoped().Where("path = ?", m.Path).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 新行：保证 scrape_status 走 GORM default:pending（即留空让数据库填）。
		if m.ScrapeStatus == "" {
			m.ScrapeStatus = "pending"
		}
		return r.db.WithContext(ctx).Create(m).Error
	}
	if err != nil {
		return err
	}

	// 已存在：仅刷新文件层面的字段。
	updates := map[string]any{
		"size_bytes":   m.SizeBytes,
		"duration_sec": m.DurationSec,
		"width":        m.Width,
		"height":       m.Height,
		"video_codec":  m.VideoCodec,
		"audio_codec":  m.AudioCodec,
		"container":    m.Container,
		"deleted_at":   nil,
	}
	// 回填硬链接身份标识，便于后续扫描去重（避免重复识别/多倍占用）。
	if m.FileID != "" && m.FileID != existing.FileID {
		updates["file_id"] = m.FileID
	}
	if m.Title != "" {
		// scanner 给出的标题只是从路径推导，刮削后 title 已被替换为
		// 真实剧名。仅在 existing 还停留在 'pending'/'' 时回填扫描标题，
		// 避免覆盖刮削结果。
		if m.ScrapeStatus == "matched" || existing.ScrapeStatus == "pending" || existing.ScrapeStatus == "" || existing.ScrapeStatus == "no_match" {
			updates["title"] = m.Title
			if m.Year > 0 {
				updates["year"] = m.Year
			}
		}
	}
	if m.ScrapeStatus == "matched" {
		updates["scrape_status"] = m.ScrapeStatus
		if m.OriginalName != "" {
			updates["original_name"] = m.OriginalName
		}
		if m.PosterURL != "" {
			updates["poster_url"] = m.PosterURL
		}
		if m.BackdropURL != "" {
			updates["backdrop_url"] = m.BackdropURL
		}
		if m.Overview != "" {
			updates["overview"] = m.Overview
		}
		if m.Rating > 0 {
			updates["rating"] = m.Rating
		}
		if m.Year > 0 {
			updates["year"] = m.Year
		}
		if m.TMDbID > 0 {
			updates["tm_db_id"] = m.TMDbID
		}
		if m.BangumiID > 0 {
			updates["bangumi_id"] = m.BangumiID
		}
		if m.Languages != "" {
			updates["languages"] = m.Languages
		}
		if m.Countries != "" {
			updates["countries"] = m.Countries
		}
		if m.Genres != "" {
			updates["genres"] = m.Genres
		}
		if m.NSFW {
			updates["nsfw"] = true
		}
	}
	if m.PosterURL != "" {
		updates["poster_url"] = m.PosterURL
	}
	if m.BackdropURL != "" {
		updates["backdrop_url"] = m.BackdropURL
	}
	if lib := m.LibraryID; lib != "" && lib != existing.LibraryID {
		updates["library_id"] = m.LibraryID
	}
	if m.SeasonNum > 0 && existing.SeasonNum != m.SeasonNum {
		updates["season_num"] = m.SeasonNum
	}
	if m.EpisodeNum > 0 && existing.EpisodeNum != m.EpisodeNum {
		updates["episode_num"] = m.EpisodeNum
	}
	if m.STRMURL != "" {
		updates["strm_url"] = m.STRMURL
	}

	if err := r.db.WithContext(ctx).Unscoped().Model(&model.Media{}).
		Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return err
	}
	// 回写 ID / 不可变字段，让 caller 拿到完整的现有行。
	*m = existing
	return nil
}

// FindByID returns the media row or (nil, nil).
func (r *MediaRepository) FindByID(ctx context.Context, id string) (*model.Media, error) {
	var m model.Media
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ListByLibrary returns paginated media items for a library.
func (r *MediaRepository) ListByLibrary(ctx context.Context, libraryID string, offset, limit int) ([]model.Media, int64, error) {
	return r.ListByLibraryFiltered(ctx, libraryID, offset, limit, MediaQueryFilter{IncludeNSFW: true})
}

func (r *MediaRepository) ListByLibraryFiltered(ctx context.Context, libraryID string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	var items []model.Media
	var total int64
	q := r.db.WithContext(ctx).Model(&model.Media{}).Where("library_id = ?", libraryID)
	q = applyMediaQueryFilter(q, filter)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("created_at desc").Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

// Search runs a LIKE search against the title field. Empty query returns the
// most recently added items.
func (r *MediaRepository) Search(ctx context.Context, query string, limit int) ([]model.Media, error) {
	return r.SearchFiltered(ctx, query, limit, MediaQueryFilter{IncludeNSFW: true})
}

func (r *MediaRepository) SearchFiltered(ctx context.Context, query string, limit int, filter MediaQueryFilter) ([]model.Media, error) {
	var items []model.Media
	q := r.db.WithContext(ctx).Model(&model.Media{}).Limit(limit)
	q = applyMediaQueryFilter(q, filter)
	if query != "" {
		like := "%" + query + "%"
		q = q.Where("title LIKE ? OR original_name LIKE ?", like, like)
	}
	err := q.Order("created_at desc").Find(&items).Error
	return items, err
}

// DeleteByLibrary purges all media tied to a library.
func (r *MediaRepository) DeleteByLibrary(ctx context.Context, libraryID string) error {
	return r.db.WithContext(ctx).Where("library_id = ?", libraryID).Delete(&model.Media{}).Error
}

// ─── Series ──────────────────────────────────────────────────────────────────

// SeriesRepository persists model.Series records.
type SeriesRepository struct{ db *gorm.DB }

// FindByID returns the series or (nil, nil).
func (r *SeriesRepository) FindByID(ctx context.Context, id string) (*model.Series, error) {
	var s model.Series
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// List returns all series (ordered by title).
func (r *SeriesRepository) List(ctx context.Context) ([]model.Series, error) {
	var s []model.Series
	err := r.db.WithContext(ctx).Order("title asc").Find(&s).Error
	return s, err
}

// ─── Playback History ────────────────────────────────────────────────────────

// HistoryRepository persists model.PlaybackHistory entries. The application
// upserts on (UserID, MediaID) so resume always reads the latest position.
type HistoryRepository struct{ db *gorm.DB }

// Upsert atomically inserts/updates the resume position.
func (r *HistoryRepository) Upsert(ctx context.Context, h *model.PlaybackHistory) error {
	var existing model.PlaybackHistory
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND media_id = ?", h.UserID, h.MediaID).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(h).Error
	}
	if err != nil {
		return err
	}
	existing.PositionMs = h.PositionMs
	existing.DurationMs = h.DurationMs
	existing.WatchedAt = h.WatchedAt
	existing.Completed = h.Completed
	return r.db.WithContext(ctx).Save(&existing).Error
}

// ListByUser returns the most recent history rows for the user.
func (r *HistoryRepository) ListByUser(ctx context.Context, userID string, limit int) ([]model.PlaybackHistory, error) {
	var rows []model.PlaybackHistory
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("watched_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}

// ─── Favorite ───────────────────────────────────────────────────────────────

// FavoriteRepository persists model.Favorite records.
type FavoriteRepository struct{ db *gorm.DB }

// Toggle flips the favourite flag for (user, media). Returns the new state.
func (r *FavoriteRepository) Toggle(ctx context.Context, userID, mediaID string) (bool, error) {
	var f model.Favorite
	err := r.db.WithContext(ctx).Where("user_id = ? AND media_id = ?", userID, mediaID).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fav := model.Favorite{UserID: userID, MediaID: mediaID}
		return true, r.db.WithContext(ctx).Create(&fav).Error
	}
	if err != nil {
		return false, err
	}
	return false, r.db.WithContext(ctx).Delete(&f).Error
}

// ListByUser returns all favourite media IDs for a user.
func (r *FavoriteRepository) ListByUser(ctx context.Context, userID string) ([]model.Favorite, error) {
	var rows []model.Favorite
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error
	return rows, err
}

// ─── Playlist ────────────────────────────────────────────────────────────────

// PlaylistRepository persists model.Playlist + PlaylistItem.
type PlaylistRepository struct{ db *gorm.DB }

// Create inserts a new playlist.
func (r *PlaylistRepository) Create(ctx context.Context, p *model.Playlist) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// ListByUser returns playlists owned by a user.
func (r *PlaylistRepository) ListByUser(ctx context.Context, userID string) ([]model.Playlist, error) {
	var rows []model.Playlist
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at desc").Find(&rows).Error
	return rows, err
}

// ─── Download ───────────────────────────────────────────────────────────────

// DownloadRepository persists model.DownloadTask records.
type DownloadRepository struct{ db *gorm.DB }

// Create inserts a new download task.
func (r *DownloadRepository) Create(ctx context.Context, t *model.DownloadTask) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// List returns all download tasks (admin view).
func (r *DownloadRepository) List(ctx context.Context) ([]model.DownloadTask, error) {
	var rows []model.DownloadTask
	err := r.db.WithContext(ctx).Order("created_at desc").Find(&rows).Error
	return rows, err
}

// ─── Subscription ───────────────────────────────────────────────────────────

// SubscriptionRepository persists model.Subscription records.
type SubscriptionRepository struct{ db *gorm.DB }

// Create inserts a new subscription rule.
func (r *SubscriptionRepository) Create(ctx context.Context, s *model.Subscription) error {
	return r.db.WithContext(ctx).Select("*").Omit("DeletedAt").Create(s).Error
}

// List returns all subscription rules.
func (r *SubscriptionRepository) List(ctx context.Context) ([]model.Subscription, error) {
	var rows []model.Subscription
	err := r.db.WithContext(ctx).Order("created_at desc").Find(&rows).Error
	return rows, err
}

// ─── Setting ─────────────────────────────────────────────────────────────────

// SettingRepository persists key/value preferences.
type SettingRepository struct{ db *gorm.DB }

// Get returns the value or empty string when absent.
func (r *SettingRepository) Get(ctx context.Context, key string) (string, error) {
	var s model.Setting
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return s.Value, err
}

// Set upserts a setting value.
func (r *SettingRepository) Set(ctx context.Context, key, value string) error {
	s := model.Setting{Key: key, Value: value, UpdatedAt: time.Now()}
	return r.db.WithContext(ctx).Save(&s).Error
}

// All returns every key/value pair (used by the admin UI).
func (r *SettingRepository) All(ctx context.Context) ([]model.Setting, error) {
	var rows []model.Setting
	err := r.db.WithContext(ctx).Find(&rows).Error
	return rows, err
}

// ─── Access Log ──────────────────────────────────────────────────────────────

// AccessLogRepository persists model.AccessLog records.
type AccessLogRepository struct{ db *gorm.DB }

// Create inserts one structured audit-trail entry.
func (r *AccessLogRepository) Create(ctx context.Context, l *model.AccessLog) error {
	return r.db.WithContext(ctx).Create(l).Error
}

// Recent returns the latest access-log entries (admin Activity panel).
func (r *AccessLogRepository) Recent(ctx context.Context, limit int) ([]model.AccessLog, error) {
	var rows []model.AccessLog
	err := r.db.WithContext(ctx).Order("created_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}

// ─── Permission ──────────────────────────────────────────────────────────────

// PermissionRepository persists model.UserPermission records.
type PermissionRepository struct{ db *gorm.DB }

// Create inserts a new permission record.
func (r *PermissionRepository) Create(ctx context.Context, p *model.UserPermission) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// FindByUserID returns the permission record for a user, or (nil, nil) when absent.
func (r *PermissionRepository) FindByUserID(ctx context.Context, userID string) (*model.UserPermission, error) {
	var p model.UserPermission
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Update updates permission fields for a user.
func (r *PermissionRepository) Update(ctx context.Context, userID string, updates map[string]bool) error {
	return r.db.WithContext(ctx).Model(&model.UserPermission{}).
		Where("user_id = ?", userID).Updates(updates).Error
}

// Upsert creates or updates a permission record.
func (r *PermissionRepository) Upsert(ctx context.Context, p *model.UserPermission) error {
	return r.db.WithContext(ctx).Where("user_id = ?", p.UserID).
		Assign(*p).FirstOrCreate(p).Error
}

// Delete removes a permission record.
func (r *PermissionRepository) Delete(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.UserPermission{}).Error
}

// ─── Refresh Token ───────────────────────────────────────────────────────────

// RefreshTokenRepository persists model.RefreshToken records.
type RefreshTokenRepository struct{ db *gorm.DB }

// Create inserts a new refresh token record.
func (r *RefreshTokenRepository) Create(ctx context.Context, t *model.RefreshToken) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// FindByHash returns the refresh token matching the hash, or (nil, nil).
func (r *RefreshTokenRepository) FindByHash(ctx context.Context, hash string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// RevokeByUserID revokes all refresh tokens for a user.
func (r *RefreshTokenRepository) RevokeByUserID(ctx context.Context, userID string) error {
	return r.db.WithContext(ctx).Model(&model.RefreshToken{}).
		Where("user_id = ?", userID).Update("revoked", true).Error
}

// DeleteExpired removes all expired refresh tokens.
func (r *RefreshTokenRepository) DeleteExpired(ctx context.Context) error {
	return r.db.WithContext(ctx).Where("expires_at < ?", time.Now()).Delete(&model.RefreshToken{}).Error
}

// Revoke revokes a specific refresh token.
func (r *RefreshTokenRepository) Revoke(ctx context.Context, hash string) error {
	return r.db.WithContext(ctx).Model(&model.RefreshToken{}).
		Where("token_hash = ?", hash).Update("revoked", true).Error
}

// HashToken returns the SHA256 hash of a token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// ─── API Config ──────────────────────────────────────────────────────────────

// ApiConfigRepository persists model.ApiConfig records.
type ApiConfigRepository struct{ db *gorm.DB }

// Create inserts a new API config record.
func (r *ApiConfigRepository) Create(ctx context.Context, c *model.ApiConfig) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// FindByProvider returns the API config for a provider, or (nil, nil).
func (r *ApiConfigRepository) FindByProvider(ctx context.Context, provider string) (*model.ApiConfig, error) {
	var c model.ApiConfig
	err := r.db.WithContext(ctx).Where("provider = ?", provider).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List returns all API configs.
func (r *ApiConfigRepository) List(ctx context.Context) ([]model.ApiConfig, error) {
	var rows []model.ApiConfig
	err := r.db.WithContext(ctx).Order("provider asc").Find(&rows).Error
	return rows, err
}

// Upsert creates or updates an API config.
func (r *ApiConfigRepository) Upsert(ctx context.Context, c *model.ApiConfig) error {
	return r.db.WithContext(ctx).Where("provider = ?", c.Provider).
		Assign(model.ApiConfig{
			Base:    model.Base{UpdatedAt: time.Now()},
			APIKey:  c.APIKey,
			BaseURL: c.BaseURL,
			Extra:   c.Extra,
			Enabled: c.Enabled,
		}).FirstOrCreate(c).Error
}

// Update updates an API config.
func (r *ApiConfigRepository) Update(ctx context.Context, c *model.ApiConfig) error {
	return r.db.WithContext(ctx).Model(&model.ApiConfig{}).
		Where("provider = ?", c.Provider).Updates(map[string]any{
		"api_key":    c.APIKey,
		"base_url":   c.BaseURL,
		"extra":      c.Extra,
		"enabled":    c.Enabled,
		"updated_at": time.Now(),
	}).Error
}

// Delete removes an API config.
func (r *ApiConfigRepository) Delete(ctx context.Context, provider string) error {
	return r.db.WithContext(ctx).Where("provider = ?", provider).Delete(&model.ApiConfig{}).Error
}

// UpdateTestResult 更新测试结果。
func (r *ApiConfigRepository) UpdateTestResult(ctx context.Context, provider, result string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.ApiConfig{}).
		Where("provider = ?", provider).Updates(map[string]any{
		"test_result":    result,
		"last_tested_at": &now,
	}).Error
}
