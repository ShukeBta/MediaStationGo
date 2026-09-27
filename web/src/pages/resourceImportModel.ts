import type {
  ResourceImportDuplicateConflict,
  ResourceImportTask,
  ResourceSearchCapabilities,
  ResourceSearchFailure,
  ResourceSearchRoot,
} from '../api/resourceImports'

export const RESOURCE_SEARCH_LIMIT = 200
export const RESOURCE_SEARCH_PAGE_SIZE = 20

type ResourceSearchTitleInput = {
  title?: string
  original_name?: string
  nsfw?: boolean
  media_type?: string
  source?: string
}

const adultResourceSources = new Set(['adult', 'javdb', 'javbus', 'fd2ppv'])

function numberedResourceSearch(input: ResourceSearchTitleInput): boolean {
  const mediaType = input.media_type?.trim().toLowerCase() ?? ''
  const source = input.source?.trim().toLowerCase() ?? ''
  return input.nsfw === true || mediaType === 'adult' || adultResourceSources.has(source)
}

export function resourceSearchPrimaryQuery(input: ResourceSearchTitleInput): string {
  const title = input.title?.trim() ?? ''
  const originalName = input.original_name?.trim() ?? ''
  return numberedResourceSearch(input) ? originalName || title : title || originalName
}

export function resourceSearchAlternateQuery(input: ResourceSearchTitleInput): string {
  if (numberedResourceSearch(input)) return ''
  const primary = resourceSearchPrimaryQuery(input)
  const originalName = input.original_name?.trim() ?? ''
  if (!originalName || originalName.localeCompare(primary, undefined, { sensitivity: 'accent' }) === 0) return ''
  return originalName
}

export function resourceSearchAlternateLabel(query: string): string {
  const hasLatin = /[A-Za-z]/.test(query)
  const hasCJK = /[\u3040-\u30ff\u3400-\u9fff\uac00-\ud7af]/.test(query)
  return hasLatin && !hasCJK ? '英文原名补查' : '原名补查'
}

const activeStatuses = new Set(['pending', 'queued', 'running', 'retrying', 'canceling'])
const completedStatuses = new Set(['completed', 'completed_with_warning', 'succeeded', 'success'])
const failedStatuses = new Set(['failed', 'error'])
const cancelledStatuses = new Set(['cancelled', 'canceled'])

export function supportsResourceSource(
  capabilities: ResourceSearchCapabilities | undefined,
  source: 'pansou',
): boolean {
  if (!capabilities) return false
  if (capabilities.sources?.includes(source)) return true
  return capabilities.pansou === true
}

export function supportsResourceLLMRerank(capabilities: ResourceSearchCapabilities | undefined): boolean {
  return capabilities?.llm_rerank === true
}

export function cappedResourceTotal(total: number): number {
  if (!Number.isFinite(total) || total <= 0) return 0
  return Math.min(Math.floor(total), RESOURCE_SEARCH_LIMIT)
}

export function cappedResourceTotalPages(total: number, pageSize: number, reportedPages?: number): number {
  const normalizedPageSize = Number.isFinite(pageSize) && pageSize > 0
    ? Math.floor(pageSize)
    : RESOURCE_SEARCH_PAGE_SIZE
  const pagesFromTotal = Math.max(1, Math.ceil(cappedResourceTotal(total) / normalizedPageSize))
  if (!Number.isFinite(reportedPages) || !reportedPages || reportedPages < 1) return pagesFromTotal
  return Math.max(1, Math.min(Math.floor(reportedPages), pagesFromTotal))
}

export function clampResourcePage(page: number, totalPages: number): number {
  const normalizedPages = Math.max(1, Math.floor(totalPages) || 1)
  if (!Number.isFinite(page)) return 1
  return Math.min(Math.max(1, Math.floor(page)), normalizedPages)
}

export function resolveResourceRootID(roots: ResourceSearchRoot[], currentRootID: string): string {
  const enabledRoots = roots.filter((root) => root.enabled !== false)
  if (enabledRoots.some((root) => root.id === currentRootID)) return currentRootID
  return enabledRoots.length === 1 ? enabledRoots[0].id : ''
}

export function isResourceImportActive(status: string): boolean {
  return activeStatuses.has(status.toLowerCase())
}

export function isResourceImportCompleted(status: string): boolean {
  return completedStatuses.has(status.toLowerCase())
}

export function isResourceImportCompletedWithWarning(status: string): boolean {
  return status.toLowerCase() === 'completed_with_warning'
}

export function isResourceImportFailed(status: string): boolean {
  return failedStatuses.has(status.toLowerCase())
}

export function isResourceImportCancelled(status: string): boolean {
  return cancelledStatuses.has(status.toLowerCase())
}

export function resourceImportProgress(progress?: number): number | null {
  if (!Number.isFinite(progress)) return null
  const value = Number(progress)
  if (value < 0) return 0
  if (value <= 1) return Math.round(value * 100)
  return Math.min(100, Math.round(value))
}

export function resourceImportTitle(task: ResourceImportTask): string {
  return task.candidate_title?.trim() || task.message?.trim() || `资源入库任务 ${task.id}`
}

export function mergeResourceImportTasks(
  current: ResourceImportTask[],
  incoming: ResourceImportTask[],
): ResourceImportTask[] {
  const merged = new Map(current.map((task) => [task.id, task]))
  incoming.forEach((task) => merged.set(task.id, task))
  return Array.from(merged.values()).sort((left, right) => taskTimestamp(right) - taskTimestamp(left))
}

export function resourceImportError(error: unknown, fallback: string): string {
  const payload = (
    error as {
      response?: { data?: { error?: string | { message?: string }; message?: string } }
      message?: string
    }
  )
  const responseError = payload.response?.data?.error
  if (typeof responseError === 'string' && responseError.trim()) return responseError
  if (responseError && typeof responseError === 'object' && responseError.message?.trim()) {
    return responseError.message
  }
  return payload.response?.data?.message || payload.message || fallback
}

export function resourceSearchFailure(error: unknown): ResourceSearchFailure | null {
  const responseError = (
    error as {
      response?: {
        data?: {
          error?: {
            code?: string
            message?: string
            capabilities?: ResourceSearchCapabilities
          }
        }
      }
    }
  ).response?.data?.error
  if (!responseError || responseError.code !== 'search_failed') return null
  return {
    code: responseError.code,
    message: responseError.message?.trim() || '资源搜索暂时不可用',
    capabilities: responseError.capabilities,
  }
}

export function resourceImportDuplicateConflict(error: unknown): ResourceImportDuplicateConflict | null {
  const response = (
    error as {
      response?: {
        status?: number
        data?: {
          error?: string | { code?: string; message?: string; reason?: string; can_force?: boolean; media_id?: string }
          message?: string
          reason?: string
          can_force?: boolean
          media_id?: string
        }
      }
    }
  ).response
  if (response?.status !== 409) return null
  const nestedError = typeof response.data?.error === 'object' ? response.data.error : undefined
  const canForce = response.data?.can_force ?? nestedError?.can_force
  if (typeof canForce !== 'boolean') return null
  return {
    message: response.data?.message
      || nestedError?.message
      || response.data?.reason
      || nestedError?.reason
      || (typeof response.data?.error === 'string' ? response.data.error : '')
      || '该资源已入库或存在重复项',
    can_force: canForce,
    media_id: response.data?.media_id || nestedError?.media_id,
  }
}

function taskTimestamp(task: ResourceImportTask): number {
  return Date.parse(task.updated_at || task.created_at || '') || 0
}
