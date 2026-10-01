import type { SiteDownloadInput, SiteSearchResult } from '../api/sites'

export interface PTResourceMetadata {
  title?: string
  original_name?: string
  year?: number
  media_type?: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  season_number?: number
  total_episodes?: number
}

export function ptDownloadInput(item: SiteSearchResult, metadata: PTResourceMetadata = {}): SiteDownloadInput {
  return {
    site_id: item.site_id,
    id: item.id,
    title: item.title,
    download_url: item.download_url || undefined,
    torrent_url: item.torrent_url || undefined,
    source_category: item.category,
    media_type: metadata.media_type,
    poster_url: metadata.poster_url || item.poster_url,
    backdrop_url: metadata.backdrop_url || item.backdrop_url,
    overview: metadata.overview || item.overview,
  }
}

export function ptSubscriptionInput(keyword: string, metadata: PTResourceMetadata, season: string, total: string) {
  const series = ['tv', 'anime', 'variety'].includes(metadata.media_type || '')
  return {
    name: metadata.title?.trim() || keyword.trim(),
    keyword: keyword.trim(),
    original_title: metadata.original_name,
    year: metadata.year,
    media_type: metadata.media_type,
    poster_url: metadata.poster_url,
    backdrop_url: metadata.backdrop_url,
    overview: metadata.overview,
    season_number: series ? Math.max(1, Math.trunc(Number(season)) || 1) : undefined,
    total_episodes: series ? Math.max(0, Math.trunc(Number(total)) || 0) : undefined,
    poll_interval_minutes: 180,
    enabled: true,
  }
}

export function ptSubscriptionResultMessage(result: { queued?: number; run_error?: string }) {
  if (result.run_error) return `订阅已保存，首次执行失败：${result.run_error}`
  const queued = Number(result.queued || 0)
  return queued > 0 ? `已创建订阅并加入 ${queued} 个下载` : '已创建订阅，当前无新增下载，将按计划继续搜索'
}

export function ptResourceSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '大小未知'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** index).toFixed(1)} ${units[index]}`
}
