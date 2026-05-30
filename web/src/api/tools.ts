import { api } from './client'

// toolsAPI groups admin-only endpoints that don't fit the other domain
// modules: organizing media files into the canonical naming layout, and
// dispatching a test notification through the configured channels.
// OrganizeOverrides are optional single-request overrides for an organize
// action. Empty fields fall back to the system settings.
export interface OrganizeOverrides {
  target_path?: string
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
