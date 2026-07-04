import { api } from './client'

export interface RecognitionWordsConfig {
  enabled: boolean
  local_text: string
  shared_urls: string[]
  shared_text?: string
  synced_at?: string
  rule_count: number
}

export interface RecognitionWordsTestResult {
  input: string
  output: string
  title: string
  year: number
  changed: boolean
}

export const recognitionWordsAPI = {
  get: () => api.get<RecognitionWordsConfig>('/admin/recognition-words').then((r) => r.data),

  save: (payload: RecognitionWordsConfig) =>
    api.put<RecognitionWordsConfig>('/admin/recognition-words', payload).then((r) => r.data),

  sync: () => api.post<RecognitionWordsConfig>('/admin/recognition-words/sync').then((r) => r.data),

  test: (input: string) =>
    api.post<RecognitionWordsTestResult>('/admin/recognition-words/test', { input }).then((r) => r.data),
}
