import { api, BATCH_REQUEST_TIMEOUT, LONG_REQUEST_TIMEOUT } from './client'
import type { Library, Media, ScanResult } from '../types'

export interface MediaPage {
  items: Media[]
  total: number
  page: number
  page_size: number
}

export interface MediaSearchPage {
  items: Media[]
  total?: number
  page?: number
  page_size?: number
}

export interface ManualScrapeCandidate {
  source: string
  media_type?: string
  title: string
  original_name?: string
  overview?: string
  poster_url?: string
  backdrop_url?: string
  year?: number
  rating?: number
  tmdb_id?: number
  bangumi_id?: number
  douban_id?: string
  thetvdb_id?: string
  languages?: string[]
  countries?: string[]
  genres?: string[]
  nsfw?: boolean
}

export interface MediaMetadataUpdate {
  title?: string
  original_name?: string
  overview?: string
  poster_url?: string
  backdrop_url?: string
  year?: number
  rating?: number
  season_num?: number
  episode_num?: number
  tmdb_id?: number
  bangumi_id?: number
  douban_id?: string
  thetvdb_id?: string
  languages?: string
  countries?: string
  genres?: string
  nsfw?: boolean
}

export const libraryAPI = {
  list: (options?: { includeHidden?: boolean }) =>
    api
      .get<Library[]>('/libraries', {
        params: options?.includeHidden ? { include_hidden: 1 } : undefined,
      })
      .then((r) => r.data),

  create: (name: string, path: string, type: string) =>
    api.post<Library>('/libraries', { name, path, type }).then((r) => r.data),

  remove: (id: string) => api.delete(`/libraries/${id}`).then((r) => r.data),

  scan: (id: string) =>
    api.post<ScanResult>(`/libraries/${id}/scan`, null, { timeout: BATCH_REQUEST_TIMEOUT }).then((r) => r.data),

  scrape: (id: string) =>
    api.post(`/libraries/${id}/scrape`, null, { timeout: BATCH_REQUEST_TIMEOUT }).then((r) => r.data),

  listMedia: (id: string, page = 1, pageSize = 50, options?: { groupVersions?: boolean }) =>
    api
      .get<MediaPage>(`/libraries/${id}/media`, {
        params: {
          page,
          page_size: pageSize,
          ...(options?.groupVersions === true ? { group_versions: 1 } : {}),
          ...(options?.groupVersions === false ? { group_versions: 0 } : {}),
        },
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),
}

export const mediaAPI = {
  search: (q: string, limit = 50) =>
    api.get<MediaSearchPage>('/media', { params: { q, limit } }).then((r) => r.data),

  searchPage: (q: string, page = 1, pageSize = 50) =>
    api
      .get<MediaSearchPage>('/media', {
        params: { q, page, page_size: pageSize },
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  get: (id: string) => api.get<Media>(`/media/${id}`).then((r) => r.data),

  updateMetadata: (id: string, payload: MediaMetadataUpdate) =>
    api.patch<Media>(`/media/${id}/metadata`, payload, { timeout: LONG_REQUEST_TIMEOUT }).then((r) => r.data),

  manualScrapeSearch: (id: string, params: { query: string; provider?: string; media_type?: string }) =>
    api
      .get<{ items: ManualScrapeCandidate[] }>(`/media/${id}/scrape/search`, { params })
      .then((r) => r.data.items),

  applyManualScrape: (id: string, match: ManualScrapeCandidate) =>
    api.post<Media>(`/media/${id}/scrape/apply`, match, { timeout: LONG_REQUEST_TIMEOUT }).then((r) => r.data),

  applyManualScrapeBatch: (mediaIDs: string[], match: ManualScrapeCandidate) =>
    api
      .post<{ applied: number; errors?: string[] }>('/media/scrape/apply', { media_ids: mediaIDs, match }, { timeout: BATCH_REQUEST_TIMEOUT })
      .then((r) => r.data),
}
