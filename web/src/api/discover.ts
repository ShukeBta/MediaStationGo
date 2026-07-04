import { api } from './client'
import type { Media } from '../types'

// TMDb-derived "Match" rows used by trending/popular rails. We re-use the
// Media interface — only TMDb id / poster / overview are populated.
export interface DiscoverItem extends Partial<Media> {
  source?: string
  media_type?: string
  tmdb_id?: number
  douban_id?: string
  bangumi_id?: number
  title: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  year?: number
  rating?: number
  original_title?: string
  original_language?: string
  genres?: string
  subscribe_keyword?: string
  subscribe_aliases?: string[]
  total_episodes?: number
  downloaded_episodes?: number
  local_media_count?: number
  missing_episodes?: number[]
  in_library?: boolean
}

export interface DiscoverSection {
  key: string
  label: string
  provider?: string
}

export interface DiscoverFeedMeta {
  page: number
  has_next: boolean
  duration_ms?: number
  error?: string
  warning?: string
  stale?: boolean
  disabled?: boolean
}

export interface DiscoverFeedResult {
  items: Record<string, DiscoverItem[]>
  meta: Record<string, DiscoverFeedMeta>
}

// 后端在 TMDb 不可达 / API key 缺失时统一返回 { items: [], error: "..." }
// 200 状态码——前端必须能区分这两种情况，不能简单用 items.length === 0
// 推断"未配置 API key"。
export interface DiscoverResp {
  items: DiscoverItem[]
  error?: string
}

export const discoverAPI = {
  trending: () =>
    api.get<DiscoverResp>('/discover/trending').then((r) => ({
      items: r.data.items ?? [],
      error: r.data.error,
    })),
  popular: () =>
    api.get<DiscoverResp>('/discover/popular').then((r) => ({
      items: r.data.items ?? [],
      error: r.data.error,
    })),
  sections: () =>
    api.get<{ sections: DiscoverSection[] }>('/discover/sections').then((r) => r.data.sections),
  search: (query: string, source = "all", mediaType = "", page = 1, pageSize = 40) =>
    api
      .get<DiscoverResp>("/discover/search", {
        params: { q: query, source, media_type: mediaType, page, page_size: pageSize },
      })
      .then((r) => ({ items: r.data.items ?? [], error: r.data.error })),
  feedPage: (sectionKeys: string[], page = 1, pageSize = 40): Promise<Record<string, DiscoverItem[]>> =>
    discoverAPI.feed(sectionKeys, page, pageSize).then((r) => r.items),
  feed: (sectionKeys: string[], page = 1, pageSize = 40): Promise<DiscoverFeedResult> =>
    api
      .get<Record<string, DiscoverItem[] | DiscoverFeedMeta | Record<string, DiscoverFeedMeta> | null>>('/discover/feed', {
        params: { sections: sectionKeys.join(','), page, page_size: pageSize },
      })
      .then((r) => {
        const raw = r.data
        const meta = ((raw._meta as Record<string, DiscoverFeedMeta> | undefined) ?? {})
        const items: Record<string, DiscoverItem[]> = {}
        for (const key of sectionKeys) {
          const row = raw[key]
          items[key] = Array.isArray(row) ? row : []
        }
        return { items, meta }
      }),
}
