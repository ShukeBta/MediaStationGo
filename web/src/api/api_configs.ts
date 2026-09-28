import { api } from './client'

export interface APIConfig {
  id: string
  provider: string
  base_url?: string
  extra?: string
  model?: string
  image_direct?: boolean
  use_proxy_pool?: boolean
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
  image_direct?: boolean
  use_proxy_pool?: boolean
  enabled?: boolean
  description?: string
}

export interface ProxyPoolItem {
  id: string
  display_url: string
  has_auth: boolean
}

export interface ProxyPoolInput {
  id?: string
  url?: string
}

export interface ProxyPoolConfig {
  proxy_pool_type: 'normal' | 'resin'
  resin_proxy_url?: string
  resin_account?: string
  has_resin_proxy_token: boolean
}

export interface ProxyPoolConfigPatch {
  proxy_pool_type: 'normal' | 'resin'
  resin_proxy_url?: string
  resin_proxy_token?: string
  resin_account?: string
}

export interface ProxyPoolCheckResult {
  total: number
  available: number
  unavailable: number
  inconclusive: number
  cleanup_token?: string
}

export interface ProxyPoolCleanupResult {
  items: ProxyPoolItem[]
  removed: number
}

export const apiConfigsAPI = {
  list: () => api.get<{ items: APIConfig[] }>('/admin/api-configs').then((r) => r.data.items),
  get: (provider: string) => api.get<APIConfig>(`/admin/api-configs/${provider}`).then((r) => r.data),
  update: (provider: string, patch: APIConfigPatch) =>
    api.put<APIConfig>(`/admin/api-configs/${provider}`, patch).then((r) => r.data),
  remove: (provider: string) => api.delete(`/admin/api-configs/${provider}`).then((r) => r.data),
  listProxyPool: () =>
    api.get<{ items: ProxyPoolItem[] }>('/admin/api-proxy-pool').then((r) => r.data.items),
  replaceProxyPool: (items: ProxyPoolInput[]) =>
    api.put<{ items: ProxyPoolItem[] }>('/admin/api-proxy-pool', { items }).then((r) => r.data.items),
  getProxyPoolConfig: () =>
    api.get<ProxyPoolConfig>('/admin/api-proxy-pool/config').then((r) => r.data),
  updateProxyPoolConfig: (patch: ProxyPoolConfigPatch) =>
    api.put<ProxyPoolConfig>('/admin/api-proxy-pool/config', patch).then((r) => r.data),
  checkProxyPool: () =>
    api.post<ProxyPoolCheckResult>('/admin/api-proxy-pool/check', undefined, { timeout: 180_000 }).then((r) => r.data),
  cleanupProxyPool: (token: string) =>
    api.post<ProxyPoolCleanupResult>('/admin/api-proxy-pool/cleanup', { token }).then((r) => r.data),
  discoverModels: (provider: string, input: { api_key?: string; base_url?: string }) =>
    api
      .post<{ items: Array<{ id: string; owned_by?: string }> }>(`/admin/api-configs/${provider}/models`, input)
      .then((r) => r.data.items),
}
