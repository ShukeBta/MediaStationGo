import { api } from './client'

export interface APIConfig {
  id: string
  provider: string
  base_url?: string
  extra?: string
  enabled: boolean
  description?: string
  has_key: boolean
  masked_key?: string
  created_at: string
  updated_at: string
}

export interface APIConfigPatch {
  api_key?: string
  base_url?: string
  extra?: string
  enabled?: boolean
  description?: string
}

export const apiConfigsAPI = {
  list: () => api.get<{ items: APIConfig[] }>('/admin/api-configs').then((r) => r.data.items),
  get: (provider: string) => api.get<APIConfig>(`/admin/api-configs/${provider}`).then((r) => r.data),
  update: (provider: string, patch: APIConfigPatch) =>
    api.put<APIConfig>(`/admin/api-configs/${provider}`, patch).then((r) => r.data),
  remove: (provider: string) => api.delete(`/admin/api-configs/${provider}`).then((r) => r.data),
  test: (provider: string) =>
    api.post<{ result: string; error?: string }>(`/admin/api-configs/${provider}/test`).then((r) => r.data),
}
