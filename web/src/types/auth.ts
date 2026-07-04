export interface User {
  id: string
  username: string
  role: 'admin' | 'user'
  tier: 'free' | 'plus'
  nickname?: string
  email?: string
  avatar_url?: string
  hide_adult?: boolean
  force_password_reset: boolean
  is_active: boolean
  is_default_admin?: boolean
  is_protected?: boolean
  realtime_online?: boolean
  realtime_device_count?: number
  last_login_at?: string
  created_at: string
  updated_at: string
}

export interface TokenPair {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: string
}

export interface UserPermission {
  id: string
  user_id: string
  can_view_dashboard: boolean
  can_play_media: boolean
  can_cast: boolean
  can_external_player: boolean
  can_favorite: boolean
  can_view_history: boolean
  can_edit_media: boolean
  can_rescrape: boolean
  can_use_ai: boolean
  can_capture_frames: boolean
  can_manage_downloads: boolean
  can_view_discover: boolean
  can_manage_subscriptions: boolean
  can_manage_sites: boolean
  can_use_ai_assistant: boolean
  can_manage_users: boolean
  can_manage_files: boolean
  can_manage_strm: boolean
  can_access_settings: boolean
  created_at: string
  updated_at: string
}

export interface RefreshTokenResponse {
  token: string
  refresh_token: string
  expires_in: number
  token_type: string
}

export interface LoginResponse {
  user: User
  tokens: TokenPair
}

export interface PermissionCheckResult {
  permissions: Record<string, boolean>
  role: string
  tier: string
  is_super: boolean
}
