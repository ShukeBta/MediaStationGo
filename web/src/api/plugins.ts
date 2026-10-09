import { api, LONG_REQUEST_TIMEOUT } from './client'

export interface PluginRunResult {
  summary: string
  metrics: { label: string; value: number }[]
  completed_at: string
  duration_ms: number
}

export interface PluginInfo {
  id: string
  name: string
  description: string
  version: string
  author: string
  capabilities: string[]
  config_fields: { key: string; label: string; description: string; type: 'boolean'; default: boolean }[]
  enabled: boolean
  config: Record<string, unknown>
  status: 'disabled' | 'ready' | 'running' | 'error'
  last_run: PluginRunResult | null
  last_error: string
}

export interface PluginUpdate {
  enabled?: boolean
  config?: Record<string, unknown>
}

export const pluginsAPI = {
  list: () => api.get<{ items: PluginInfo[] }>('/admin/plugins').then((r) => r.data.items),
  update: (id: string, input: PluginUpdate) =>
    api.patch<PluginInfo>(`/admin/plugins/${encodeURIComponent(id)}`, input).then((r) => r.data),
  run: (id: string) =>
    api.post<PluginRunResult>(`/admin/plugins/${encodeURIComponent(id)}/run`, undefined, {
      timeout: LONG_REQUEST_TIMEOUT,
    }).then((r) => r.data),
}
