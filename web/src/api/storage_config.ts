import { api, BATCH_REQUEST_TIMEOUT, LONG_REQUEST_TIMEOUT } from './client'

export type StorageType = 'alist' | 'openlist' | 'webdav' | 'cloud115' | 'clouddrive2'

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

export interface CloudUploadResult {
  source_path: string
  dest_path: string
  uploaded: number
  moved?: number
  skipped: number
  bytes: number
  errors?: string[]
  items?: Array<{
    source: string
    target: string
    action: 'upload' | 'move' | 'skip' | 'error'
    size?: number
    reason?: string
  }>
}

export interface CloudScanStatus {
  library_id: string
  provider: string
  stage: string
  state: string
  dirs: number
  discovered: number
  visited: number
  added: number
  updated: number
  skipped: number
  removed: number
  error?: string
  resume_hint?: string
  estimate_message?: string
  files_per_second?: number
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

  logout: (type: StorageType) =>
    api
      .post<StorageConfig>(`/admin/storage/${type}/logout`)
      .then((r) => r.data),

  test: (type: StorageType, config: Record<string, string>) =>
    api
      .post<{ ok: boolean; error?: string }>(`/admin/storage/${type}/test`, {
        type,
        config,
      }, {
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  uploadLocal: (
    type: StorageType,
    input: {
      source_path: string
      dest_path: string
      recursive: boolean
      include_sidecars: boolean
      overwrite: boolean
      transfer_mode?: 'copy' | 'move'
    },
  ) =>
    api
      .post<{ result: CloudUploadResult; error?: string }>(`/admin/storage/${type}/upload-local`, input, {
        timeout: BATCH_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  scanAllCloud: () =>
    api
      .post<{ items: CloudScanStatus[]; message?: string; estimate_message?: string }>('/admin/cloud/scan-all')
      .then((r) => r.data),

  cancelCloudScan: (libraryID = '', provider = '') =>
    api
      .post<{ cancelled: number; message?: string }>('/admin/cloud/scan/cancel', null, {
        params: libraryID ? { library_id: libraryID } : provider ? { provider } : undefined,
      })
      .then((r) => r.data),

  cloudScanStatus: () =>
    api
      .get<{ items: CloudScanStatus[] }>('/admin/cloud/scan/status')
      .then((r) => r.data),
}

// cloudAPI drives 网盘 browsing, QR login and 302 import.
export const cloudAPI = {
  list: (type: StorageType, dir = '') =>
    api
      .get<{ items: CloudEntry[]; error?: string }>(`/admin/cloud/${type}/list`, {
        params: { dir },
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  mkdir: (type: StorageType, dir: string, name: string) =>
    api
      .post<{ entry: CloudEntry }>(`/admin/cloud/${type}/mkdir`, { dir, name }, {
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  rename: (type: StorageType, ref: string, name: string) =>
    api
      .put<{ entry: CloudEntry }>(`/admin/cloud/${type}/rename`, { ref, name }, {
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  import: (type: StorageType, ref: string, name: string, size: number) =>
    api
      .post(`/admin/cloud/${type}/import`, { ref, name, size })
      .then((r) => r.data),

  mount: (type: StorageType, dir = '', name = '', media_type = 'movie', dir_path = '') =>
    api
      .post(`/admin/cloud/${type}/mount`, { dir, dir_path, name, media_type }, {
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((r) => r.data),

  qrStart: (type: StorageType) =>
    api.post<QRSession>(`/admin/cloud/${type}/qr/start`).then((r) => r.data),

  qrPoll: (type: StorageType, sess: QRSession) =>
    api.post<QRStatus>(`/admin/cloud/${type}/qr/poll`, sess).then((r) => r.data),
}
