import { api } from './client'
import type { Subscription } from '../types'

export function buildSiteSearchFeedURL(keyword: string, source?: string, aliases: string[] = []) {
  const params = new URLSearchParams()
  params.set('keyword', keyword)
  if (source) params.set('source', source)
  const seen = new Set([keyword.trim().toLowerCase()])
  aliases
    .map((alias) => alias.trim())
    .filter(Boolean)
    .forEach((alias) => {
      const key = alias.toLowerCase()
      if (seen.has(key)) return
      seen.add(key)
      params.append('alias', alias)
    })
  return `site-search://search?${params.toString()}`
}

export function buildResourceImportFeedURL(aliases: string[] = []) {
  const params = new URLSearchParams()
  aliases
    .map((alias) => alias.trim())
    .filter(Boolean)
    .forEach((alias) => params.append('alias', alias))
  const query = params.toString()
  return `resource-import://default${query ? `?${query}` : ''}`
}

export interface SubscriptionCreateInput {
  name: string
  feed_url?: string
  delivery_mode?: 'download' | 'resource_import'
  library_id?: string
  library_root_id?: string
  resource_source?: string
  max_imports_per_run?: number
  poll_interval_minutes?: number
  season_number?: number
  filter?: string
  media_type?: string
  media_category?: string
  save_path?: string
  search_mode?: string
  imdb_id?: string
  source?: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  original_name?: string
  year?: number
  resolution?: string
  quality?: string
  effects?: string
  release_groups?: string
  exclude_words?: string
  min_seeders?: number
  max_seeders?: number
  min_size_gb?: number
  max_size_gb?: number
  free_only?: boolean
  wash_enabled?: boolean
  wash_priority?: string
  total_episodes?: number
  priority?: number
  enabled?: boolean
}

export function buildSubscriptionAliases(item: {
  title?: string
  original_name?: string
  subscribe_keyword?: string
  subscribe_aliases?: string[]
  year?: number
}) {
  const withYear = (value?: string) => {
    const title = (value || '').trim()
    if (!title) return ''
    return item.year && item.year > 0 ? `${title} ${item.year}` : title
  }
  return [
    ...(item.subscribe_aliases || []),
    item.title || '',
    item.original_name || '',
    withYear(item.title),
    withYear(item.original_name),
    item.subscribe_keyword || '',
  ]
}

export const subscriptionsAPI = {
  list: () =>
    api
      .get<{ items: Subscription[] }>('/subscriptions', subscriptionListRequestConfig())
      .then((r) => r.data.items),

  history: () =>
    api
      .get<{ items: Subscription[] }>('/subscriptions/history', subscriptionListRequestConfig())
      .then((r) => r.data.items),

  create: (input: SubscriptionCreateInput) =>
    api.post<Subscription>('/subscriptions', input).then((r) => r.data),

  update: (id: string, input: Partial<Subscription>) =>
    api.put(`/subscriptions/${id}`, input).then((r) => r.data),

  remove: (id: string) => api.delete(`/subscriptions/${id}`).then((r) => r.data),

  purgeHistory: (id: string) => api.delete(`/subscriptions/${id}/history`).then((r) => r.data),

  restore: (id: string) =>
    api.post<Subscription>(`/subscriptions/${id}/restore`).then((r) => r.data),

  runNow: (id: string) =>
    api.post<{ queued: number }>(`/subscriptions/${id}/run`).then((r) => r.data),
}

function subscriptionListRequestConfig() {
  return {
    headers: { 'Cache-Control': 'no-cache' },
    params: { _ts: Date.now() },
  }
}
