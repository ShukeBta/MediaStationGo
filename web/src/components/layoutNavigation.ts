import type { LucideIcon } from 'lucide-react'
import {
  Activity,
  Cast,
  Clock,
  CloudDownload,
  Compass,
  FileText,
  Globe,
  HardDrive,
  Heart,
  Home,
  Image,
  KeySquare,
  Library,
  ListMusic,
  Rss,
  Search,
  Settings,
  Sliders,
  Sparkles,
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
    label: '媒体浏览',
    icon: Home,
    activePaths: ['/', '/libraries', '/library', '/poster-wall', '/discover', '/search', '/dlna', '/ai'],
    items: [
      { to: '/', label: '系统首页', icon: Home, end: true },
      { to: '/libraries', label: '媒体库', icon: Library },
      { to: '/poster-wall', label: '海报墙', icon: Image },
      { to: '/discover', label: '精彩发现', icon: Compass, permission: 'can_view_discover' },
      { to: '/search', label: '智能搜索', icon: Search, permission: 'can_use_ai' },
      { to: '/dlna', label: 'DLNA 投屏', icon: Cast, permission: 'can_cast' },
      { to: '/ai', label: 'AI 助理', icon: Sparkles, permission: 'can_use_ai_assistant' },
    ],
  },
  {
    id: 'personal',
    label: '个人观影',
    icon: User,
    activePaths: ['/favourites', '/playlists', '/playlist', '/history', '/profile', '/play-profiles'],
    items: [
      { to: '/favourites', label: '我的收藏', icon: Heart },
      { to: '/playlists', label: '播放列表', icon: ListMusic },
      { to: '/history', label: '观看历史', icon: Clock },
    ],
  },
  {
    id: 'downloads',
    label: '下载与订阅',
    icon: CloudDownload,
    activePaths: ['/downloads', '/download-clients', '/subscriptions', '/site-search', '/sites', '/tasks'],
    items: [
      { to: '/downloads', label: '下载中心', icon: CloudDownload, permission: 'can_manage_downloads' },
      { to: '/subscriptions', label: '订阅管理', icon: Rss, permission: 'can_manage_subscriptions' },
      { to: '/sites', label: '站点管理', icon: Globe, permission: 'can_manage_sites' },
      { to: '/tasks', label: '实时任务', icon: Activity },
    ],
  },
  {
    id: 'tools',
    label: '文件与自动化',
    icon: HardDrive,
    activePaths: ['/storage', '/storage-config', '/files', '/strm', '/duplicates', '/scheduler', '/recycle', '/stats'],
    adminOnly: true,
    items: [
      { to: '/storage', label: '存储与文件', icon: HardDrive },
      { to: '/strm', label: 'STRM 管理', icon: FileText },
    ],
  },
  {
    id: 'system',
    label: '系统配置',
    icon: Settings,
    activePaths: ['/admin', '/sites', '/notify-channels', '/license', '/settings', '/assistant'],
    adminOnly: true,
    items: [
      { to: '/admin', label: '媒体与用户', icon: Settings },
      { to: '/settings', label: '系统设置', icon: Sliders },
      { to: '/license', label: '授权许可', icon: KeySquare },
    ],
  },
]

export const NAV_GROUP_PATHS: Record<LayoutNavGroupID, string[]> = LAYOUT_NAV_GROUPS.reduce(
  (paths, group) => ({ ...paths, [group.id]: group.activePaths }),
  {} as Record<LayoutNavGroupID, string[]>,
)
