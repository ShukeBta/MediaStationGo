import type { Media } from './media'

export interface HistoryItem {
  id: string
  user_id: string
  media_id: string
  position_ms: number
  duration_ms: number
  watched_at: string
  completed: boolean
  media?: Media
}

export interface HistoryStats {
  total: number
  completed: number
  watched_ms: number
  watched_hours: number
  last_watched?: string
}
