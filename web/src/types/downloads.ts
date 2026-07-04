export interface DownloadTask {
  id: string
  source: string
  title: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  save_path: string
  status: string
  progress: number
  state?: string
  dlspeed?: number
  upspeed?: number
  num_seeds?: number
  num_leechs?: number
  size?: number
  downloaded?: number
  created_at: string
  updated_at: string
}

export interface QBitTorrent {
  hash: string
  name: string
  title: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  state: string
  progress: number
  dlspeed: number
  upspeed: number
  num_seeds: number
  num_leechs: number
  size: number
  downloaded: number
  save_path: string
}

export interface DownloadClient {
  id: string
  name: string
  type: 'qbittorrent' | 'transmission' | 'aria2'
  host: string
  username: string
  is_default: boolean
  enabled: boolean
  created_at: string
  updated_at: string
}

export interface DownloadClientTypeInfo {
  type: string
  name: string
  description: string
}
