import { api } from './client'

export interface MediaTitleCleanupSuggestion {
  media_id: string
  current_title?: string
  source_directory?: string
  filename?: string
  title: string
  year?: number
  confidence: number
  reason?: string
}

export interface MediaTitleCleanupPreview {
  library_id: string
  candidate_count: number
  batch_count: number
  remaining_count: number
  suggestions: MediaTitleCleanupSuggestion[]
}

export interface MediaTitleCleanupJob {
  id: string
  library_id: string
  status: 'queued' | 'running' | 'completed' | 'failed'
  stage: 'queued' | 'preparing' | 'cleaning' | 'validating' | 'completed' | 'failed'
  message: string
  progress: number
  completed_groups: number
  total_groups: number
  preview?: MediaTitleCleanupPreview
  error?: string
  started_at: string
  updated_at: string
  finished_at?: string
}

export const titleCleanupAPI = {
  startPreview: (libraryID: string, groupLimit = 5) =>
    api
      .post<MediaTitleCleanupJob>(`/libraries/${libraryID}/title-cleanup/preview`, {
        group_limit: groupLimit,
      })
      .then((response) => response.data),

  previewStatus: (libraryID: string, jobID: string) =>
    api
      .get<MediaTitleCleanupJob>(`/libraries/${libraryID}/title-cleanup/preview/${jobID}`)
      .then((response) => response.data),

  apply: (libraryID: string, items: MediaTitleCleanupSuggestion[]) =>
    api
      .post<{ updated: number }>(`/libraries/${libraryID}/title-cleanup/apply`, { items })
      .then((response) => response.data),
}
