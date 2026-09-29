import type { LucideIcon } from 'lucide-react'
import {
  Activity,
  Cast,
  Clock,
  CloudDownload,
  Compass,
  Database,
  Globe,
  HardDrive,
  Heart,
  Home,
  Image,
  KeySquare,
  Library,
  ListMusic,
  MessageSquareText,
  Rss,
  Search,
  Settings,
  Sliders,
  Sparkles,
  Trash2,
  User,
} from 'lucide-react'

export type LayoutNavGroupID = 'media' | 'personal' | 'downloads' | 'tools' | 'system'

export type LayoutNavItem = {
  to: string
  label: string
  icon: LucideIcon
  end?: boolean
  permission?: string
  adminOnly?: boolean
}

export type LayoutNavGroup = {
  id: LayoutNavGroupID
  label: string
  icon: LucideIcon
  activePaths: string[]
  adminOnly?: boolean
  items: LayoutNavItem[]
}

export const LAYOUT_NAV_GROUPS: LayoutNavGroup[] = [
  {
    id: 'media',
    label: '观影空间',
    icon: Home,
    activePaths: ['/', '/libraries', '/library', '/poster-wall', '/discover', '/search', '/people', '/dlna', '/ai'],
    items: [
      { to: '/', label: '影院首页', icon: Home, end: true, permission: 'can_view_dashboard' },
      { to: '/libraries', label: '媒体库', icon: Library, permission: 'can_play_media' },
      { to: '/poster-wall', label: '海报墙', icon: Image, permission: 'can_play_media' },
      { to: '/discover', label: '精彩发现', icon: Compass, permission: 'can_view_discover' },
      { to: '/search', label: '搜索', icon: Search, permission: 'can_play_media' },
      { to: '/people', label: '人物资料', icon: User, permission: 'can_play_media' },
      { to: '/dlna', label: 'DLNA 投屏', icon: Cast, permission: 'can_cast' },
      { to: '/ai', label: 'AI 助理', icon: Sparkles, permission: 'can_use_ai_assistant' },
    ],
  },
  {
    id: 'personal',
    label: '我的片单',
    icon: User,
    activePaths: ['/favourites', '/playlists', '/playlist', '/history', '/play-profiles', '/recycle'],
    items: [
      { to: '/favourites', label: '我的收藏', icon: Heart, permission: 'can_favorite' },
      { to: '/playlists', label: '播放列表', icon: ListMusic, permission: 'can_play_media' },
      { to: '/history', label: '观看历史', icon: Clock, permission: 'can_view_history' },
      { to: '/recycle', label: '回收站', icon: Trash2, permission: 'can_manage_files' },
    ],
  },
  {
    id: 'downloads',
    label: '内容管理',
    icon: CloudDownload,
    activePaths: ['/downloads', '/download-clients', '/subscriptions', '/site-search', '/pt-resources', '/sites'],
    items: [
      { to: '/downloads', label: '下载中心', icon: Activity, permission: 'can_manage_downloads' },
      { to: '/subscriptions', label: '自动追更', icon: Rss, adminOnly: true },
      { to: '/pt-resources', label: 'PT 资源中心', icon: Database, permission: 'can_manage_sites' },
      { to: '/sites', label: 'PT / RSS 站点', icon: Globe, permission: 'can_manage_sites', adminOnly: true },
    ],
  },
  {
    id: 'tools',
    label: '工作空间',
    icon: HardDrive,
    activePaths: ['/storage', '/storage-config', '/files', '/strm', '/duplicates', '/scheduler', '/recycle', '/stats', '/tasks', '/playback-stats', '/player-request-logs'],
    adminOnly: true,
    items: [
      { to: '/storage', label: '存储与文件', icon: HardDrive },
      { to: '/tasks', label: '系统任务', icon: Activity },
      { to: '/playback-stats', label: '播放统计', icon: Activity },
      { to: '/player-request-logs', label: '播放器请求日志', icon: Clock },
    ],
  },
  {
    id: 'system',
    label: '系统管理',
    icon: Settings,
    activePaths: ['/admin', '/sites', '/notify-channels', '/license', '/settings', '/assistant'],
    adminOnly: true,
    items: [
      { to: '/admin', label: '媒体与用户', icon: Settings },
      { to: '/settings', label: '系统设置', icon: Sliders },
      { to: '/notify-channels', label: '通知配置', icon: MessageSquareText },
      { to: '/license', label: '授权许可', icon: KeySquare },
    ],
  },
]

export const NAV_GROUP_PATHS: Record<LayoutNavGroupID, string[]> = LAYOUT_NAV_GROUPS.reduce(
  (paths, group) => ({ ...paths, [group.id]: group.activePaths }),
  {} as Record<LayoutNavGroupID, string[]>,
)
