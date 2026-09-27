import { create } from 'zustand'

import { getMyPermissions } from '../api/permission'

interface PermissionState {
  permissions: Record<string, boolean>
  role: string
  tier: string
  isSuper: boolean
  isLoading: boolean
  error: string | null
  fetchPermissions: () => Promise<void>
  hasPermission: (key: string) => boolean
  clearPermissions: () => void
}

export const usePermissionStore = create<PermissionState>((set, get) => ({
  permissions: {},
  role: '',
  tier: 'free',
  isSuper: false,
  isLoading: false,
  error: null,

  fetchPermissions: async () => {
    set({ isLoading: true, error: null })
    try {
      const result = await getMyPermissions()
      set({
        permissions: result.permissions ?? {},
        role: result.role ?? '',
        tier: result.tier ?? 'free',
        isSuper: result.is_super ?? false,
        isLoading: false,
      })
    } catch (err) {
      set({
        isLoading: false,
        error: err instanceof Error ? err.message : 'Failed to fetch permissions',
      })
    }
  },

  hasPermission: (key: string) => {
    const state = get()
    // Only administrators bypass per-user permission rows.
    if (state.isSuper) {
      return true
    }
    return (state.permissions ?? {})[key] === true
  },

  clearPermissions: () => {
    set({
      permissions: {},
      role: '',
      tier: 'free',
      isSuper: false,
      error: null,
    })
  },
}))

// Default permissions for new users (without fetching from server)
export const defaultPermissions: Record<string, boolean> = {
  can_view_dashboard: true,
  can_play_media: true,
  can_cast: true,
  can_external_player: true,
  can_favorite: true,
  can_view_history: true,
  can_edit_media: false,
  can_rescrape: false,
  can_use_ai: false,
  can_capture_frames: false,
  can_manage_downloads: false,
  can_view_discover: false,
  can_manage_subscriptions: false,
  can_manage_sites: false,
  can_use_ai_assistant: false,
  can_manage_users: false,
  can_manage_files: false,
  can_manage_strm: false,
  can_access_settings: false,
}

// Permission display names for UI
export const permissionDisplayNames: Record<string, string> = {
  can_view_dashboard: '查看仪表盘',
  can_play_media: '播放媒体',
  can_cast: '投屏',
  can_external_player: '外部播放器',
  can_favorite: '收藏',
  can_view_history: '观看历史',
  can_edit_media: '编辑媒体',
  can_rescrape: '重新刮削',
  can_use_ai: '使用 AI 搜索',
  can_capture_frames: '截图',
  can_manage_downloads: '管理下载',
  can_view_discover: '发现',
  can_manage_subscriptions: '管理订阅',
  can_manage_sites: '管理站点',
  can_use_ai_assistant: 'AI 助手',
  can_manage_users: '管理用户',
  can_manage_files: '管理文件',
  can_manage_strm: '管理 STRM',
  can_access_settings: '访问设置',
}

// Permission categories for grouping
export const permissionCategories = {
  basic: [
    'can_view_dashboard',
    'can_play_media',
    'can_cast',
    'can_external_player',
    'can_favorite',
    'can_view_history',
  ],
  media: [
    'can_edit_media',
    'can_rescrape',
    'can_capture_frames',
    'can_manage_files',
    'can_manage_strm',
  ],
  advanced: [
    'can_use_ai',
    'can_use_ai_assistant',
    'can_view_discover',
    'can_manage_downloads',
    'can_manage_subscriptions',
    'can_manage_sites',
  ],
  admin: [
    'can_manage_users',
    'can_access_settings',
  ],
}
