import { api } from './client'

// toolsAPI groups admin-only endpoints that don't fit the other domain
// modules: organizing media files into the canonical naming layout, and
// dispatching a test notification through the configured channels.
// OrganizeOverrides are optional single-request overrides for an organize
// action. Empty fields fall back to the system settings.
// source_path = 源目录（待整理文件所在），dest_path = 目的地目录（整理输出到哪里）。
export interface OrganizeOverrides {
  source_path?: string
  dest_path?: string
  transfer_mode?: string
}

export const toolsAPI = {
  organizeMedia: (mediaID: string, opts?: OrganizeOverrides) =>
    api
      .post<{ path: string }>(`/admin/media/${mediaID}/organize`, opts ?? {})
      .then((r) => r.data),

  organizeLibrary: (libraryID: string, opts?: OrganizeOverrides) =>
    api
      .post<Record<string, unknown>>(
        `/admin/libraries/${libraryID}/organize`,
        opts ?? {},
      )
      .then((r) => r.data),

  notifyTest: (title: string, body: string) =>
    api
      .post<{ message: string }>('/admin/notify/test', { title, body })
      .then((r) => r.data),
}
