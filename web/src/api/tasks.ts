import { api } from './client'
import type { QBitTorrent } from '../types'

export interface ActiveTranscode {
  media_id: string
  encoder: string
  started_at: string
  playlist_ok: boolean
}

export type TaskItemStatus = 'pending' | 'running' | 'succeeded' | 'failed'

export interface TaskItem {
  id: string
  kind: string // organize | scan | scrape
  status: TaskItemStatus
  name: string
  source?: string
  dest_path?: string
  library_id?: string
  error?: string
}

export interface BackgroundTask {
  id: string
  kind: string
  name: string
  status: 'running' | 'completed' | 'failed'
  stage?: string
  source_path?: string
  dest_path?: string
  message?: string
  error?: string
  details?: string[]
  metrics?: Record<string, number>
  items?: TaskItem[]
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
