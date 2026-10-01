import { api, LONG_REQUEST_TIMEOUT } from './client'

// ─── TypeScript interfaces ──────────────────────────────────────────────
// Note: the canonical Site type lives in ../types/index.ts
// The backend uses json:"url" (not base_url) and json:"type" (not site_type).

export interface SiteSearchResult {
  site_name: string
  site_id: string
  id?: string
  title: string
  subtitle?: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  torrent_url: string
  download_url: string
  category?: string
  size: number
  seeders: number
  leechers: number
  snatched?: number
  free: boolean
  adult?: boolean
  upload_time?: string
}

export interface SiteSubscribeResponse {
  subscription?: unknown
  queued?: number
  search_keyword?: string
  category?: string
  include_adult?: boolean
  explanation?: string[]
  run_error?: string
}

export interface SiteCategory {
  id: string
  name: string
  group: string
  parent_id?: string
  site_id?: string
  site_name?: string
  site_type?: string
  adult: boolean
  description?: string
}

export interface SiteBrowseResponse {
  items: SiteSearchResult[]
  total: number
  page: number
  page_size?: number
  total_pages?: number
  category?: string
  keyword?: string
}

export interface QBitTorrentFile {
  index: number
  name: string
  size: number
  priority: number
}

export interface SiteDownloadPrepareResponse {
  hash: string
  files: QBitTorrentFile[]
}

export interface SiteAPIErrorPayload {
  error?: string
  code?: string
  reason?: string
  title?: string
}

export interface SiteDownloadInput {
  site_id?: string
  id?: string
  title: string
  download_url?: string
  torrent_url?: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  save_path?: string
  media_type?: string
  media_category?: string
  source_category?: string
  selected_files?: string[]
}

export interface CreateSiteInput {
  name: string
  url: string
  type?: string
  auth_type?: string
  cookie?: string
  api_key?: string
  auth_header?: string
  enabled?: boolean
  is_default?: boolean
  extra?: string
}

// ─── API client ─────────────────────────────────────────────────────────

export const sitesAPI = {
  // List all sites
  list: () => api.get('/sites').then((r) => r.data),

  // Get single site with decrypted fields
  get: (id: string | number) => api.get(`/sites/${id}`).then((r) => r.data),

  // Create a new site
  create: (data: Record<string, unknown>) =>
    api.post('/sites', data).then((r) => r.data),

  // Update existing site
  update: (id: string | number, data: Record<string, unknown>) =>
    api.put(`/sites/${id}`, data).then((r) => r.data),

  // Delete a site
  remove: (id: string | number) =>
    api.delete(`/sites/${id}`).then((r) => r.data),

  // Test site connectivity
  test: (id: string | number) =>
    api.post(`/sites/${id}/test`).then((r) => r.data),

  // Get supported site types
  types: () => api.get('/sites/types').then((r) => r.data),

  // Get supported auth types
  authTypes: () => api.get('/sites/auth-types').then((r) => r.data),

  // Search across all sites
  search: (keyword: string) =>
    api
      .get('/sites/search', { params: { keyword }, timeout: LONG_REQUEST_TIMEOUT })
      .then((r) => r.data),

  categories: (siteID = '') =>
    api
      .get<{ items: SiteCategory[] }>('/sites/categories', { params: { site_id: siteID || undefined } })
      .then((r) => r.data.items ?? []),

  browse: (params: {
    site_id?: string
    category?: string
    keyword?: string
    page?: number
    include_adult?: boolean
  }) =>
    api
      .get<SiteBrowseResponse>('/sites/browse', { params })
      .then((r) => r.data),

  detail: (siteID: string, id: string, signal?: AbortSignal) =>
    api
      .get('/sites/detail', { params: { site_id: siteID, id }, signal })
      .then((r) => r.data),

  download: (input: SiteDownloadInput) => api.post('/sites/download', input, { timeout: LONG_REQUEST_TIMEOUT }).then((r) => r.data),

  prepareDownload: (input: SiteDownloadInput) =>
    api.post<SiteDownloadPrepareResponse>('/sites/download/prepare', input).then((r) => r.data),

  confirmDownload: (input: SiteDownloadInput & { hash: string; selected_file_indexes?: number[] }) =>
    api.post('/sites/download/confirm', input).then((r) => r.data),

  cancelPreparedDownload: (hash: string) =>
    api.post('/sites/download/cancel', { hash }).then((r) => r.data),

  subscribe: (input: {
    site_id?: string
    id?: string
    category?: string
    include_adult?: boolean
    name: string
    keyword: string
    filter?: string
    original_title?: string
    year?: number
    media_type?: string
    media_category?: string
    poster_url?: string
    backdrop_url?: string
    overview?: string
    save_path?: string
    season_number?: number
    total_episodes?: number
    poll_interval_minutes?: number
    enabled?: boolean
  }) => api.post<SiteSubscribeResponse>('/sites/subscribe', input, { timeout: LONG_REQUEST_TIMEOUT }).then((r) => r.data),
}
