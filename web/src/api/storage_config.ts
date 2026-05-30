import { api } from './client'

export type StorageType = 'alist' | 's3' | 'webdav'

export interface StorageConfig {
  id: string
  type: StorageType
  config: Record<string, string>
  enabled: boolean
  last_error?: string
  created_at: string
  updated_at: string
}

export const storageAPI = {
  status: () =>
    api
      .get<{ items: StorageConfig[] }>('/admin/storage/status')
      .then((r) => r.data.items),

  get: (type: StorageType) =>
    api.get<StorageConfig>(`/admin/storage/${type}`).then((r) => r.data),

  save: (type: StorageType, config: Record<string, string>, enabled = true) =>
    api
      .put<StorageConfig>(`/admin/storage/${type}`, { type, config, enabled })
      .then((r) => r.data),

  test: (type: StorageType, config: Record<string, string>) =>
    api
      .post<{ ok: boolean; error?: string }>(`/admin/storage/${type}/test`, {
        type,
        config,
      })
      .then((r) => r.data),
}
