import { api } from './client'

export interface APIConfig {
  id: string
  provider: string
  base_url?: string
  extra?: string
  model?: string
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
  model?: string
  enabled?: boolean
  description?: string
}

export const apiConfigsAPI = {
  list: () => api.get<{ items: APIConfig[] }>('/admin/api-configs').then((r) => r.data.items),
  get: (provider: string) => api.get<APIConfig>(`/admin/api-configs/${provider}`).then((r) => r.data),
  update: (provider: string, patch: APIConfigPatch) =>
    api.put<APIConfig>(`/admin/api-configs/${provider}`, patch).then((r) => r.data),
  remove: (provider: string) => api.delete(`/admin/api-configs/${provider}`).then((r) => r.data),
  discoverModels: (provider: string, input: { api_key?: string; base_url?: string }) =>
    api
      .post<{ items: Array<{ id: string; owned_by?: string }> }>(`/admin/api-configs/${provider}/models`, input)
      .then((r) => r.data.items),
}
