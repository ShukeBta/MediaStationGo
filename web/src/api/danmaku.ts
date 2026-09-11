import { api } from './client'

// 弹幕：服务端已经做好匹配/去重/屏蔽/密度采样与时间轴修正，
// 客户端只负责渲染。字段沿用弹弹play 的 cid/p/m，外加结构化补充字段。

export interface DanmakuComment {
  cid: string
  p: string
  m: string
  time: number
  mode: number
  mode_name?: string
  color: number
  size?: number
  user?: string
}

export interface DanmakuPayload {
  media_id: string
  source: string
  episode_id: string
  anime_title?: string
  episode_title?: string
  match_mode?: string
  provider_shift_seconds: number
  offset_seconds: number
  ch_convert: number
  count: number
  total: number
  filtered: number
  skipped: number
  truncated: boolean
  comments: DanmakuComment[]
}

export interface DanmakuAttempt {
  source: string
  mode: string
  outcome: string
  error?: string
  candidate_count?: number
}

export interface DanmakuMatchResult {
  matched: boolean
  source?: string
  match_mode?: string
  episode_id?: string
  anime_title?: string
  episode_title?: string
  shift: number
  status: 'matched' | 'unmatched' | 'failed'
  ambiguous?: boolean
  candidates: Array<{
    episode_id: string
    anime_title?: string
    episode_title?: string
    episode_number?: string
    type_description?: string
    shift: number
  }>
  attempts: DanmakuAttempt[]
}

export interface DanmakuUnavailable {
  code: 'danmaku_unavailable'
  error: string
}

/** 服务端明确说"没匹配到弹幕"，与"服务不可用"是两种状态，UI 要分开提示。 */
export class DanmakuUnmatchedError extends Error {
  attempts: DanmakuAttempt[]

  constructor(message: string, attempts: DanmakuAttempt[] = []) {
    super(message)
    this.name = 'DanmakuUnmatchedError'
    this.attempts = attempts
  }
}

export const danmakuAPI = {
  fetch: (mediaId: string, options: { chConvert?: number; offsetSeconds?: number; withRelated?: boolean } = {}) =>
    api
      .get<DanmakuPayload>(`/media/${encodeURIComponent(mediaId)}/danmaku`, {
        params: {
          ch_convert: options.chConvert ?? 0,
          with_related: options.withRelated === false ? 'false' : 'true',
          ...(options.offsetSeconds ? { offset_seconds: options.offsetSeconds } : {}),
        },
      })
      .then((r) => r.data),

  state: (mediaId: string) =>
    api.get<DanmakuMatchResult>(`/media/${encodeURIComponent(mediaId)}/danmaku/match`).then((r) => r.data),

  match: (mediaId: string) =>
    api.post<DanmakuMatchResult>(`/media/${encodeURIComponent(mediaId)}/danmaku/match`).then((r) => r.data),

  setManual: (
    mediaId: string,
    input: { episode_id: string; provider?: string; anime_title?: string; episode_title?: string; offset_seconds?: number },
  ) => api.patch(`/media/${encodeURIComponent(mediaId)}/danmaku`, input).then((r) => r.data),

  setOffset: (mediaId: string, offsetSeconds: number) =>
    api.patch(`/media/${encodeURIComponent(mediaId)}/danmaku`, { offset_seconds: offsetSeconds }).then((r) => r.data),

  clear: (mediaId: string) =>
    api.delete(`/media/${encodeURIComponent(mediaId)}/danmaku`).then((r) => r.data),

  search: (keyword: string, episode?: number) =>
    api
      .get(`/danmaku/search`, { params: { keyword, ...(episode ? { episode } : {}) } })
      .then((r) => r.data),
}

/** 把服务端错误翻译成 UI 能区分的状态。 */
export function describeDanmakuError(error: unknown): { kind: 'unavailable' | 'unmatched' | 'unknown'; message: string } {
  const response = (error as { response?: { status?: number; data?: { code?: string; error?: string } } })?.response
  const code = response?.data?.code
  const message = response?.data?.error || (error as Error)?.message || '弹幕加载失败'
  if (response?.status === 404 || code === 'danmaku_unmatched') {
    return { kind: 'unmatched', message }
  }
  if (response?.status === 503 || code === 'danmaku_unavailable') {
    return { kind: 'unavailable', message }
  }
  return { kind: 'unknown', message }
}
