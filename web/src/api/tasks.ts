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

export interface StartupStatus {
  state: 'starting' | 'ready' | 'failed'
  stage: string
  elapsed_seconds: number
  stage_elapsed_seconds: number
  directories_found: number
  directories_watched: number
  warnings: string[]
}

export const tasksAPI = {
	startup: () => api.get<StartupStatus>('/tasks/startup').then((r) => r.data),
  snapshot: () => api.get<TasksSnapshot>('/tasks').then((r) => r.data),
  definitions: () => api.get<TaskDefinition[]>('/tasks/definitions').then(r => r.data),
  run: (key: string) => api.post(`/tasks/definitions/${encodeURIComponent(key)}/run`).then(r => r.data),
  history: (params: TaskFilter) => api.get<TaskPage<BackgroundTask>>('/tasks/history', { params }).then(r => r.data),
  logDays: (params: TaskFilter) => api.get<TaskLogDay[]>('/tasks/log-days', { params }).then(r => r.data),
  logs: (params: TaskFilter) => api.get<TaskPage<TaskLog>>('/tasks/logs', { params }).then(r => r.data),
  pending: (library_id: string, page: number) => api.get<TaskPage<PendingScrape>>('/tasks/scrape-pending', { params: { library_id, page, page_size: 25 } }).then(r => r.data),
  scrape: (id: string) => api.post(`/media/${encodeURIComponent(id)}/scrape`, {}, { timeout: 120000 }).then(r => r.data),
}

export interface TaskDefinition { key: string; name: string; trigger: string; schedule?: string; running: boolean; manual: boolean; last_error?: string }
export interface TaskPage<T> { items: T[]; total: number; page: number; page_size: number }
export interface TaskFilter { from: string; to: string; kind?: string; task_id?: string; status?: string; page?: number; page_size?: number }
export interface TaskLogDay { day: string; count: number }
export interface TaskLog { id: string; task_id: string; kind: string; logged_at: string; level: string; message: string }
export interface PendingScrape { id: string; title: string; library_id: string; scrape_status: string; is_strm: boolean }
