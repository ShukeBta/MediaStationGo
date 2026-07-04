import { api, BATCH_REQUEST_TIMEOUT } from './client'

export type GenerateSTRMInput = {
  library_id: string
  output_dir?: string
  base_url?: string
  enabled?: boolean
  overwrite?: boolean
  include_local?: boolean
  preserve_tree?: boolean
  refresh_library?: boolean
  scrape_after?: boolean
}

export type GenerateSTRMResult = {
  library_id: string
  output_dir: string
  generated: number
  updated: number
  skipped: number
  cleaned: number
  total?: number
  remaining?: number
  batch_limited?: boolean
  ignored?: number
  previewed?: number
  errors?: string[]
  ignored_items?: string[]
  refresh?: STRMRefreshResult
  items?: Array<{
    media_id: string
    title: string
    file_path: string
    url?: string
    action: string
    reason?: string
  }>
}

export type GenerateSTRMTreeInput = {
  provider: string
  tree_text?: string
  paths?: string[]
  source_root?: string
  output_prefix?: string
  output_dir: string
  base_url?: string
  overwrite?: boolean
  cleanup?: boolean
  dry_run?: boolean
  batch_limit?: number
  recognize_rename?: boolean
  transfer_subtitles?: boolean
  missing_only?: boolean
  refresh_library?: boolean
  scrape_after?: boolean
}

export type RepairSTRMInput = {
  output_dir: string
  base_url?: string
  dry_run?: boolean
  refresh_library?: boolean
  scrape_after?: boolean
}

export type RepairSTRMResult = {
  output_dir: string
  repaired: number
  previewed?: number
  skipped: number
  errors?: string[]
  refresh?: STRMRefreshResult
  items?: Array<{
    file_path: string
    before?: string
    after?: string
    action: string
    reason?: string
  }>
}

export type STRMRefreshResult = {
  requested: boolean
  queued: boolean
  reason?: string
  scrape_requested?: boolean
  scrape_queued?: boolean
  scrape_reason?: string
  targets?: Array<{
    library_id: string
    root_id?: string
    name: string
    path: string
  }>
}

export type STRMOutputPreset = {
  label: string
  path: string
  kind: 'default' | 'library' | string
}

export const strmAPI = {
  set: (mediaID: string, url: string) =>
    api.put(`/media/${mediaID}/strm`, { url }).then((r) => r.data),
  clear: (mediaID: string) => api.delete(`/media/${mediaID}/strm`).then((r) => r.data),
  outputPresets: () =>
    api.get<{ items: STRMOutputPreset[] }>('/strm/output-presets').then((r) => r.data.items),
  importURL: (libraryID: string, title: string, url: string) =>
    api.post('/strm/import', { library_id: libraryID, title, url }).then((r) => r.data),
  generate: (input: GenerateSTRMInput) =>
    api
      .post<GenerateSTRMResult>('/strm/generate', input, { timeout: BATCH_REQUEST_TIMEOUT })
      .then((r) => r.data),
  generateFromTree: (input: GenerateSTRMTreeInput) =>
    api
      .post<GenerateSTRMResult>('/strm/generate-from-tree', input, { timeout: BATCH_REQUEST_TIMEOUT })
      .then((r) => r.data),
  repair: (input: RepairSTRMInput) =>
    api
      .post<RepairSTRMResult>('/strm/repair', input, { timeout: BATCH_REQUEST_TIMEOUT })
      .then((r) => r.data),
}
