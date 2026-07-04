import { api } from './client'
import type { Media } from '../types'

export interface SearchIntent {
  query: string
  year?: number
  genre?: string
  type?: string
  sort?: string
  language?: string
}

export interface ExternalMediaResult {
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
  subscribe_keyword: string
  subscribe_aliases?: string[]
  total_episodes?: number
  downloaded_episodes?: number
  local_media_count?: number
  missing_episodes?: number[]
  in_library?: boolean
}

export const aiAPI = {
  status: () =>
    api
      .get<{ enabled: boolean; provider: string; model: string }>('/ai/status')
      .then((r) => r.data),

  smartSearch: (query: string) =>
    api
      .post<{ intent: SearchIntent; items: Media[]; external_items: ExternalMediaResult[] }>(
        '/ai/search',
        { query },
      )
      .then((r) => r.data),

  recommend: () => api.get<{ titles: string[] }>('/ai/recommend').then((r) => r.data.titles),
}
