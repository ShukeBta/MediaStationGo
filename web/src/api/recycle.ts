import { api } from './client'
import type { Media } from '../types'

export interface RecycleBinItem extends Media {
  deleted_at: string
  deletion_kind: 'media' | 'version'
  can_manage: boolean
}

export const recycleAPI = {
  list: () => api.get<{ items: RecycleBinItem[] }>('/recycle').then((r) => r.data.items),

  softDelete: (id: string) => api.delete(`/media/${id}`).then((r) => r.data),

  softDeleteMany: (ids: string[]) =>
    api.post<{ applied: number }>('/recycle/soft-delete', { media_ids: ids }).then((r) => r.data),

  restore: (id: string) => api.post(`/media/${id}/restore`).then((r) => r.data),

  restoreMany: (ids: string[]) =>
    api.post<{ applied: number; errors?: string[]; failed_ids?: string[] }>('/recycle/restore', { media_ids: ids }).then((r) => r.data),

  purge: (id: string) => api.delete(`/media/${id}/purge`).then((r) => r.data),

  purgeMany: (ids: string[]) =>
    api.post<{ applied: number; errors?: string[]; failed_ids?: string[] }>('/recycle/purge', { media_ids: ids }).then((r) => r.data),

  exportNFO: (id: string) =>
    api.post<{ path: string }>(`/media/${id}/nfo`).then((r) => r.data),

  exportLibraryNFO: (id: string) =>
    api.post<{ written: number }>(`/libraries/${id}/nfo`).then((r) => r.data),
}
