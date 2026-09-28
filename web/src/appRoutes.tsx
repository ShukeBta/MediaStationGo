/* eslint-disable react-refresh/only-export-components */
import { lazy, type ReactElement } from 'react'
import { Navigate } from 'react-router-dom'

const HomePage = lazy(() => import('./pages/HomePage').then((m) => ({ default: m.HomePage })))
const LibraryPage = lazy(() => import('./pages/LibraryPage').then((m) => ({ default: m.LibraryPage })))
const LibrariesPage = lazy(() => import('./pages/LibrariesPage').then((m) => ({ default: m.LibrariesPage })))
const SearchPage = lazy(() => import('./pages/SearchPage').then((m) => ({ default: m.SearchPage })))
const PeoplePage = lazy(() => import('./pages/PeoplePage').then((m) => ({ default: m.PeoplePage })))
const PersonDetailPage = lazy(() => import('./pages/PersonDetailPage').then((m) => ({ default: m.PersonDetailPage })))
const FavouritesPage = lazy(() => import('./pages/FavouritesPage').then((m) => ({ default: m.FavouritesPage })))
const PlaylistsPage = lazy(() => import('./pages/PlaylistsPage').then((m) => ({ default: m.PlaylistsPage })))
const PlaylistDetailPage = lazy(() =>
  import('./pages/PlaylistDetailPage').then((m) => ({ default: m.PlaylistDetailPage })),
)
const MediaDetailPage = lazy(() => import('./pages/MediaDetailPage').then((m) => ({ default: m.MediaDetailPage })))
const PlayerPage = lazy(() => import('./pages/PlayerPage').then((m) => ({ default: m.PlayerPage })))
const AdminPage = lazy(() => import('./pages/AdminPage').then((m) => ({ default: m.AdminPage })))
const DownloadsPage = lazy(() => import('./pages/DownloadsPage').then((m) => ({ default: m.DownloadsPage })))
const SubscriptionsPage = lazy(() =>
  import('./pages/SubscriptionsPage').then((m) => ({ default: m.SubscriptionsPage })),
)
const StatsPage = lazy(() => import('./pages/StatsPage').then((m) => ({ default: m.StatsPage })))
const PlaybackStatsPage = lazy(() => import('./pages/PlaybackStatsPage').then((m) => ({ default: m.PlaybackStatsPage })))
const PlayerRequestLogsPage = lazy(() => import('./pages/PlayerRequestLogsPage').then((m) => ({ default: m.PlayerRequestLogsPage })))
const DiscoverPage = lazy(() => import('./pages/DiscoverPage').then((m) => ({ default: m.DiscoverPage })))
const TasksPage = lazy(() => import('./pages/TasksPage').then((m) => ({ default: m.TasksPage })))
const RecycleBinPage = lazy(() => import('./pages/RecycleBinPage').then((m) => ({ default: m.RecycleBinPage })))
const DlnaPage = lazy(() => import('./pages/DlnaPage').then((m) => ({ default: m.DlnaPage })))
const FileManagerPage = lazy(() =>
  import('./pages/FileManagerPage').then((m) => ({ default: m.FileManagerPage })),
)
const StoragePage = lazy(() => import('./pages/StoragePage').then((m) => ({ default: m.StoragePage })))
const DuplicatesPage = lazy(() => import('./pages/DuplicatesPage').then((m) => ({ default: m.DuplicatesPage })))
const SchedulerPage = lazy(() => import('./pages/SchedulerPage').then((m) => ({ default: m.SchedulerPage })))
const WatchHistoryPage = lazy(() =>
  import('./pages/WatchHistoryPage').then((m) => ({ default: m.WatchHistoryPage })),
)
const PosterWallPage = lazy(() => import('./pages/PosterWallPage').then((m) => ({ default: m.PosterWallPage })))
const SitesPage = lazy(() => import('./pages/SitesPage').then((m) => ({ default: m.SitesPage })))
const SiteSearchPage = lazy(() => import('./pages/SiteSearchPage').then((m) => ({ default: m.SiteSearchPage })))
const ProfilePage = lazy(() => import('./pages/ProfilePage').then((m) => ({ default: m.ProfilePage })))
const PTResourcesPage = lazy(() => import('./pages/PTResourcesPage').then((m) => ({ default: m.PTResourcesPage })))
const AIAssistantPage = lazy(() =>
  import('./pages/AIAssistantPage').then((m) => ({ default: m.AIAssistantPage })),
)
const StrmPage = lazy(() => import('./pages/StrmPage').then((m) => ({ default: m.StrmPage })))
const ProfileManagementPage = lazy(() =>
  import('./pages/ProfileManagementPage').then((m) => ({ default: m.ProfileManagementPage })),
)
const NotifyChannelsPage = lazy(() =>
  import('./pages/NotifyChannelsPage').then((m) => ({ default: m.NotifyChannelsPage })),
)
const SettingsPage = lazy(() => import('./pages/SettingsPage').then((m) => ({ default: m.SettingsPage })))
const AssistantChatPage = lazy(() =>
  import('./pages/AssistantChatPage').then((m) => ({ default: m.AssistantChatPage })),
)
const DownloadClientsPage = lazy(() =>
  import('./pages/DownloadClientsPage').then((m) => ({ default: m.DownloadClientsPage })),
)
const StorageConfigPage = lazy(() =>
  import('./pages/StorageConfigPage').then((m) => ({ default: m.StorageConfigPage })),
)
const LicensePage = lazy(() => import('./pages/LicensePage').then((m) => ({ default: m.LicensePage })))

export type AppRoute = {
  path?: string
  index?: boolean
  element: ReactElement
  adminOnly?: boolean
  permission?: string
}

export const appRoutes: AppRoute[] = [
  { index: true, element: <HomePage />, permission: 'can_view_dashboard' },
  { path: 'libraries', element: <LibrariesPage />, permission: 'can_play_media' },
  { path: 'library/:id', element: <LibraryPage />, permission: 'can_play_media' },
  { path: 'discover', element: <DiscoverPage />, permission: 'can_view_discover' },
  { path: 'search', element: <SearchPage />, permission: 'can_play_media' },
  { path: 'people', element: <PeoplePage />, permission: 'can_play_media' },
  { path: 'people/:id', element: <PersonDetailPage />, permission: 'can_play_media' },
  { path: 'favourites', element: <FavouritesPage />, permission: 'can_favorite' },
  { path: 'playlists', element: <PlaylistsPage />, permission: 'can_play_media' },
  { path: 'playlist/:id', element: <PlaylistDetailPage />, permission: 'can_play_media' },
  { path: 'media/:id', element: <MediaDetailPage />, permission: 'can_play_media' },
  { path: 'play/:id', element: <PlayerPage />, permission: 'can_play_media' },
  { path: 'downloads', element: <DownloadsPage />, permission: 'can_manage_downloads' },
  { path: 'subscriptions', element: <SubscriptionsPage />, adminOnly: true },
  { path: 'profile', element: <ProfilePage /> },
  { path: 'dlna', element: <DlnaPage />, permission: 'can_cast' },
  { path: 'history', element: <WatchHistoryPage />, permission: 'can_view_history' },
  { path: 'poster-wall', element: <PosterWallPage />, permission: 'can_play_media' },
  { path: 'site-search', element: <SiteSearchPage />, permission: 'can_manage_sites' },
  { path: 'pt-resources', element: <PTResourcesPage />, permission: 'can_manage_sites' },
  { path: 'ai', element: <AIAssistantPage />, permission: 'can_use_ai_assistant' },
  { path: 'play-profiles', element: <ProfileManagementPage /> },
  { path: 'api-configs', element: <Navigate to="/admin?tab=api" replace /> },
  { path: 'tools', element: <Navigate to="/storage" replace /> },
  { path: 'sites', element: <SitesPage />, adminOnly: true },
  { path: 'files', element: <FileManagerPage />, adminOnly: true },
  { path: 'storage', element: <StoragePage />, adminOnly: true },
  { path: 'duplicates', element: <DuplicatesPage />, adminOnly: true },
  { path: 'scheduler', element: <SchedulerPage />, adminOnly: true },
  { path: 'tasks', element: <TasksPage />, adminOnly: true },
  { path: 'playback-stats', element: <PlaybackStatsPage />, adminOnly: true },
  { path: 'player-request-logs', element: <PlayerRequestLogsPage />, adminOnly: true },
  { path: 'recycle', element: <RecycleBinPage />, permission: 'can_manage_files' },
  { path: 'strm', element: <StrmPage />, adminOnly: true },
  { path: 'notify-channels', element: <NotifyChannelsPage />, adminOnly: true },
  { path: 'settings', element: <SettingsPage />, adminOnly: true },
  { path: 'assistant', element: <AssistantChatPage />, adminOnly: true },
  { path: 'download-clients', element: <DownloadClientsPage />, adminOnly: true },
  { path: 'license', element: <LicensePage />, adminOnly: true },
  { path: 'storage-config', element: <StorageConfigPage />, adminOnly: true },
  { path: 'stats', element: <StatsPage />, adminOnly: true },
  { path: 'admin', element: <AdminPage />, adminOnly: true },
]
