import { api, BATCH_REQUEST_TIMEOUT, LONG_REQUEST_TIMEOUT } from './client'
import type { Library, LibraryRoot, Media, ScanResult } from '../types'
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
  nsfw?: boolean
}

export interface ScrapeOptions {
  episode_artwork?: boolean
  episode_images?: boolean
  refresh_matched?: boolean
  include_matched?: boolean
}

export type ManualScrapeApplyOptions = ScrapeOptions

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

  create: (name: string, path: string, type: string) =>
    api.post<Library>('/libraries', { name, path, type }).then((r) => r.data),

  createWithRoots: (name: string, type: string, roots: LibraryRootInput[], coverURL = '') =>
    api.post<Library>('/libraries', { name, type, roots, cover_url: coverURL }).then((r) => r.data),

  update: (id: string, payload: { cover_url: string }) =>
    api.patch<Library>(`/libraries/${id}`, payload).then((r) => r.data),

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
}

export const mediaAPI = {
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

  get: (id: string) => api.get<Media>(`/media/${id}`).then((r) => r.data),

  updateMetadata: (id: string, payload: MediaMetadataUpdate) =>
    api.patch<Media>(`/media/${id}/metadata`, payload, { timeout: LONG_REQUEST_TIMEOUT }).then((r) => r.data),

  manualScrapeSearch: (id: string, params: { query: string; provider?: string; media_type?: string }) =>
    api
      .get<{ items: ManualScrapeCandidate[] }>(`/media/${id}/scrape/search`, { params })
      .then((r) => r.data.items),

  applyManualScrape: (id: string, match: ManualScrapeCandidate, options?: ManualScrapeApplyOptions) =>
    api
      .post<Media>(
        `/media/${id}/scrape/apply`,
        episodeImageOption(options) === undefined ? match : { ...match, episode_images: episodeImageOption(options) },
        { timeout: LONG_REQUEST_TIMEOUT },
      )
      .then((r) => r.data),

  applyManualScrapeBatch: (mediaIDs: string[], match: ManualScrapeCandidate, options?: ManualScrapeApplyOptions) =>
    api
      .post<{ applied: number; errors?: string[] }>(
        '/media/scrape/apply',
        { media_ids: mediaIDs, match, episode_images: episodeImageOption(options) },
        { timeout: BATCH_REQUEST_TIMEOUT },
      )
      .then((r) => r.data),
}

function episodeImageOption(options?: ScrapeOptions): boolean | undefined {
  return options?.episode_images ?? options?.episode_artwork
}
