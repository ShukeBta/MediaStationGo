import { api } from './client'

export type PlaybackDetail = { id: string; played_at: string; title: string; media_id: string; user_name: string; library_name: string; client?: string; season_num?: number; episode_num?: number }
export type PlaybackStats = { total: number; buckets: { period: string; count: number }[]; items: PlaybackDetail[]; ranking: { work_key: string; title: string; media_id: string; season_num: number; count: number }[]; page: number; page_size: number; time_zone: string }
export type PlayerRequestLog = { id: string; requested_at: string; route: string; method: string; status: number; duration_ms: number; ip: string; user_id?: string; query: string }
export const playbackStatsAPI = {
  stats: async (params: Record<string, string | number>, signal?: AbortSignal) => (await api.get<PlaybackStats>('/admin/playback-stats', { params, signal })).data,
  requests: async (params: Record<string, string | number>, signal?: AbortSignal) => (await api.get<{ items: PlayerRequestLog[]; total: number; dropped: number }>('/admin/player-request-logs', { params, signal })).data,
}
