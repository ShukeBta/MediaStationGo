import type { Media } from './media'

export interface Hardware {
  cpu_percent: number
  memory_used: number
  memory_total: number
  disk_used: number
  disk_total: number
  go_version: string
  goroutines: number
}

export interface StatsSnapshot {
  libraries: number
  media_count: number
  users_count: number
  total_size_bytes: number
  total_seconds: number
  recently_added: Media[]
  hardware: Hardware
  generated_at: string
}
