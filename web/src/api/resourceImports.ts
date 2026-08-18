import { api, LONG_REQUEST_TIMEOUT } from './client'

export interface ResourceSearchRoot {
  id: string
  name?: string
  path?: string
  enabled?: boolean
}

export interface ResourceSearchCapabilities {
  sources?: string[]
  pansou?: boolean
  llm_rerank?: boolean
}

export interface ResourceSearchCandidate {
  index: number
  title: string
  size_bytes?: number
  size_text?: string
  source?: string
  seeders?: number
  resolution?: string
  subtitle?: string
  resource_type?: string
  summary?: string
  compatibility_warning?: string
}

export interface ResourceSearchResponse {
  session_id: string
  query: string
  page: number
  page_size: number
  total: number
  unfiltered_total: number
  total_pages: number
  roots?: ResourceSearchRoot[]
  capabilities?: ResourceSearchCapabilities
  facets?: {
    sources?: string[]
    resolutions?: string[]
  }
  results: ResourceSearchCandidate[]
}

export interface ResourceSearchFailure {
  code: string
  message: string
  capabilities?: ResourceSearchCapabilities
}

export interface ResourceSearchRequest {
  query: string
  source?: string
  page?: number
  page_size?: number
  root_id?: string
  result_query?: string
  source_filter?: string
  resolution_filter?: string
  subtitle_filter?: string
  sort_by?: string
}

export interface ResourceImportTask {
  id: string
  library_id: string
  library_name?: string
  user_id?: string
  creator_username?: string
  search_session_id?: string
  candidate_index?: number
  candidate_title?: string
  source?: string
  root_id?: string
  root_name?: string
  status: string
  stage?: string
  progress?: number
  message?: string
  error?: string
  media_id?: string
  subscription_id?: string
  subscription_follow?: boolean
  manual_replenish?: boolean
  work_key?: string
  season_number?: number
  title_class?: string
  target_openlist_path?: string
  outcome?: string
  attempt?: number
  upgrade_media_id?: string
  upgrade_scope?: 'media' | 'work'
  keep_old_version?: boolean
  created_at?: string
  updated_at?: string
  finished_at?: string
}

export interface ResourceImportPage {
  items: ResourceImportTask[]
  total: number
  page: number
  page_size: number
}

export interface ResourceImportDuplicateConflict {
  message: string
  can_force: boolean
  media_id?: string
}

type RawResourceImportTask = Partial<ResourceImportTask> & {
  id: string
  status: string
  username?: string
  library_root_id?: string
  candidate?: {
    index: number
    title: string
    source?: string
    size_bytes?: number
  }
  current_stage?: string
  progress_percent?: number
}
type ResourceImportListResponse = RawResourceImportTask[] | {
  items: RawResourceImportTask[]
  total?: number
  page?: number
  page_size?: number
}
type ResourceImportTaskResponse = RawResourceImportTask | { task: RawResourceImportTask }

export const resourceImportsAPI = {
  search: (libraryID: string, payload: ResourceSearchRequest) =>
    api
      .post<ResourceSearchResponse>(`/libraries/${libraryID}/resource-searches`, payload, {
        timeout: LONG_REQUEST_TIMEOUT,
      })
      .then((response) => response.data),

  previewManual: (libraryID: string, title: string, input: string, rootID?: string) =>
    api
      .post<ResourceSearchResponse>(`/libraries/${libraryID}/manual-resource-previews`, {
        title,
        input,
        ...(rootID?.trim() ? { root_id: rootID.trim() } : {}),
      })
      .then((response) => response.data),

  create: (
    libraryID: string,
    payload: {
      search_session_id: string
      candidate_index: number
      root_id: string
      force_duplicate?: boolean
      upgrade_media_id?: string
      upgrade_scope?: 'media' | 'work'
      keep_old_version?: boolean
    },
  ) =>
    api
      .post<ResourceImportTaskResponse>(`/libraries/${libraryID}/resource-imports`, payload)
      .then((response) => unwrapTask(response.data)),

  replenishEpisodes: (mediaID: string, input: string) =>
    api
      .post<ResourceImportTaskResponse>(`/media/${encodeURIComponent(mediaID)}/episode-replenishments`, { input })
      .then((response) => unwrapTask(response.data)),

  listLibrary: (libraryID: string, status?: string) =>
    api
      .get<ResourceImportListResponse>(`/libraries/${libraryID}/resource-imports`, {
        params: status ? { status } : undefined,
      })
      .then((response) => unwrapTaskList(response.data)),

  listAll: (status?: string) =>
    api
      .get<ResourceImportListResponse>('/resource-imports', {
        params: status ? { status } : undefined,
      })
      .then((response) => unwrapTaskList(response.data)),

  listAllPage: (page = 1, pageSize = 12, status?: string) =>
    api
      .get<ResourceImportListResponse>('/resource-imports', {
        params: {
          page,
          page_size: pageSize,
          ...(status ? { status } : {}),
        },
      })
      .then((response) => unwrapTaskPage(response.data, page, pageSize)),

  get: (taskID: string) =>
    api
      .get<ResourceImportTaskResponse>(`/resource-imports/${taskID}`)
      .then((response) => unwrapTask(response.data)),

  cancel: (taskID: string) =>
    api
      .post<ResourceImportTaskResponse>(`/resource-imports/${taskID}/cancel`)
      .then((response) => unwrapTask(response.data)),

  retry: (taskID: string) =>
    api
      .post<ResourceImportTaskResponse>(`/resource-imports/${taskID}/retry`)
      .then((response) => unwrapTask(response.data)),

  removeFailed: (taskID: string) =>
    api.delete(`/resource-imports/${taskID}`).then(() => undefined),
}

function unwrapTaskList(payload: ResourceImportListResponse): ResourceImportTask[] {
  if (Array.isArray(payload)) return payload.map(normalizeTask)
  if (payload && Array.isArray(payload.items)) return payload.items.map(normalizeTask)
  throw new Error('资源入库任务列表响应格式无效')
}

function unwrapTaskPage(payload: ResourceImportListResponse, page: number, pageSize: number): ResourceImportPage {
  if (Array.isArray(payload)) {
    const items = payload.map(normalizeTask)
    return { items, total: items.length, page, page_size: pageSize }
  }
  if (payload && Array.isArray(payload.items)) {
    const items = payload.items.map(normalizeTask)
    return {
      items,
      total: payload.total ?? items.length,
      page: payload.page ?? page,
      page_size: payload.page_size ?? pageSize,
    }
  }
  throw new Error('资源入库任务分页响应格式无效')
}

function unwrapTask(payload: ResourceImportTaskResponse): ResourceImportTask {
  if (isTask(payload)) return normalizeTask(payload)
  if (payload && isTask(payload.task)) return normalizeTask(payload.task)
  throw new Error('资源入库任务响应格式无效')
}

function isTask(payload: unknown): payload is RawResourceImportTask {
  if (!payload || typeof payload !== 'object') return false
  const task = payload as Partial<ResourceImportTask>
  return typeof task.id === 'string' && typeof task.status === 'string'
}

function normalizeTask(task: RawResourceImportTask): ResourceImportTask {
  return {
    ...task,
    library_id: task.library_id || '',
    creator_username: task.creator_username || task.username,
    root_id: task.root_id || task.library_root_id,
    candidate_index: task.candidate_index ?? task.candidate?.index,
    candidate_title: task.candidate_title || task.candidate?.title,
    source: task.source || task.candidate?.source,
    stage: task.stage || task.current_stage,
    progress: task.progress ?? task.progress_percent,
  }
}
