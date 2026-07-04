import type { ManualScrapeCandidate } from '../api/library'
import type { Media } from '../types'

export interface ManualSearchProvider {
  value: string
  label: string
}

export const manualSearchProviders: ManualSearchProvider[] = [
  { value: 'tmdb', label: 'TMDb' },
  { value: 'douban', label: '豆瓣' },
  { value: 'bangumi', label: 'Bangumi' },
  { value: 'thetvdb', label: 'TheTVDB' },
  { value: 'adult', label: 'Adult / 番号' },
]

export function candidateKey(item: ManualScrapeCandidate): string {
  return `${item.source}:${item.tmdb_id || item.bangumi_id || item.douban_id || item.thetvdb_id || item.title}:${item.media_type || ''}`
}

export function manualSearchProvidersForSelection(selectedProviders: string[], query: string): string[] {
  const explicitProvider = providerFromQueryPrefix(query)
  if (explicitProvider) return [explicitProvider]
  return selectedProviders.length > 0 ? selectedProviders : manualSearchProviders.map((provider) => provider.value)
}

export function toggleProvider(current: string[], value: string): string[] {
  if (current.includes(value)) {
    return current.filter((item) => item !== value)
  }
  const next = [...current, value]
  return next.length === manualSearchProviders.length ? [] : next
}

export function mergeManualCandidates(
  current: ManualScrapeCandidate[],
  incoming: ManualScrapeCandidate[],
): ManualScrapeCandidate[] {
  if (incoming.length === 0) return current
  const byKey = new Map(current.map((item) => [candidateKey(item), item]))
  for (const item of incoming) {
    byKey.set(candidateKey(item), item)
  }
  return Array.from(byKey.values())
}

export function isEpisodeArtworkTarget(media: Media, mediaType?: string, targetCount = 1): boolean {
  const type = (mediaType || '').toLowerCase()
  return (
    type === 'tv' ||
    type === 'anime' ||
    type === 'variety' ||
    media.season_num > 0 ||
    media.episode_num > 0 ||
    targetCount > 1
  )
}

export function candidateIDText(item: ManualScrapeCandidate): string {
  const parts = [
    item.tmdb_id ? `TMDb ${item.tmdb_id}` : '',
    item.original_name && item.source === 'adult' ? `番号 ${item.original_name}` : '',
    item.douban_id ? `豆瓣 ${item.douban_id}` : '',
    item.bangumi_id ? `Bangumi ${item.bangumi_id}` : '',
    item.thetvdb_id ? `TheTVDB ${item.thetvdb_id}` : '',
    item.media_type ? item.media_type : '',
  ].filter(Boolean)
  return parts.join(' · ')
}

function providerFromQueryPrefix(query: string): string {
  const prefix = query.trim().toLowerCase().match(/^([a-z]+)\s*:/)?.[1]
  switch (prefix) {
    case 'tmdb':
      return 'tmdb'
    case 'douban':
      return 'douban'
    case 'bangumi':
    case 'bgm':
      return 'bangumi'
    case 'thetvdb':
    case 'tvdb':
      return 'thetvdb'
    case 'adult':
    case 'jav':
      return 'adult'
    default:
      return ''
  }
}
