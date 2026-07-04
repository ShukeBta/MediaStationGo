import type { DiscoverItem } from '../api/discover'
import { buildSiteSearchFeedURL, buildSubscriptionAliases } from '../api/subscriptions'
import { buildSubscribeKeyword } from './discoverPageModel'
import { defaultSubscriptionExcludeWords } from './subscriptionFormModel'

export type DiscoverSubscriptionForm = {
  keyword: string
  search_mode: string
  imdb_id: string
  media_type: string
  resolution: string
  quality: string
  effects: string
  release_groups: string
  exclude_words: string
  wash_enabled: boolean
  wash_priority: string
  save_path: string
  media_category: string
  priority: number
  run_now: boolean
}

export function discoverSubscriptionKeyword(item: DiscoverItem): string {
  return item.subscribe_keyword || buildSubscribeKeyword(item)
}

export function initialDiscoverSubscriptionForm(item: DiscoverItem): DiscoverSubscriptionForm {
  return {
    keyword: discoverSubscriptionKeyword(item),
    search_mode: 'keyword',
    imdb_id: '',
    media_type: item.media_type || '',
    resolution: 'best',
    quality: '',
    effects: '',
    release_groups: '',
    exclude_words: defaultSubscriptionExcludeWords,
    wash_enabled: false,
    wash_priority: 'balanced',
    save_path: '',
    media_category: '',
    priority: 50,
    run_now: true,
  }
}

export function discoverItemMetaText(item: DiscoverItem): string {
  return [
    item.media_type,
    item.year && item.year > 0 ? item.year : '',
    item.rating ? `★ ${item.rating.toFixed(1)}` : '',
  ]
    .filter(Boolean)
    .join(' · ')
}

export function buildDiscoverSubscriptionInput(
  item: DiscoverItem,
  form: DiscoverSubscriptionForm,
  source: string,
) {
  const finalKeyword = form.keyword.trim() || discoverSubscriptionKeyword(item)
  return {
    name: `${item.title} 自动订阅`,
    feed_url: buildSiteSearchFeedURL(finalKeyword, source, buildSubscriptionAliases(item)),
    filter: finalKeyword,
    media_type: form.media_type || undefined,
    media_category: form.media_category || undefined,
    save_path: form.save_path || undefined,
    search_mode: form.search_mode,
    imdb_id: form.imdb_id || undefined,
    source,
    poster_url: item.poster_url || undefined,
    backdrop_url: item.backdrop_url || undefined,
    overview: item.overview || undefined,
    original_name: item.original_name || undefined,
    year: item.year || undefined,
    total_episodes: item.total_episodes || undefined,
    resolution: form.resolution === 'best' ? 'best' : form.resolution,
    quality: form.quality || undefined,
    effects: form.effects || undefined,
    release_groups: form.release_groups || undefined,
    exclude_words: form.exclude_words || undefined,
    wash_enabled: form.wash_enabled,
    wash_priority: form.wash_priority,
    priority: form.priority,
    enabled: true,
  }
}

export function apiErrorMessage(err: unknown, fallback: string): string {
  return (err as { response?: { data?: { error?: string } } })?.response?.data?.error ?? fallback
}
