import { api, BATCH_REQUEST_TIMEOUT, LONG_REQUEST_TIMEOUT } from './client'
import type { Library, LibraryRoot, Media, MediaPartList, MediaVersionList, ScanResult } from '../types'
import type { SeriesCard } from '../utils/groupSeries'

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

export interface SeriesPage {
  items: SeriesCard[]
  total: number
  page: number
  page_size: number
}

export interface GeneratedArtworkStatus {
  library_id: string
  enabled: boolean
  pending: number
  running: number
  completed: number
  failed: number
  canceled: number
}

export interface MediaAggregationResult {
  action: 'group' | 'detach'
  updated: number
  group_key?: string
}

export interface LibraryRootInput {
  name?: string
  path: string
  enabled?: boolean
  sort_order?: number
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
  release_date?: string
  rating?: number
  tmdb_id?: number
  bangumi_id?: number
  douban_id?: string
  thetvdb_id?: string
  languages?: string[]
  countries?: string[]
  genres?: string[]
  actors?: string[]
  people?: Array<{
    name: string
    image_url?: string
    profile_url?: string
    source?: string
    source_id?: string
  }>
  nsfw?: boolean
}

export interface ScrapeOptions {
  refresh_matched?: boolean
  include_matched?: boolean
}

export interface MediaMetadataUpdate {
  title?: string
  original_name?: string
  overview?: string
  poster_url?: string
  backdrop_url?: string
  year?: number
  release_date?: string
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
  actors?: string
  nsfw?: boolean
}

export const libraryAPI = {
  list: (options?: { includeHidden?: boolean }) =>
    api
      .get<Library[]>('/libraries', {
        params: options?.includeHidden ? { include_hidden: 1 } : undefined,
      })
      .then((r) => r.data),

  get: (id: string, options?: { includeHidden?: boolean }) =>
    api
      .get<Library>(`/libraries/${id}`, {
        params: options?.includeHidden ? { include_hidden: 1 } : undefined,
      })
      .then((r) => r.data),

  create: (name: string, path: string, type: string, titleMode = 'smart') =>
    api.post<Library>('/libraries', { name, path, type, title_mode: titleMode }).then((r) => r.data),

  createWithRoots: (name: string, type: string, titleMode: string, roots: LibraryRootInput[]) =>
    api.post<Library>('/libraries', { name, type, title_mode: titleMode, roots }).then((r) => r.data),

  update: (id: string, patch: { enabled?: boolean; title_mode?: 'smart' | 'filename'; generate_artwork?: boolean }) =>
    api.patch<Library>(`/libraries/${id}`, patch).then((r) => r.data),

  remove: (id: string) => api.delete(`/libraries/${id}`).then((r) => r.data),

  listRoots: (id: string) => api.get<LibraryRoot[]>(`/libraries/${id}/roots`).then((r) => r.data),

  addRoot: (id: string, root: LibraryRootInput) =>
    api.post<LibraryRoot>(`/libraries/${id}/roots`, root).then((r) => r.data),

  updateRoot: (id: string, rootID: string, root: Partial<LibraryRootInput>) =>
    api.patch<LibraryRoot>(`/libraries/${id}/roots/${rootID}`, root).then((r) => r.data),

  removeRoot: (id: string, rootID: string) => api.delete(`/libraries/${id}/roots/${rootID}`).then((r) => r.data),

  scan: (id: string) =>
    api.post<ScanResult>(`/libraries/${id}/scan`, null, { timeout: BATCH_REQUEST_TIMEOUT }).then((r) => r.data),

  scanRoot: (id: string, rootID: string) =>
    api.post<ScanResult>(`/libraries/${id}/roots/${rootID}/scan`, null, { timeout: BATCH_REQUEST_TIMEOUT }).then((r) => r.data),

  generatedArtworkStatus: (id: string) =>
    api.get<GeneratedArtworkStatus>(`/libraries/${id}/generated-artwork`).then((r) => r.data),

  runGeneratedArtwork: (id: string) =>
    api.post<{ queued: number }>(`/libraries/${id}/generated-artwork`).then((r) => r.data),

  cancelGeneratedArtwork: (id: string) =>
    api.delete<{ canceled: number }>(`/libraries/${id}/generated-artwork`).then((r) => r.data),

  scrape: (id: string, options?: ScrapeOptions) =>
    api.post(`/libraries/${id}/scrape`, options ?? null, { timeout: BATCH_REQUEST_TIMEOUT }).then((r) => r.data),

  listMedia: (id: string, page = 1, pageSize = 50, options?: { groupVersions?: boolean }) =>
    api
      .get<MediaPage>(`/libraries/${id}/media`, {
        params: {
          page,
          page_size: pageSize,
          group_versions: options?.groupVersions === false ? 0 : undefined,
        },
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  listSeries: (id: string, page = 1, pageSize = 500) =>
    api
      .get<SeriesPage>(`/libraries/${id}/series`, {
        params: { page, page_size: pageSize },
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  listSeriesEpisodes: (id: string, key: string) =>
    api
      .get<{ items: Media[]; total: number }>(`/libraries/${id}/series/episodes`, {
        params: { key },
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  updateAggregation: (
    id: string,
    payload: { action: 'group' | 'detach'; media_ids: string[]; title?: string },
  ) => api.post<MediaAggregationResult>(`/libraries/${id}/media-aggregation`, payload).then((r) => r.data),
}

export const mediaAPI = {
  featured: () =>
    api.get<{ item: SeriesCard | null; week: string }>('/media/featured').then((r) => r.data),

  recent: (limit = 24) =>
    api.get<SeriesCard[]>('/media/recent', { params: { limit } }).then((r) => r.data),

  search: (q: string, limit = 50) =>
    api.get<MediaSearchPage>('/media', { params: { q, limit } }).then((r) => r.data),

  searchPage: (q: string, page = 1, pageSize = 50, options?: { groupVersions?: boolean }) =>
    api
      .get<MediaSearchPage>('/media', {
        params: {
          q,
          page,
          page_size: pageSize,
          group_versions: options?.groupVersions === false ? 0 : undefined,
        },
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  searchSeriesPage: (q: string, page = 1, pageSize = 36, signal?: AbortSignal) =>
    api
      .get<SeriesPage>('/media', {
        params: {
          q,
          page,
          page_size: pageSize,
          group_series: 1,
        },
        signal,
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  get: (id: string) => api.get<Media>(`/media/${id}`).then((r) => r.data),

  listVersions: (id: string) => api.get<MediaVersionList>(`/media/${id}/versions`).then((r) => r.data),

  listParts: (id: string) => api.get<MediaPartList>(`/media/${id}/parts`).then((r) => r.data),

  deleteVersion: (id: string, versionID: string) =>
    api
      .delete<{ deleted_id: string; next_media_id?: string }>(`/media/${id}/versions/${versionID}`)
      .then((r) => r.data),

  updateMetadata: (id: string, payload: MediaMetadataUpdate) =>
    api.patch<Media>(`/media/${id}/metadata`, payload, { timeout: LONG_REQUEST_TIMEOUT }).then((r) => r.data),

  generateArtworkAt: (id: string, timestampSeconds: number) =>
    api
      .post<Media>(
        `/media/${id}/generated-artwork`,
        { timestamp_seconds: timestampSeconds },
        { timeout: LONG_REQUEST_TIMEOUT },
      )
      .then((r) => r.data),

  manualScrapeSearch: (id: string, params: { query: string; provider?: string; media_type?: string }) =>
    api
      .get<{ items: ManualScrapeCandidate[] }>(`/media/${id}/scrape/search`, { params })
      .then((r) => r.data.items),

  applyManualScrape: (id: string, match: ManualScrapeCandidate) =>
    api
      .post<Media>(
        `/media/${id}/scrape/apply`,
        match,
        { timeout: LONG_REQUEST_TIMEOUT },
      )
      .then((r) => r.data),

  applyManualScrapeBatch: (mediaIDs: string[], match: ManualScrapeCandidate) =>
    api
      .post<{ applied: number; errors?: string[] }>(
        '/media/scrape/apply',
        { media_ids: mediaIDs, match },
        { timeout: BATCH_REQUEST_TIMEOUT },
      )
      .then((r) => r.data),
}
