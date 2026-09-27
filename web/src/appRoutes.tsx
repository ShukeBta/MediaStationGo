/* eslint-disable react-refresh/only-export-components */
import { lazy, type ReactElement } from 'react'
import { Navigate } from 'react-router-dom'

const HomePage = lazy(() => import('./pages/HomePage').then((m) => ({ default: m.HomePage })))
const LibraryPage = lazy(() => import('./pages/LibraryPage').then((m) => ({ default: m.LibraryPage })))
const LibrariesPage = lazy(() => import('./pages/LibrariesPage').then((m) => ({ default: m.LibrariesPage })))
const SearchPage = lazy(() => import('./pages/SearchPage').then((m) => ({ default: m.SearchPage })))
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
const ProfilePage = lazy(() => import('./pages/ProfilePage').then((m) => ({ default: m.ProfilePage })))
const StatsPage = lazy(() => import('./pages/StatsPage').then((m) => ({ default: m.StatsPage })))
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
}

export const appRoutes: AppRoute[] = [
  { index: true, element: <HomePage /> },
  { path: 'libraries', element: <LibrariesPage /> },
  { path: 'library/:id', element: <LibraryPage /> },
  { path: 'discover', element: <DiscoverPage /> },
  { path: 'search', element: <SearchPage /> },
  { path: 'favourites', element: <FavouritesPage /> },
  { path: 'playlists', element: <PlaylistsPage /> },
  { path: 'playlist/:id', element: <PlaylistDetailPage /> },
  { path: 'media/:id', element: <MediaDetailPage /> },
  { path: 'play/:id', element: <PlayerPage /> },
  { path: 'downloads', element: <DownloadsPage /> },
  { path: 'subscriptions', element: <SubscriptionsPage /> },
  { path: 'profile', element: <ProfilePage /> },
  { path: 'dlna', element: <DlnaPage /> },
  { path: 'history', element: <WatchHistoryPage /> },
  { path: 'poster-wall', element: <PosterWallPage /> },
  { path: 'site-search', element: <SiteSearchPage /> },
  { path: 'pt-resources', element: <PTResourcesPage /> },
  { path: 'ai', element: <AIAssistantPage /> },
  { path: 'play-profiles', element: <ProfileManagementPage /> },
  { path: 'api-configs', element: <Navigate to="/admin?tab=api" replace /> },
  { path: 'tools', element: <Navigate to="/storage" replace /> },
  { path: 'sites', element: <SitesPage />, adminOnly: true },
  { path: 'files', element: <FileManagerPage />, adminOnly: true },
  { path: 'storage', element: <StoragePage />, adminOnly: true },
  { path: 'duplicates', element: <DuplicatesPage />, adminOnly: true },
  { path: 'scheduler', element: <SchedulerPage />, adminOnly: true },
  { path: 'tasks', element: <TasksPage />, adminOnly: true },
  { path: 'recycle', element: <RecycleBinPage />, adminOnly: true },
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
