package service

import (
	"context"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SystemInfo returns the full Emby identity payload.
func (e *EmbyService) SystemInfo() map[string]any {
	return map[string]any{
		"Id":                     embyServerID,
		"ServerId":               embyServerID,
		"ServerName":             "MediaStationGo",
		"Version":                embyCompatVersion,
		"ServerVersion":          embyCompatVersion,
		"ProductName":            "Emby Server",
		"OperatingSystem":        "Windows",
		"Architecture":           "X64",
		"LocalAddress":           "",
		"WanAddress":             "",
		"HasPendingRestart":      false,
		"IsShuttingDown":         false,
		"SupportsLibraryMonitor": true,
		"SupportsHttps":          false,
		"SupportsAutoDiscovery":  true,
		"HttpServerPortNumber":   e.cfg.App.Port,
		"HttpsPortNumber":        0,
		"PublishedServerUrl":     "",
		"WebSocketPortNumber":    e.cfg.App.Port,
		"CompletedInstallations": []any{},
		"CanSelfRestart":         false,
		"CanLaunchWebBrowser":    false,
		"CanRestart":             false,
	}
}

// SystemInfoPublic 是不需要认证的精简版（Emby Web 客户端登陆前会拉）。
func (e *EmbyService) SystemInfoPublic() map[string]any {
	return map[string]any{
		"Id":                     embyServerID,
		"ServerId":               embyServerID,
		"ServerName":             "MediaStationGo",
		"Version":                embyCompatVersion,
		"ServerVersion":          embyCompatVersion,
		"ProductName":            "Emby Server",
		"OperatingSystem":        "Windows",
		"LocalAddress":           "",
		"WanAddress":             "",
		"HttpServerPortNumber":   e.cfg.App.Port,
		"HttpsPortNumber":        0,
		"SupportsHttps":          false,
		"SupportsAutoDiscovery":  true,
		"StartupWizardCompleted": true,
	}
}

// ListUsers returns Emby-shaped users.
func (e *EmbyService) ListUsers(ctx context.Context) ([]map[string]any, error) {
	users, err := e.repo.User.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, e.userPayload(&u))
	}
	return out, nil
}

// FindUser 用 ID 查用户，用于 /Users/Me 与 /Users/{id}。
func (e *EmbyService) FindUser(ctx context.Context, id string) (map[string]any, error) {
	u, err := e.repo.User.FindByID(ctx, id)
	if err != nil || u == nil {
		return nil, err
	}
	return e.userPayload(u), nil
}

func (e *EmbyService) userPayload(u *model.User) map[string]any {
	canDownload := u.Role == "admin"
	return map[string]any{
		"Id":                        u.ID,
		"Name":                      u.Username,
		"ServerId":                  embyServerID,
		"ServerName":                "MediaStationGo",
		"HasPassword":               true,
		"HasConfiguredPassword":     true,
		"HasConfiguredEasyPassword": false,
		"EnableAutoLogin":           false,
		"LastLoginDate":             u.LastLoginAt,
		"LastActivityDate":          u.UpdatedAt,
		"Configuration": map[string]any{
			"PlayDefaultAudioTrack":      true,
			"DisplayCollectionsView":     true,
			"DisplayMissingEpisodes":     false,
			"SubtitleMode":               "Default",
			"EnableNextEpisodeAutoPlay":  true,
			"AudioLanguagePreference":    "",
			"SubtitleLanguagePreference": "",
		},
		"Policy": map[string]any{
			"IsAdministrator":                u.Role == "admin",
			"IsHidden":                       false,
			"IsDisabled":                     !u.IsActive,
			"EnableUserPreferenceAccess":     true,
			"EnableRemoteAccess":             true,
			"EnableMediaPlayback":            true,
			"EnableAudioPlaybackTranscoding": true,
			"EnableVideoPlaybackTranscoding": true,
			"EnablePlaybackRemuxing":         true,
			"EnableLiveTvAccess":             false,
			"EnableContentDownloading":       canDownload,
			"EnableSyncTranscoding":          canDownload,
			"EnableMediaConversion":          canDownload,
			"EnableAllChannels":              true,
			"EnableAllFolders":               true,
			"EnableAllDevices":               true,
			"AuthenticationProviderId":       embyLocalAuthenticationProviderID,
			"PasswordResetProviderId":        embyLocalPasswordResetProviderID,
		},
	}
}

// Views 返回 Emby 中"虚拟根目录"——每个 library 一个条目。
func (e *EmbyService) Views(ctx context.Context, userID string) (map[string]any, error) {
	libs, err := e.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	libs = FilterDisplayCloudLibraries(ctx, e.repo, libs)
	visibility := e.mediaVisibility(ctx, userID)
	items := make([]map[string]any, 0, len(libs))
	for _, l := range libs {
		if !e.libraryVisibleFromCachedVisibility(l, visibility) {
			continue
		}
		items = append(items, e.libraryAsViews(ctx, userID, &l)...)
	}
	return map[string]any{"Items": items, "TotalRecordCount": len(items), "StartIndex": 0}, nil
}

func (e *EmbyService) libraryAsView(ctx context.Context, l *model.Library) map[string]any {
	return e.libraryAsViewWith(ctx, "", l, l.ID, l.Name, e.libraryCollectionType(ctx, l))
}

func (e *EmbyService) libraryAsViews(ctx context.Context, userID string, l *model.Library) []map[string]any {
	if l == nil {
		return nil
	}
	realView := e.libraryAsViewWith(ctx, userID, l, l.ID, l.Name, e.libraryCollectionType(ctx, l))
	if strings.EqualFold(strings.TrimSpace(l.Type), "music") {
		return []map[string]any{realView}
	}
	shape, err := e.libraryMediaShape(ctx, l.ID)
	if err == nil && shape.HasMovies && shape.HasEpisodes {
		return []map[string]any{
			e.libraryAsViewWith(ctx, userID, l, virtualLibraryID("movies", l.ID), l.Name+" · 电影", "movies"),
			e.libraryAsViewWith(ctx, userID, l, virtualLibraryID("shows", l.ID), l.Name+" · 剧集", "tvshows"),
		}
	}
	return []map[string]any{realView}
}

func (e *EmbyService) libraryAsViewWith(ctx context.Context, userID string, l *model.Library, id, name, collectionType string) map[string]any {
	counts := e.libraryViewCounts(ctx, userID, l, id, collectionType)
	userData := map[string]any{
		"PlaybackPositionTicks": 0,
		"PlayCount":             0,
		"IsFavorite":            false,
		"Played":                false,
		"UnplayedItemCount":     counts.Unplayed,
	}
	return map[string]any{
		"Id":                       id,
		"Name":                     name,
		"CollectionType":           collectionType,
		"ServerId":                 embyServerID,
		"Type":                     "CollectionFolder",
		"IsFolder":                 true,
		"Path":                     l.Path,
		"SortName":                 strings.ToLower(name),
		"DateCreated":              l.CreatedAt.UTC().Format(time.RFC3339),
		"CanDelete":                false,
		"CanDownload":              false,
		"DisplayPreferencesId":     id,
		"PrimaryImageItemId":       id,
		"PrimaryImageAspectRatio":  1.7777777777777777,
		"RecursiveItemCount":       counts.Recursive,
		"ChildCount":               counts.Child,
		"SpecialFeatureCount":      0,
		"EnableMediaSourceDisplay": true,
		"PlayAccess":               "Full",
		"ExternalUrls":             []any{},
		"ProviderIds":              map[string]string{},
		"Genres":                   []string{},
		"Tags":                     []string{},
		"ImageTags":                map[string]string{},
		"BackdropImageTags":        []string{},
		"UserData":                 userData,
	}
}
