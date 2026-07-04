export interface Site {
  id: string
  name: string
  type: string
  url: string
  auth_type: string
  cookie?: string
  api_key?: string
  auth_header?: string
  user_agent?: string
  rss_url?: string
  timeout?: number
  priority?: number
  use_proxy?: boolean
  rate_limit?: boolean
  browser_emulation?: boolean
  login_status?: string
  upload_bytes?: number
  download_bytes?: number
  downloader?: string
  enabled: boolean
  is_default: boolean
  extra?: string
  last_error?: string
  last_check_at?: string
  created_at: string
  updated_at: string
}

export interface SiteTypeInfo {
  value: string
  name: string
  description: string
}

export interface AuthTypeInfo {
  value: string
  name: string
  description: string
}
