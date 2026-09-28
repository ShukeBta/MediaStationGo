import { api, LONG_REQUEST_TIMEOUT } from './client'

export type PersonSummary = {
  Id: string
  Name: string
  Type: string
  OriginalTitle?: string
  Overview?: string
  ImageURL?: string
  Role?: string
  PremiereDate?: string
  EndDate?: string
  Department?: string
  ProductionLocations?: string[]
  Aliases?: string[]
  ProviderIds?: Record<string, string>
  RecursiveItemCount?: number
}

export type PersonWork = { id: string; title: string; year: number; poster_url?: string; episode_title?: string }
export const peopleAPI = {
  search: async (q: string, offset = 0, signal?: AbortSignal) =>
    (await api.get<{ Items: PersonSummary[]; TotalRecordCount: number }>('/people', { params: { q, offset, limit: 24 }, signal })).data,
  get: async (id: string) => (await api.get<PersonSummary>(`/people/${encodeURIComponent(id)}`)).data,
  works: async (id: string, offset = 0) => (await api.get<{ items: PersonWork[]; total: number }>(`/people/${encodeURIComponent(id)}/works`, { params: { offset } })).data,
  refresh: async (id: string) => { await api.post(`/people/${encodeURIComponent(id)}/refresh`, {}, { timeout: LONG_REQUEST_TIMEOUT }) },
  translate: async (id: string) => { await api.post(`/people/${encodeURIComponent(id)}/translate`, {}, { timeout: LONG_REQUEST_TIMEOUT }) },
  media: async (id: string) => (await api.get<{ items: PersonSummary[] }>(`/media/${id}/people`)).data,
  translateMedia: async (id: string) => (await api.post<{ translated: number }>(`/media/${id}/people/translate`, {}, { timeout: LONG_REQUEST_TIMEOUT })).data,
  refreshMedia: async (id: string) => { await api.post(`/media/${id}/people/refresh`, {}, { timeout: LONG_REQUEST_TIMEOUT }) },
  settings: async () => (await api.get<{ enabled: boolean }>('/people/translation-settings')).data,
  saveSettings: async (enabled: boolean) => (await api.put<{ enabled: boolean }>('/people/translation-settings', { enabled })).data,
}

export function peopleErrorMessage(error: unknown): string {
  return (error as { response?: { data?: { error?: string } } })?.response?.data?.error ?? '人物资料加载失败'
}
