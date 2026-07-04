import type { DiscoverItem, DiscoverSection } from '../api/discover'

export const defaultSections = [
  'tmdb_trending_day',
  'tmdb_latest_movie',
  'tmdb_latest_tv',
  'douban_hot_movie',
  'douban_hot_tv',
  'bangumi_calendar',
]

export const discoverStorageKey = 'mediastation.discover.sections'
export const discoverRowsStorageKey = 'mediastation.discover.rows'
const discoverStorageVersion = 3
const discoverRowsStorageVersion = 1
const discoverRowsCacheMaxAgeMs = 6 * 60 * 60 * 1000
const legacyDefaultAdditions = ['tmdb_latest_movie', 'tmdb_latest_tv']

interface CachedDiscoverRow {
  page: number
  has_next: boolean
  items: DiscoverItem[]
}

interface CachedDiscoverRowsPayload {
  version: number
  saved_at: number
  rows: Record<string, CachedDiscoverRow>
}

export const defaultSectionDefs: DiscoverSection[] = [
  { key: 'tmdb_trending_day', label: 'TMDb 今日趋势', provider: 'tmdb' },
  { key: 'tmdb_latest_movie', label: 'TMDb 最新电影', provider: 'tmdb' },
  { key: 'tmdb_latest_tv', label: 'TMDb 最新剧集', provider: 'tmdb' },
  { key: 'tmdb_popular_movie', label: 'TMDb 热门电影', provider: 'tmdb' },
  { key: 'douban_hot_movie', label: '豆瓣热门电影', provider: 'douban' },
  { key: 'douban_hot_tv', label: '豆瓣热门剧集', provider: 'douban' },
  { key: 'bangumi_calendar', label: 'Bangumi 每日放送', provider: 'bangumi' },
]

export function discoverItemSource(item: DiscoverItem): string {
  return item.source || (item.bangumi_id ? 'bangumi' : item.douban_id ? 'douban' : 'tmdb')
}

export function readSavedSections(sections: DiscoverSection[]): string[] {
  try {
    const raw = window.localStorage.getItem(discoverStorageKey)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    const allowed = new Set(sections.map((section) => section.key))
    if (Array.isArray(parsed)) {
      return orderSectionKeys(addLegacyDefaults(parsed, allowed), sections)
    }
    if (!parsed || !Array.isArray(parsed.selected)) return []
    const selected = sanitizeSectionKeys(parsed.selected, allowed)
    if (parsed.version === discoverStorageVersion) {
      return orderSectionKeys(selected, sections)
    }
    return orderSectionKeys(addLegacyDefaults(selected, allowed), sections)
  } catch {
    return []
  }
}

export function serializeSavedSections(selected: string[]): string {
  return JSON.stringify({ version: discoverStorageVersion, selected })
}

export function readCachedDiscoverRows(selected: string[]): {
  rows: Record<string, DiscoverItem[]>
  rowCanNext: Record<string, boolean>
} {
  try {
    const raw = window.localStorage.getItem(discoverRowsStorageKey)
    if (!raw) return { rows: {}, rowCanNext: {} }
    const parsed = JSON.parse(raw) as Partial<CachedDiscoverRowsPayload>
    if (
      parsed.version !== discoverRowsStorageVersion ||
      typeof parsed.saved_at !== 'number' ||
      Date.now() - parsed.saved_at > discoverRowsCacheMaxAgeMs ||
      !parsed.rows
    ) {
      return { rows: {}, rowCanNext: {} }
    }
    const allowed = new Set(selected)
    const rows: Record<string, DiscoverItem[]> = {}
    const rowCanNext: Record<string, boolean> = {}
    for (const [key, row] of Object.entries(parsed.rows)) {
      if (!allowed.has(key) || row.page !== 1 || !Array.isArray(row.items) || row.items.length === 0) {
        continue
      }
      rows[key] = row.items
      rowCanNext[key] = Boolean(row.has_next)
    }
    return { rows, rowCanNext }
  } catch {
    return { rows: {}, rowCanNext: {} }
  }
}

export function writeCachedDiscoverRow(
  key: string,
  page: number,
  items: DiscoverItem[],
  hasNext: boolean,
) {
  if (page !== 1 || items.length === 0) return
  try {
    const current = readRawDiscoverRowsCache()
    current.rows[key] = {
      page,
      has_next: hasNext,
      items,
    }
    current.saved_at = Date.now()
    window.localStorage.setItem(discoverRowsStorageKey, JSON.stringify(current))
  } catch {
    // Best-effort UI cache only; failing to persist should never break Discover.
  }
}

function readRawDiscoverRowsCache(): CachedDiscoverRowsPayload {
  try {
    const raw = window.localStorage.getItem(discoverRowsStorageKey)
    if (!raw) return emptyDiscoverRowsCache()
    const parsed = JSON.parse(raw) as Partial<CachedDiscoverRowsPayload>
    if (parsed.version !== discoverRowsStorageVersion || !parsed.rows) {
      return emptyDiscoverRowsCache()
    }
    return {
      version: discoverRowsStorageVersion,
      saved_at: typeof parsed.saved_at === 'number' ? parsed.saved_at : Date.now(),
      rows: parsed.rows,
    }
  } catch {
    return emptyDiscoverRowsCache()
  }
}

function emptyDiscoverRowsCache(): CachedDiscoverRowsPayload {
  return {
    version: discoverRowsStorageVersion,
    saved_at: Date.now(),
    rows: {},
  }
}

function sanitizeSectionKeys(keys: unknown[], allowed: Set<string>): string[] {
  return keys.filter((key): key is string => typeof key === 'string' && allowed.has(key))
}

function addLegacyDefaults(keys: unknown[], allowed: Set<string>): string[] {
  const out = sanitizeSectionKeys(keys, allowed)
  for (const key of legacyDefaultAdditions) {
    if (allowed.has(key) && !out.includes(key)) {
      out.push(key)
    }
  }
  return out
}

function orderSectionKeys(keys: string[], sections: DiscoverSection[]): string[] {
  const selected = new Set(keys)
  return sections.map((section) => section.key).filter((key) => selected.has(key))
}

export function buildSubscribeKeyword(item: DiscoverItem): string {
  return [item.title, item.year && item.year > 0 ? item.year : ''].filter(Boolean).join(' ')
}
