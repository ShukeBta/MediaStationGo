import { api } from './client'
import type { DownloadTask, QBitTorrent } from '../types'

export interface DownloadsState {
  tasks: DownloadTask[]
  torrents: QBitTorrent[] | null
}

export interface AddDownloadInput {
  url: string
  save_path?: string
  title?: string
  poster_url?: string
  backdrop_url?: string
  overview?: string
  media_type?: string
  media_category?: string
  source_category?: string
}

export const downloadsAPI = {
  list: () => api.get<DownloadsState>('/downloads').then((r) => r.data),

  add: (url: string, savePath = '', meta: Omit<AddDownloadInput, 'url' | 'save_path'> = {}) =>
    api
      .post<DownloadTask>('/downloads', { url, save_path: savePath, ...meta })
      .then((r) => r.data),

  remove: (hash: string, clientID: string, deleteFiles = false) =>
    api
      .delete(`/downloads/${hash}`, { params: { client_id: clientID, delete_files: deleteFiles } })
      .then((r) => r.data),

  reload: () => api.post('/downloads/reload').then((r) => r.data),
}
