import { api } from './client'

export interface TMDbCatalogItem {
  key: string
  title: string
  overview: string
  season_num: number
  episode_num: number
  release_date: string
  still_url: string
  poster_url: string
  fetched_at: string
  episode_count: number
  complete: boolean
}

export interface TMDbCatalogEpisode extends TMDbCatalogItem {
  available: boolean
  media_id?: string
}

export interface TMDbCatalogSeason extends TMDbCatalogItem {
  episodes: TMDbCatalogEpisode[]
  recheck?: { status: string; due_at: string; checked_at?: string; last_error?: string }
}

export interface TMDbSeriesCatalog {
  series?: TMDbCatalogItem
  seasons: TMDbCatalogSeason[]
}

export const tmdbCatalogAPI = {
  get: (mediaID: string) => api.get<TMDbSeriesCatalog>(`/media/${mediaID}/tmdb-catalog`).then((r) => r.data),
  refresh: (mediaID: string) => api.post<{ task_id: string }>(`/media/${mediaID}/tmdb-catalog/refresh`).then((r) => r.data),
}
