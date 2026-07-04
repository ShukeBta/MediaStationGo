import type { DownloadTask, QBitTorrent } from '../types'

export type DownloadCardItem = {
  id?: string
  hash?: string
  title: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  save_path?: string
  status?: string
  state?: string
  progress: number
  dlspeed?: number
  upspeed?: number
  num_seeds?: number
  num_leechs?: number
  size?: number
  downloaded?: number
  created_at?: string
  updated_at?: string
}

export function toLiveCard(t: QBitTorrent): DownloadCardItem {
  return {
    hash: t.hash,
    title: t.title || t.name || '下载任务',
    poster_url: t.poster_url,
    backdrop_url: t.backdrop_url,
    overview: t.overview,
    save_path: t.save_path,
    state: t.state,
    progress: t.progress,
    dlspeed: t.dlspeed,
    upspeed: t.upspeed,
    num_seeds: t.num_seeds,
    num_leechs: t.num_leechs,
    size: t.size,
    downloaded: t.downloaded,
  }
}

export function toTaskCard(t: DownloadTask): DownloadCardItem {
  return {
    id: t.id,
    title: t.title || '下载任务',
    poster_url: t.poster_url,
    backdrop_url: t.backdrop_url,
    overview: t.overview,
    save_path: t.save_path,
    status: t.status,
    state: t.state,
    progress: t.progress,
    dlspeed: t.dlspeed,
    upspeed: t.upspeed,
    num_seeds: t.num_seeds,
    num_leechs: t.num_leechs,
    size: t.size,
    downloaded: t.downloaded,
    created_at: t.created_at,
    updated_at: t.updated_at,
  }
}
