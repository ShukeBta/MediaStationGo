import { api } from './client'
import type { ScrapeOptions } from './library'

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
  media_type?: string
  media_category?: string
  scan_after?: boolean
  scrape_after?: boolean
  library_id?: string
  dry_run?: boolean
}

// OrganizeSource is a selectable organize source directory (e.g. the download
// directory) surfaced so operators can organize an arbitrary directory and not
// only registered libraries.
export interface OrganizeSource {
  label: string
  path: string
  kind: string
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

  // organizeSources lists selectable source directories (download/media dir).
  organizeSources: () =>
    api
      .get<{ sources: OrganizeSource[] }>('/admin/organize/sources')
      .then((r) => r.data.sources ?? []),

  // organizeDirectory organizes an arbitrary source directory (e.g. downloads)
  // into the destination with dedup + 洗版 (resolution replacement).
  organizeDirectory: (opts: OrganizeOverrides) =>
    api
      .post<{
        organized: number
        skipped: number
        replaced?: number
        reclassified?: number
        source_path?: string
        dest_path?: string
        errors?: string[]
        dry_run?: boolean
        items?: Array<{
          source: string
          target?: string
          action: string
          reason?: string
          media_type?: string
          category?: string
          title?: string
        }>
        scans?: Array<{
          library_id: string
          name: string
          path: string
          visited: number
          added: number
          updated: number
          removed: number
          error?: string
        }>
        scrapes?: Array<{
          library_id: string
          name: string
          path: string
          matched: number
          skipped?: boolean
          reason?: string
          error?: string
        }>
      }>(
        '/admin/organize/source',
        opts,
      )
      .then((r) => r.data),

  notifyTest: (title: string, body: string) =>
    api
      .post<{ message: string }>('/admin/notify/test', { title, body })
      .then((r) => r.data),

  // repairAndRescrapeAll 触发「全库修复+重刮」：先从媒体路径中的
  // {tmdb-N}/{bangumi-N} 占位符回填缺失/错误的外部 ID，再批量重刮整库。
  // 后端异步执行，立即返回；进度通过 WS "scrape" topic 推送。
  repairAndRescrapeAll: (options?: ScrapeOptions) =>
    api
      .post<{ status: string }>('/admin/media/repair-rescrape', options ?? {})
      .then((r) => r.data),

  // repairAndRescrapeLibrary 触发「单库修复+重刮」：只对指定媒体库回填占位符
  // 外部 ID 并重刮，不影响其它库。后端异步执行，进度通过 WS "scrape" topic 推送。
  repairAndRescrapeLibrary: (libraryID: string, options?: ScrapeOptions) =>
    api
      .post<{ status: string }>(`/admin/libraries/${libraryID}/repair-rescrape`, options ?? {})
      .then((r) => r.data),
}
