// Package repository 实现基于 GORM 的数据访问层。
// 每个方法接受 context.Context 以便后续插入取消/追踪。
//
// Repository 故意保持精简：它们只负责持久化数据，不处理业务逻辑。
// 业务逻辑位于 internal/service。
package repository

import "gorm.io/gorm"

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
	RegCode        *RegistrationCodeRepository
	SignIn         *SignInRepository
	UserDevice     *UserDeviceRepository
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
		RegCode:        &RegistrationCodeRepository{db: db},
		SignIn:         &SignInRepository{db: db},
		UserDevice:     &UserDeviceRepository{db: db},
	}
}
