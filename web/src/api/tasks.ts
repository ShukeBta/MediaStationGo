import { api } from './client'
import type { QBitTorrent } from '../types'

export interface ActiveTranscode {
  media_id: string
  encoder: string
  started_at: string
  playlist_ok: boolean
}

export interface BackgroundTask {
  id: string
  kind: string
  name: string
  status: 'running' | 'completed' | 'failed' | 'canceled'
  stage?: string
  source_path?: string
  dest_path?: string
  message?: string
  error?: string
  details?: string[]
  metrics?: Record<string, number>
  started_at: string
  updated_at: string
  finished_at?: string
}

export interface BackgroundTaskSnapshot {
  active: BackgroundTask[]
  recent: BackgroundTask[]
}

export interface TasksSnapshot {
  transcodes: ActiveTranscode[]
  torrents: QBitTorrent[] | null
  background_tasks?: BackgroundTaskSnapshot
}

export const tasksAPI = {
  snapshot: () => api.get<TasksSnapshot>('/tasks').then((r) => r.data),
}
