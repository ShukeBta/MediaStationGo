import type { DiscoverItem, DiscoverSection } from '../api/discover'

export const defaultSections = [
  'tmdb_trending_day',
  'tmdb_latest_movie',
  'tmdb_latest_tv',
  'douban_hot_movie',
  'douban_hot_tv',
  'bangumi_calendar',
]

export const fd2PPVSortOptions = [
  { value: 'release', label: '发行日期' },
  { value: 'views', label: '观看数' },
  { value: 'likes', label: '喜欢数' },
  { value: 'favorites', label: '收藏数' },
  { value: 'comments', label: '留言数' },
] as const

export const discoverRowsStorageKey = 'mediastation.discover.rows'
const discoverRowsStorageVersion = 3
const discoverRowsCacheMaxAgeMs = 6 * 60 * 60 * 1000

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
	{ key: 'adult_fd2ppv', label: 'FC2 作品', provider: 'adult', group: 'adult' },
	{ key: 'adult_javdb_popular', label: 'JavDB 今日热门', provider: 'adult', group: 'adult' },
	{ key: 'adult_followed_performers', label: '关注女优', provider: 'adult', group: 'adult' },
	{ key: 'adult_followed', label: '关注女优新作', provider: 'adult', group: 'adult' },
	{ key: 'adult_javdb_performers_new', label: 'JavDB 新人女优', provider: 'adult', group: 'adult' },
	{ key: 'adult_javdb_performers_monthly', label: 'JavDB 月榜女优', provider: 'adult', group: 'adult' },
	{ key: 'adult_javdb_performers_fanza', label: 'JavDB Fanza(DMM)推薦', provider: 'adult', group: 'adult' },
]

export function discoverItemSource(item: DiscoverItem): string {
  return item.source || (item.bangumi_id ? 'bangumi' : item.douban_id ? 'douban' : 'tmdb')
}

export function discoverSourceLabel(source?: string): string {
  if (source?.trim().toLowerCase() === 'fd2ppv') return 'FC2'
  return source?.trim() || '推荐'
}

export function discoverMediaTypeLabel(mediaType?: string): string {
  switch (mediaType?.trim().toLowerCase()) {
    case 'movie':
      return '电影'
    case 'tv':
      return '剧集'
    case 'anime':
      return '动漫'
    case 'adult':
      return '成人作品'
    default:
      return mediaType?.trim() || '推荐'
  }
}

export function discoverCardMetaText(item: DiscoverItem): string {
  const releaseText = item.release_date?.trim() || (item.year && item.year > 0 ? String(item.year) : '')
  return [discoverMediaTypeLabel(item.media_type), releaseText].filter(Boolean).join(' · ')
}

export function discoverCardSecondaryText(item: DiscoverItem): string {
  if (item.media_type === 'adult' || item.media_type === 'person') return ''
  const originalName = item.original_name?.trim()
  if (originalName && originalName.toLowerCase() !== item.title.trim().toLowerCase()) {
    return originalName
  }
  if (item.rating && item.rating > 0) {
    const source = discoverItemSource(item).toLowerCase()
    const ratingLabel = source === 'douban' ? '豆瓣评分' : source === 'bangumi' ? 'Bangumi 评分' : '评分'
    return `${ratingLabel} ${item.rating.toFixed(1)}`
  }
  return item.overview?.trim() || ''
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

export function orderSelectedSections(keys: string[], sections: DiscoverSection[]): string[] {
  const available = new Set(sections.map((section) => section.key))
  const selected = new Set<string>()
  const ordered: string[] = []
  for (const value of keys) {
    const key = value.trim()
    if (!available.has(key) || selected.has(key)) continue
    selected.add(key)
    ordered.push(key)
  }
  return ordered
}

export function buildSubscribeKeyword(item: DiscoverItem): string {
  return [item.title, item.year && item.year > 0 ? item.year : ''].filter(Boolean).join(' ')
}
