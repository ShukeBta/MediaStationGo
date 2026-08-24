import { api } from './client'
import { useAuthStore } from '../stores/auth'

export interface SubtitleTrack {
  lang: string
  label: string
  name: string
  path: string
  url: string
  codec: string
  source: string
  application_key: string
}

export interface CloudSubtitleMaterializeResult {
  status: 'success' | 'skipped' | 'failed'
  media_id: string
  provider?: string
  discovered: number
  cached: number
  removed: number
  reason?: string
  error?: string
}

export interface SubtitleSearchCandidate {
  candidate_id: string
  provider: string
  application_key: string
  can_apply?: boolean
  unavailable_reason?: string
  can_preview?: boolean
  preview_unavailable_reason?: string
  title: string
  filename: string
  language: string
  source_score: number
  rank: number
  media_id: string
  episode_key: string
  url: string
  subtitle_group: string
  source_type: string
  language_tags: string[]
  formats: string[]
  like_count: number
  download_count: number
  uploader: string
  uploaded_at: string
  uploaded_date: string
}

export interface SubtitleSearchResponse {
  session_id: string
  expires_at: number
  media_id: string
  title: string
  category: string
  query: string
  items: SubtitleSearchCandidate[]
}

export interface SubtitleSeasonSearchResponse extends Omit<SubtitleSearchResponse, 'season'> {
  season: number
  detail_url: string
  detail_title: string
}

export interface SubtitleSeasonEpisodeSelection {
  media_id: string
  episode_key: string
  candidate_id?: string
}

export interface SubtitleSeasonTaskDetail {
  media_id: string
  episode_key: string
  status: 'success' | 'skipped' | 'failed'
  count: number
  error: string
}

export interface SubtitleSeasonTask {
  id: string
  media_id: string
  season: number
  status: 'queued' | 'running' | 'completed' | 'failed'
  stage: 'queued' | 'download' | 'applying' | 'completed' | 'failed'
  progress_current: number
  progress_total: number
  succeeded: number
  skipped: number
  failed: number
  current_episode: string
  error: string
  created_at: number
  updated_at: number
  started_at: number
  completed_at: number
  retry_of: string
  details: SubtitleSeasonTaskDetail[]
}

export interface SubtitleCandidatePreview extends SubtitleSearchCandidate {
  media_id: string
  content_sample: string
  preview_char_count: number
  preview_line_count: number
}

export interface SubtitleApplyResult {
  media_id: string
  status: string
  source: string
  filename: string
  count: number
  reason: string
}

export type SubtitleASRSourceLanguage = 'auto' | 'ja' | 'en' | 'zh' | 'ko'

export interface SubtitleASRProfile {
  provider: 'local' | 'openai' | 'deepseek' | 'siliconflow'
  provider_label: string
  model: string
  local: boolean
}

export interface SubtitleASRResult {
  filename: string
  source: string
  language: string
  segment_count: number
  duration: number
  translation_provider?: string
  translation_model?: string
  asr_model?: string
}

export interface SubtitleASRTask {
  id: string
  owner_id: string
  media_id: string
  source_language: SubtitleASRSourceLanguage
  asr_model: string
  translation_provider: string
  translation_model: string
  status: 'queued' | 'running' | 'completed' | 'failed' | 'canceled'
  stage: 'queued' | 'starting' | 'extracting_audio' | 'using_cached_audio' | 'uploading_audio' | 'transcribing' | 'using_cached_transcript' | 'translating' | 'saving' | 'completed' | 'failed' | 'canceled'
  progress_current: number
  progress_total: number
  result: SubtitleASRResult | null
  error: string
  created_at: number
  updated_at: number
  started_at: number
  completed_at: number
  attempt_count: number
  cached_audio: boolean
  cached_transcript: boolean
  media_title?: string
  media_filename?: string
  media_available: boolean
}

export const subtitlesAPI = {
  list: (mediaId: string) =>
    api
      .get<{ tracks: SubtitleTrack[] | null }>(`/media/${mediaId}/subtitles`)
      .then((r) => r.data.tracks ?? []),

  previewExisting: (mediaId: string, path: string) =>
    api
      .get<string>(`/subtitles/${encodeURIComponent(mediaId)}`, {
        params: { path },
        responseType: 'text',
      })
      .then((r) => r.data),

  delete: (mediaId: string, path: string) =>
    api
      .delete<{ deleted: boolean }>(`/media/${encodeURIComponent(mediaId)}/subtitles`, { params: { path } })
      .then((r) => r.data),

  refreshCloud: (mediaId: string) =>
    api
      .post<CloudSubtitleMaterializeResult>(`/media/${encodeURIComponent(mediaId)}/subtitles/cloud-refresh`)
      .then((r) => r.data),

  search: (mediaId: string, limit = 20) =>
    api
      .post<SubtitleSearchResponse>(`/media/${encodeURIComponent(mediaId)}/subtitles/search`, { limit })
      .then((r) => r.data),

  searchSeason: (mediaId: string, season: number, title: string, episodes: SubtitleSeasonEpisodeSelection[], limit = 20) =>
    api
      .post<SubtitleSeasonSearchResponse>(`/media/${encodeURIComponent(mediaId)}/subtitles/season/search`, {
        season,
        title,
        limit,
        episodes,
      })
      .then((r) => r.data),

  applySeason: (mediaId: string, searchSessionId: string, season: number, episodes: SubtitleSeasonEpisodeSelection[]) =>
    api
      .post<SubtitleSeasonTask>(`/media/${encodeURIComponent(mediaId)}/subtitles/season/apply`, {
        search_session_id: searchSessionId,
        season,
        episodes,
      })
      .then((r) => r.data),

  getSeasonTask: (mediaId: string, taskId: string) =>
    api
      .get<SubtitleSeasonTask>(
        `/media/${encodeURIComponent(mediaId)}/subtitles/season/tasks/${encodeURIComponent(taskId)}`,
      )
      .then((r) => r.data),

  retrySeasonTask: (mediaId: string, taskId: string, mediaIds: string[] = []) =>
    api
      .post<SubtitleSeasonTask>(
        `/media/${encodeURIComponent(mediaId)}/subtitles/season/tasks/${encodeURIComponent(taskId)}/retry`,
        { media_ids: mediaIds },
      )
      .then((r) => r.data),

  previewCandidate: (mediaId: string, searchSessionId: string, candidateId: string) =>
    api
      .post<SubtitleCandidatePreview>(`/media/${encodeURIComponent(mediaId)}/subtitles/preview`, {
        search_session_id: searchSessionId,
        candidate_id: candidateId,
      })
      .then((r) => r.data),

  applyCandidate: (mediaId: string, searchSessionId: string, candidateId: string) =>
    api
      .post<{ result: SubtitleApplyResult; tracks: SubtitleTrack[] }>(
        `/media/${encodeURIComponent(mediaId)}/subtitles/apply`,
        { search_session_id: searchSessionId, candidate_id: candidateId },
      )
      .then((r) => r.data),

  createASR: (mediaId: string, sourceLanguage: SubtitleASRSourceLanguage, asrModel: string, profile: SubtitleASRProfile) =>
    api
      .post<SubtitleASRTask>(`/media/${encodeURIComponent(mediaId)}/subtitles/asr`, {
        source_language: sourceLanguage,
        asr_model: asrModel,
        translation_provider: profile.provider,
        translation_model: profile.model,
      })
      .then((r) => r.data),

  getASR: (mediaId: string, taskId: string) =>
    api
      .get<SubtitleASRTask>(
        `/media/${encodeURIComponent(mediaId)}/subtitles/asr/${encodeURIComponent(taskId)}`,
      )
      .then((r) => r.data),

  listASRTasks: () =>
    api
      .get<{ items: SubtitleASRTask[] | null }>('/subtitles/asr/tasks')
      .then((r) => r.data.items ?? []),

  listASRProfiles: () =>
    api
      .get<{ items: SubtitleASRProfile[] | null }>('/subtitles/asr/profiles')
      .then((r) => r.data.items ?? []),

  listASRModels: () =>
    api
      .get<{ items: string[] | null }>('/subtitles/asr/models')
      .then((r) => r.data.items ?? []),

  retryASR: (taskId: string, asrModel: string, profile: SubtitleASRProfile) =>
    api
      .post<SubtitleASRTask>(`/subtitles/asr/tasks/${encodeURIComponent(taskId)}/retry`, {
        asr_model: asrModel,
        translation_provider: profile.provider,
        translation_model: profile.model,
      })
      .then((r) => r.data),

  updateASRModel: (taskId: string, asrModel: string, profile: SubtitleASRProfile) =>
    api
      .post<SubtitleASRTask>(`/subtitles/asr/tasks/${encodeURIComponent(taskId)}/model`, {
        asr_model: asrModel,
        translation_provider: profile.provider,
        translation_model: profile.model,
      })
      .then((r) => r.data),

  cancelASR: (taskId: string) =>
    api
      .post<SubtitleASRTask>(`/subtitles/asr/tasks/${encodeURIComponent(taskId)}/cancel`)
      .then((r) => r.data),

  retranslateASR: (taskId: string, profile: SubtitleASRProfile) =>
    api
      .post<SubtitleASRTask>(`/subtitles/asr/tasks/${encodeURIComponent(taskId)}/retranslate`, {
        translation_provider: profile.provider,
        translation_model: profile.model,
      })
      .then((r) => r.data),

  deleteASR: (taskId: string) =>
    api
      .delete<{ deleted: boolean }>(`/subtitles/asr/tasks/${encodeURIComponent(taskId)}`)
      .then((r) => r.data),

  url: (mediaId: string, path: string) => {
    const token = useAuthStore.getState().token ?? ''
    return `/api/subtitles/${encodeURIComponent(mediaId)}?path=${encodeURIComponent(
      path,
    )}&token=${encodeURIComponent(token)}`
  },
}
