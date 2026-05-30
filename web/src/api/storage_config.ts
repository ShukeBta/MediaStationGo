import { api } from './client'

export type StorageType = 'alist' | 's3' | 'webdav' | 'cloud115' | 'quark'

export interface CloudEntry {
  id: string
  name: string
  is_dir: boolean
  size: number
  pick_code?: string
}

export interface QRSession {
  uid: string
  time: number
  sign: string
  qr_image_url: string
}

export interface QRStatus {
  state: 'waiting' | 'scanned' | 'confirmed' | 'expired'
  cookie?: string
}

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

// cloudAPI drives 网盘 browsing, QR login and 302 import.
export const cloudAPI = {
  list: (type: StorageType, dir = '') =>
    api
      .get<{ items: CloudEntry[]; error?: string }>(`/admin/cloud/${type}/list`, {
        params: { dir },
      })
      .then((r) => r.data),

  import: (type: StorageType, ref: string, name: string, size: number) =>
    api
      .post(`/admin/cloud/${type}/import`, { ref, name, size })
      .then((r) => r.data),

  qrStart: (type: StorageType) =>
    api.post<QRSession>(`/admin/cloud/${type}/qr/start`).then((r) => r.data),

  qrPoll: (type: StorageType, sess: QRSession) =>
    api.post<QRStatus>(`/admin/cloud/${type}/qr/poll`, sess).then((r) => r.data),
}
