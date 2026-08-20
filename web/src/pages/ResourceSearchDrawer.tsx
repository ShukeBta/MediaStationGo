import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertTriangle,
  ArrowLeft,
  ChevronLeft,
  ChevronRight,
  Download,
  Globe,
  Languages,
  Link2,
  LoaderCircle,
  RotateCcw,
  Search,
  SlidersHorizontal,
  X,
} from 'lucide-react'

import { confirmAction } from '../components/confirmAction'
import {
  resourceImportsAPI,
  type EpisodeReplenishmentContext,
  type ResourceImportDuplicateConflict,
  type ResourceImportTask,
  type ResourceSearchCapabilities,
  type ResourceSearchCandidate,
  type ResourceSearchFailure,
  type ResourceSearchResponse,
  type ResourceSearchRoot,
} from '../api/resourceImports'
import type { LibraryRoot } from '../types'
import { formatSize } from './libraryPageModel'
import { ResourceImportTaskView } from './ResourceImportTaskView'
import { ManualResourceTaskDialog } from './ManualResourceTaskDialog'
import {
  RESOURCE_SEARCH_PAGE_SIZE,
  cappedResourceTotal,
  cappedResourceTotalPages,
  clampResourcePage,
  isResourceImportActive,
  resolveResourceRootID,
  resourceImportError,
  resourceImportDuplicateConflict,
  resourceSearchAlternateLabel,
  resourceSearchFailure,
  supportsResourceSource,
} from './resourceImportModel'

type ResourceSearchDrawerProps = {
  open: boolean
  embedded?: boolean
  sidecar?: boolean
  showCloseButton?: boolean
  autoSearch?: boolean
  initialQuery?: string
  alternateQuery?: string
  releaseDate?: string
  upgradeMediaID?: string
  upgradeScope?: 'media' | 'work'
  replenishment?: EpisodeReplenishmentContext
  fixedRootID?: string
  canRemoveOldVersion?: boolean
  libraryID: string
  libraryName: string
  libraryRoots: LibraryRoot[]
  tasks: ResourceImportTask[]
  taskID: string
  onTaskIDChange: (taskID: string) => void
  onTaskChanged: (task: ResourceImportTask) => void
  onClose: () => void
}

type SearchSource = '' | 'pansou'

type ResourceViewFilters = {
  resultQuery: string
  source: string
  resolution: string
  subtitle: string
  sortBy: string
}

const emptyResourceFilters = (): ResourceViewFilters => ({
  resultQuery: '',
  source: '',
  resolution: '',
  subtitle: '',
  sortBy: 'relevance',
})

export function ResourceSearchDrawer({
  open,
  embedded = false,
  sidecar = false,
  showCloseButton = true,
  autoSearch = false,
  initialQuery,
  alternateQuery,
  releaseDate,
  upgradeMediaID,
  upgradeScope,
  replenishment,
  fixedRootID,
  canRemoveOldVersion = false,
  libraryID,
  libraryName,
  libraryRoots,
  tasks,
  taskID,
  onTaskIDChange,
  onTaskChanged,
  onClose,
}: ResourceSearchDrawerProps) {
  const [query, setQuery] = useState('')
  const [response, setResponse] = useState<ResourceSearchResponse | null>(null)
  const [source, setSource] = useState<SearchSource>('')
  const [capabilities, setCapabilities] = useState<ResourceSearchCapabilities>()
  const [searchFailure, setSearchFailure] = useState<ResourceSearchFailure | null>(null)
  const [selectedRootID, setSelectedRootID] = useState('')
  const [jumpPage, setJumpPage] = useState('1')
  const [searching, setSearching] = useState(false)
  const [filtering, setFiltering] = useState(false)
  const [keepOldVersion, setKeepOldVersion] = useState(true)
  const [upgradeInputMode, setUpgradeInputMode] = useState<'search' | 'direct'>('search')
  const [directDialogOpen, setDirectDialogOpen] = useState(false)
  const [filters, setFilters] = useState<ResourceViewFilters>(emptyResourceFilters)
  const [appliedFilters, setAppliedFilters] = useState<ResourceViewFilters>(emptyResourceFilters)
  const [importingIndex, setImportingIndex] = useState<number | null>(null)
  const [searchError, setSearchError] = useState('')
  const [taskError, setTaskError] = useState('')
  const [localTask, setLocalTask] = useState<ResourceImportTask | null>(null)
  const [busyAction, setBusyAction] = useState<'cancel' | 'retry' | null>(null)
  const [duplicateConflict, setDuplicateConflict] = useState<{
    candidate: ResourceSearchCandidate
    conflict: ResourceImportDuplicateConflict
  } | null>(null)
  const autoSearchKey = useRef('')

  const roots = useMemo(
    () => searchRoots(response, libraryRoots),
    [libraryRoots, response],
  )
  const selectedTask = taskID
    ? (localTask?.id === taskID ? localTask : tasks.find((task) => task.id === taskID) ?? null)
    : null
  const selectedTaskStatus = selectedTask?.status ?? ''
  const total = cappedResourceTotal(response?.total ?? 0)
  const totalPages = cappedResourceTotalPages(
    response?.total ?? 0,
    response?.page_size ?? RESOURCE_SEARCH_PAGE_SIZE,
    response?.total_pages,
  )
  const currentPage = clampResourcePage(response?.page ?? 1, totalPages)
  const pansouAvailable = capabilities === undefined || supportsResourceSource(capabilities, 'pansou')
  const upgrading = Boolean(upgradeMediaID?.trim())
  const replenishing = Boolean(replenishment?.media_id.trim())
  const hasAppliedFilters = resourceFiltersActive(appliedFilters)
  const embeddedExpanded = Boolean(taskID || response || searching || searchFailure || duplicateConflict)
  const normalizedAlternateQuery = alternateQuery?.trim() ?? ''

  useEffect(() => {
    const nextQuery = initialQuery?.trim()
    if (!open || !nextQuery) return
    setQuery(nextQuery)
    setResponse(null)
    setSearchFailure(null)
    setSearchError('')
    setSource('')
    setJumpPage('1')
    setFilters(emptyResourceFilters())
    setAppliedFilters(emptyResourceFilters())
  }, [initialQuery, open])

  useEffect(() => {
    setKeepOldVersion(true)
    setUpgradeInputMode('search')
    setDirectDialogOpen(false)
  }, [replenishment?.media_id, upgradeMediaID])

  useEffect(() => {
    setSelectedRootID((current) => {
      const lockedRootID = fixedRootID?.trim()
      if (lockedRootID && roots.some((root) => root.enabled !== false && root.id === lockedRootID)) {
        return lockedRootID
      }
      return resolveResourceRootID(roots, current)
    })
  }, [fixedRootID, roots])

  useEffect(() => {
    if (!open || embedded) return
    const previousOverflow = document.body.style.overflow
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    document.body.style.overflow = 'hidden'
    window.addEventListener('keydown', onKeyDown)
    return () => {
      document.body.style.overflow = previousOverflow
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [embedded, onClose, open])

  useEffect(() => {
    if (!taskID) {
      setLocalTask(null)
      setTaskError('')
      return
    }
    const next = tasks.find((task) => task.id === taskID)
    if (next) setLocalTask(next)
  }, [taskID, tasks])

  useEffect(() => {
    if (!open || !taskID || (selectedTaskStatus && !isResourceImportActive(selectedTaskStatus))) return
    let cancelled = false
    const tick = async () => {
      try {
        const task = await resourceImportsAPI.get(taskID)
        if (cancelled) return
        setLocalTask(task)
        onTaskChanged(task)
        setTaskError('')
      } catch (requestError) {
        if (!cancelled) setTaskError(resourceImportError(requestError, '任务进度加载失败'))
      }
    }
    void tick()
    const timer = window.setInterval(tick, 2_000)
    return () => {
      cancelled = true
      window.clearInterval(timer)
    }
  }, [onTaskChanged, open, selectedTaskStatus, taskID])

  const runSearch = useCallback(async (
    page: number,
    nextSource: SearchSource = source,
    nextFilters: ResourceViewFilters = appliedFilters,
    cachedView = false,
    queryOverride?: string,
  ) => {
    const normalizedQuery = (queryOverride ?? query).trim()
    if (!normalizedQuery) {
      setSearchError('请输入要查找的影片或剧集名称')
      return
    }
    const lockedRootID = fixedRootID?.trim() ?? ''
    const effectiveRootID = lockedRootID || resolveResourceRootID(roots, selectedRootID)
    if (lockedRootID && !roots.some((root) => root.enabled !== false && root.id === lockedRootID)) {
      setSearchError(`当前作品的媒体库目录不可用，无法${replenishing ? '补集' : '升级片源'}`)
      return
    }
    if (roots.length > 1 && !effectiveRootID) {
      setSearchError('该媒体库有多个目录，请先明确选择入库目录')
      return
    }
    setSource(nextSource)
    if (cachedView) setFiltering(true)
    else setSearching(true)
    setSearchError('')
    setSearchFailure(null)
    if (!cachedView) setResponse(null)
    setDuplicateConflict(null)
    try {
      const payload = {
        query: normalizedQuery,
        source: nextSource || undefined,
        page,
        page_size: RESOURCE_SEARCH_PAGE_SIZE,
        root_id: effectiveRootID || undefined,
        result_query: nextFilters.resultQuery.trim() || undefined,
        source_filter: nextFilters.source || undefined,
        resolution_filter: nextFilters.resolution || undefined,
        subtitle_filter: nextFilters.subtitle || undefined,
        sort_by: nextFilters.sortBy === 'relevance' ? undefined : nextFilters.sortBy,
      }
      const next = replenishing
        ? await resourceImportsAPI.searchReplenishment(replenishment?.media_id ?? '', payload)
        : await resourceImportsAPI.search(libraryID, payload)
      if (!next.session_id || !Array.isArray(next.results)) {
        throw new Error('资源搜索响应缺少会话或结果列表')
      }
      setResponse(next)
      setAppliedFilters({ ...nextFilters })
      setCapabilities(next.capabilities)
      setJumpPage(String(clampResourcePage(next.page, cappedResourceTotalPages(next.total, next.page_size, next.total_pages))))
    } catch (requestError) {
      const failure = resourceSearchFailure(requestError)
      if (failure) {
        setSearchFailure(failure)
        if (failure.capabilities) setCapabilities(failure.capabilities)
      } else {
        setSearchError(resourceImportError(requestError, '资源搜索失败'))
      }
    } finally {
      if (cachedView) setFiltering(false)
      else setSearching(false)
    }
  }, [appliedFilters, fixedRootID, libraryID, query, replenishment, replenishing, roots, selectedRootID, source])

  useEffect(() => {
    const nextQuery = initialQuery?.trim()
    if (!open || !autoSearch || !nextQuery) return
    const key = `${libraryID}\u0000${replenishment?.media_id ?? ''}\u0000${nextQuery}`
    if (autoSearchKey.current === key) return
    autoSearchKey.current = key
    const cleared = emptyResourceFilters()
    setFilters(cleared)
    setAppliedFilters(cleared)
    void runSearch(1, '', cleared, false, nextQuery)
  }, [autoSearch, initialQuery, libraryID, open, replenishment?.media_id, runSearch])

  if (!open) return null

  const submitSearch = (event: FormEvent) => {
    event.preventDefault()
    const cleared = emptyResourceFilters()
    setFilters(cleared)
    void runSearch(1, source, cleared)
  }

  const searchAlternateQuery = () => {
    if (!normalizedAlternateQuery || searching) return
    const cleared = emptyResourceFilters()
    setQuery(normalizedAlternateQuery)
    setFilters(cleared)
    setAppliedFilters(cleared)
    void runSearch(1, source, cleared, false, normalizedAlternateQuery)
  }

  const submitFilters = (event: FormEvent) => {
    event.preventDefault()
    void runSearch(1, source, filters, true)
  }

  const resetFilters = () => {
    const cleared = emptyResourceFilters()
    setFilters(cleared)
    void runSearch(1, source, cleared, true)
  }

  const searchPansou = () => {
    const cleared = emptyResourceFilters()
    setFilters(cleared)
    setAppliedFilters(cleared)
    void runSearch(1, 'pansou', cleared)
  }

  const selectSource = (nextSource: SearchSource) => {
    if (nextSource === source || searching) return
    const cleared = emptyResourceFilters()
    setFilters(cleared)
    setAppliedFilters(cleared)
    if (query.trim()) {
      void runSearch(1, nextSource, cleared)
      return
    }
    setSource(nextSource)
    setResponse(null)
    setSearchFailure(null)
    setSearchError('')
    setDuplicateConflict(null)
    setJumpPage('1')
  }

  const importCandidate = async (candidate: ResourceSearchCandidate, forceDuplicate = false) => {
    if (!response) return
    const importRootID = fixedRootID?.trim() || selectedRootID
    if (!importRootID) {
      setSearchError('该媒体库有多个目录，请先明确选择入库目录')
      return
    }
    setImportingIndex(candidate.index)
    setSearchError('')
    setDuplicateConflict(null)
    try {
      const task = replenishing
        ? await resourceImportsAPI.createReplenishment(replenishment?.media_id ?? '', response.session_id, candidate.index)
        : await resourceImportsAPI.create(libraryID, {
          search_session_id: response.session_id,
          candidate_index: candidate.index,
          root_id: importRootID,
          force_duplicate: forceDuplicate || undefined,
          upgrade_media_id: upgradeMediaID?.trim() || undefined,
          upgrade_scope: upgrading ? upgradeScope : undefined,
          keep_old_version: upgrading ? keepOldVersion : undefined,
        })
      setLocalTask(task)
      onTaskChanged(task)
      onTaskIDChange(task.id)
    } catch (requestError) {
      const conflict = resourceImportDuplicateConflict(requestError)
      if (conflict) {
        setDuplicateConflict({ candidate, conflict })
      } else {
        setSearchError(resourceImportError(requestError, '提交入库任务失败'))
      }
    } finally {
      setImportingIndex(null)
    }
  }

  const cancelTask = async (task: ResourceImportTask) => {
    const confirmed = await confirmAction({
      title: '取消资源入库任务',
      message: '确定取消当前任务吗？取消后，已经转存到 115 的文件可能仍会保留。',
      confirmText: '取消任务',
      cancelText: '继续执行',
    })
    if (!confirmed) return
    setBusyAction('cancel')
    setTaskError('')
    try {
      const next = await resourceImportsAPI.cancel(task.id)
      setLocalTask(next)
      onTaskChanged(next)
    } catch (requestError) {
      setTaskError(resourceImportError(requestError, '取消任务失败'))
    } finally {
      setBusyAction(null)
    }
  }

  const retryTask = async (task: ResourceImportTask) => {
    setBusyAction('retry')
    setTaskError('')
    try {
      const next = await resourceImportsAPI.retry(task.id)
      setLocalTask(next)
      onTaskChanged(next)
      onTaskIDChange(next.id)
    } catch (requestError) {
      setTaskError(resourceImportError(requestError, '重试任务失败'))
    } finally {
      setBusyAction(null)
    }
  }

  const panel = (
    <aside
      className={sidecar
        ? 'flex max-h-[calc(100dvh-1rem)] w-full flex-col overflow-hidden rounded-3xl border border-white/60 bg-[var(--app-bg)] shadow-2xl sm:max-h-[92vh]'
        : embedded
          ? `flex w-full flex-col overflow-hidden rounded-lg border border-gray-200 bg-[var(--app-bg)] ${embeddedExpanded ? 'sm:h-[65vh] sm:min-h-[26rem] sm:max-h-[42rem]' : ''}`
          : 'absolute inset-y-0 right-0 flex w-full flex-col border-l border-gray-200 bg-[var(--app-bg)] shadow-2xl sm:max-w-3xl lg:max-w-4xl'}
      aria-label={embedded ? `${libraryName} 查找资源` : undefined}
    >
      {(!embedded || sidecar || taskID) && (
        <header className="flex h-16 shrink-0 items-center gap-3 border-b border-gray-200 bg-[var(--app-panel)] px-4 sm:px-6">
          {taskID && (
            <button
              type="button"
              className="rounded-lg p-2 text-sand-500 hover:bg-gray-100 hover:text-ink-600"
              title="返回搜索结果"
              aria-label="返回搜索结果"
              onClick={() => onTaskIDChange('')}
            >
              <ArrowLeft size={19} />
            </button>
          )}
          <div className="min-w-0 flex-1">
            <h2 className="flex min-w-0 items-center gap-2 font-display text-lg font-bold text-ink-600">
              {!taskID && <Search size={19} className="shrink-0 text-brand-600" />}
              <span className="truncate">{taskID ? '资源入库进度' : replenishing ? '补集' : upgrading ? upgradeScope === 'work' ? '整剧升级片源' : '升级片源' : '查找资源'}</span>
            </h2>
            <p className="truncate text-xs text-sand-500">{libraryName}</p>
          </div>
          {showCloseButton && (!embedded || sidecar) && (
            <button
              type="button"
              className="rounded-lg p-2 text-sand-500 hover:bg-gray-100 hover:text-ink-600"
              title="关闭"
              aria-label="关闭查找资源面板"
              onClick={onClose}
            >
              <X size={20} />
            </button>
          )}
        </header>
      )}

        {taskID ? (
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-6">
            {taskError && <InlineError message={taskError} />}
            {selectedTask ? (
              <div>
                <ResourceImportTaskView
                  task={selectedTask}
                  busyAction={busyAction}
                  onCancel={(task) => void cancelTask(task)}
                  onRetry={(task) => void retryTask(task)}
                />
                {isResourceImportActive(selectedTask.status) && (
                  <p className="mt-3 text-xs leading-5 text-sand-500">
                    任务进度通过当前账号鉴权后的接口轮询更新。取消后，已经转存到 115 的文件可能仍会保留。
                  </p>
                )}
              </div>
            ) : (
              <div className="flex items-center gap-2 py-8 text-sm text-sand-500">
                <LoaderCircle size={17} className="animate-spin" />
                正在加载任务详情…
              </div>
            )}
          </div>
        ) : (
          <>
            {(upgrading || replenishing) && (
              <div className="flex shrink-0 items-center gap-2 border-b border-gray-200 bg-[var(--app-panel)] px-4 py-3 sm:px-6">
                <span className="text-xs font-semibold text-ink-100">添加方式</span>
                <div className="inline-flex overflow-hidden rounded-lg border border-gray-200 bg-white">
                  <button
                    type="button"
                    className={`inline-flex h-9 items-center gap-1.5 border-r border-gray-200 px-3 text-xs font-semibold ${upgradeInputMode === 'search' ? 'bg-brand-500 text-white' : 'text-ink-100 hover:bg-gray-50'}`}
                    onClick={() => setUpgradeInputMode('search')}
                  >
                    <Search size={14} />
                    查找资源
                  </button>
                  <button
                    type="button"
                    className={`inline-flex h-9 items-center gap-1.5 px-3 text-xs font-semibold ${upgradeInputMode === 'direct' ? 'bg-brand-500 text-white' : 'text-ink-100 hover:bg-gray-50'}`}
                    onClick={() => setUpgradeInputMode('direct')}
                  >
                    <Link2 size={14} />
                    直接添加
                  </button>
                </div>
              </div>
            )}
            {(upgrading || replenishing) && upgradeInputMode === 'direct' ? (
              <div className="flex min-h-56 flex-1 flex-col items-center justify-center px-6 text-center">
                <Link2 className="mb-3 h-9 w-9 text-brand-500" />
                <h3 className="text-sm font-semibold text-ink-600">{replenishing ? '填写补集链接' : '直接添加片源'}</h3>
                <p className="mt-1 max-w-sm text-xs leading-5 text-sand-500">输入 115 分享链接、磁链或 ED2K 链接，解析后直接创建{replenishing ? '补集' : '升级'}任务。</p>
                <button type="button" className="btn-primary mt-4 h-10 px-4" onClick={() => setDirectDialogOpen(true)}>
                  <Link2 size={16} />
                  填写链接
                </button>
              </div>
            ) : (
            <>
            {replenishing && replenishment && (
              <div className="border-b border-gray-200 bg-sand-50 px-4 py-3 text-xs text-sand-600 sm:px-6">
                <p className="font-semibold text-ink-100">Season {replenishment.season} · {replenishment.title}</p>
                <p className="mt-1">当前已入库 {replenishment.existing_episodes.length} 集；已识别缺集：{formatEpisodeList(replenishment.missing_episodes) || '无'}。</p>
                <p className="mt-1 text-sand-500">缺集以当前最高集 E{replenishment.known_episode_upper_bound || '-'} 为上界；资源入库后仍会按实际文件名校验。</p>
              </div>
            )}
            <form
              className={`shrink-0 border-b border-gray-200 bg-[var(--app-panel)] px-4 ${embedded && !sidecar ? 'py-3' : 'py-4 sm:px-6'}`}
              onSubmit={submitSearch}
            >
              <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
                <SearchSourceControl
                  source={source}
                  searching={searching}
                  pansou={pansouAvailable}
                  onSelect={selectSource}
                />
                {releaseDate?.trim() && (
                  <p className="text-xs text-sand-500">
                    发行日期：<span className="font-medium text-ink-100">{releaseDate.trim()}</span>
                  </p>
                )}
              </div>

              <div className="flex min-w-0 gap-2">
                <label className="min-w-0 flex-1">
                  <span className="sr-only">资源关键词</span>
                  <input
                    autoFocus={!embedded}
                    className="input-field h-11 min-w-0 w-full py-2.5 text-base sm:text-sm"
                    value={query}
                    placeholder="片名、剧名、番号或资源关键词"
                    maxLength={200}
                    onChange={(event) => setQuery(event.target.value)}
                  />
                </label>
                <button
                  type="submit"
                  className="btn-primary h-11 w-11 shrink-0 p-0"
                  title={searching ? '查找中' : '查找资源'}
                  aria-label={searching ? '查找中' : '查找资源'}
                  disabled={searching}
                >
                  {searching ? <LoaderCircle size={18} className="animate-spin" /> : <Search size={18} />}
                </button>
              </div>

              {normalizedAlternateQuery && (
                <button
                  type="button"
                  className="mt-2 inline-flex h-8 max-w-full items-center gap-1.5 rounded-md border border-gray-200 bg-white px-2.5 text-xs font-medium text-ink-100 transition hover:border-primary-300 hover:text-brand-500 disabled:cursor-not-allowed disabled:opacity-50"
                  title={`${resourceSearchAlternateLabel(normalizedAlternateQuery)}：${normalizedAlternateQuery}`}
                  disabled={searching}
                  onClick={searchAlternateQuery}
                >
                  <Languages size={14} className="shrink-0" />
                  <span className="shrink-0">{resourceSearchAlternateLabel(normalizedAlternateQuery)}</span>
                  <span className="truncate text-sand-500">{normalizedAlternateQuery}</span>
                </button>
              )}

              {roots.length > 0 && (
                <div className="mt-3">
                  {fixedRootID || roots.length === 1 ? (
                    <p className="truncate text-xs text-sand-500" title={(roots.find((root) => root.id === selectedRootID) ?? roots[0]).path}>
                      {replenishing ? '补集目录' : upgrading ? '升级目录' : '入库目录'}：{rootLabel(roots.find((root) => root.id === selectedRootID) ?? roots[0])}
                    </p>
                  ) : (
                    <label className="block">
                      <span className="mb-1 block text-xs font-semibold text-ink-100">入库目录</span>
                      <select
                        className="input-field h-10 py-2 text-base sm:text-sm"
                        value={selectedRootID}
                        onChange={(event) => setSelectedRootID(event.target.value)}
                      >
                        <option value="">请选择本次入库目录</option>
                        {roots.map((root) => (
                          <option key={root.id} value={root.id}>{rootLabel(root)}</option>
                        ))}
                      </select>
                    </label>
                  )}
                </div>
              )}

              {upgrading && canRemoveOldVersion && (
                <label className="mt-3 flex items-center gap-2 text-xs font-semibold text-ink-100">
                  <input
                    type="checkbox"
                    checked={keepOldVersion}
                    onChange={(event) => setKeepOldVersion(event.target.checked)}
                  />
                  <span>保留旧版本</span>
                </label>
              )}

              {searchError && <InlineError message={searchError} className="mt-3" />}
            </form>

            {duplicateConflict && (
              <DuplicateConfirmation
                candidate={duplicateConflict.candidate}
                conflict={duplicateConflict.conflict}
                importing={importingIndex !== null}
                upgrading={upgrading || replenishing}
                onForce={() => void importCandidate(duplicateConflict.candidate, true)}
                onCancel={() => setDuplicateConflict(null)}
              />
            )}

            <div className="min-h-0 flex-1 overflow-y-auto px-4 sm:px-6">
              {response && !searching && (
                <ResourceSearchFilters
                  filters={filters}
                  sources={response.facets?.sources ?? []}
                  resolutions={response.facets?.resolutions ?? []}
                  busy={filtering}
                  onChange={setFilters}
                  onSubmit={submitFilters}
                  onReset={resetFilters}
                />
              )}

              {(!embedded || sidecar) && !response && !searching && !searchFailure && (
                <div className="flex min-h-56 flex-col items-center justify-center text-center text-sand-500">
                  <Search className="mb-3 h-9 w-9" />
                  <p className="text-sm">选择搜索模式并输入关键词查找资源</p>
                </div>
              )}

              {searching && (
                <div className="flex min-h-56 flex-col items-center justify-center text-center" aria-live="polite">
                  <LoaderCircle className="mb-3 h-8 w-8 animate-spin text-brand-500" />
                  <p className="text-sm font-medium text-ink-100">
                    {source === 'pansou' ? '正在查找网盘资源…' : '正在查找资源…'}
                  </p>
                </div>
              )}

              {searchFailure && !searching && (
                <ResourceSearchEmptyState
                  failed
                  source={source}
                  pansouAvailable={pansouAvailable}
                  onSearchPansou={searchPansou}
                />
              )}

              {response && !searching && (
                <>
                  {response.results.length === 0 ? (
                    <ResourceSearchEmptyState
                      source={source}
                      pansouAvailable={pansouAvailable}
                      filtered={hasAppliedFilters}
                      onResetFilters={resetFilters}
                      onSearchPansou={searchPansou}
                    />
                  ) : (
                    <>
                      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 py-3 text-xs text-sand-500">
                        <span>
                          {response.unfiltered_total > response.total
                            ? `筛选后 ${total} / ${response.unfiltered_total} 条`
                            : `共 ${total} 条`}
                        </span>
                        <span>第 {currentPage} / {totalPages} 页</span>
                      </div>
                      <div>
                        {response.results.slice(0, RESOURCE_SEARCH_PAGE_SIZE).map((candidate) => (
                          <ResourceCandidateRow
                            key={`${response.session_id}-${candidate.index}`}
                            candidate={candidate}
                            importing={importingIndex === candidate.index}
                            importDisabled={!(fixedRootID?.trim() || selectedRootID) || importingIndex !== null}
                            actionLabel={replenishing ? '补集' : upgrading ? '升级' : '入库'}
                            onImport={() => void importCandidate(candidate)}
                          />
                        ))}
                      </div>
                    </>
                  )}
                </>
              )}
            </div>

            {response && totalPages > 1 && (
              <ResourceSearchPagination
                page={currentPage}
                totalPages={totalPages}
                jumpPage={jumpPage}
                disabled={searching || filtering}
                onJumpPageChange={setJumpPage}
                onPageChange={(page) => void runSearch(clampResourcePage(page, totalPages), source, appliedFilters, true)}
              />
            )}
            </>
            )}
          </>
        )}
        {directDialogOpen && (
          <ManualResourceTaskDialog
            fixedLibraryID={libraryID}
            fixedLibraryName={libraryName}
            fixedRootID={fixedRootID}
            upgradeMediaID={upgradeMediaID}
            upgradeScope={upgradeScope}
            canRemoveOldVersion={canRemoveOldVersion}
            replenishMediaID={replenishment?.media_id}
            replenishment={replenishment}
            onClose={() => setDirectDialogOpen(false)}
            onCreated={(task) => {
              setLocalTask(task)
              onTaskChanged(task)
              onTaskIDChange(task.id)
              setDirectDialogOpen(false)
            }}
          />
        )}
    </aside>
  )

  if (embedded) return panel

  return (
    <div className="fixed inset-0 z-[80]" role="dialog" aria-modal="true" aria-label={`${libraryName} 查找资源`}>
      <button
        type="button"
        className="absolute inset-0 hidden bg-black/35 backdrop-blur-sm sm:block"
        aria-label="关闭查找资源面板"
        onClick={onClose}
      />
      {panel}
    </div>
  )
}

function formatEpisodeList(values: number[]): string {
  const episodes = [...new Set(values.filter((value) => Number.isInteger(value) && value > 0))].sort((left, right) => left - right)
  const ranges: string[] = []
  for (let index = 0; index < episodes.length;) {
    const start = episodes[index]
    let end = start
    while (episodes[index + 1] === end + 1) {
      index += 1
      end = episodes[index]
    }
    ranges.push(start === end ? `E${start}` : `E${start}–E${end}`)
    index += 1
  }
  return ranges.join('、')
}

function SearchSourceControl({
  source,
  searching,
  pansou,
  onSelect,
}: {
  source: SearchSource
  searching: boolean
  pansou: boolean
  onSelect: (source: SearchSource) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs font-semibold text-ink-100">搜索模式</span>
      <div className="inline-flex overflow-hidden rounded-lg border border-gray-200 bg-white">
        <SourceButton
          active={source === ''}
          disabled={searching}
          loading={searching && source === ''}
          onClick={() => onSelect('')}
        >
          普通
        </SourceButton>
        {pansou && (
          <SourceButton
            active={source === 'pansou'}
            disabled={searching}
            loading={searching && source === 'pansou'}
            onClick={() => onSelect('pansou')}
          >
            网盘
          </SourceButton>
        )}
      </div>
    </div>
  )
}

function ResourceSearchFilters({
  filters,
  sources,
  resolutions,
  busy,
  onChange,
  onSubmit,
  onReset,
}: {
  filters: ResourceViewFilters
  sources: string[]
  resolutions: string[]
  busy: boolean
  onChange: (filters: ResourceViewFilters) => void
  onSubmit: (event: FormEvent) => void
  onReset: () => void
}) {
  return (
    <form className="border-b border-gray-200 py-3" onSubmit={onSubmit}>
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="inline-flex items-center gap-1.5 text-xs font-semibold text-ink-100">
          <SlidersHorizontal size={14} />
          筛选与排序
        </span>
        <button
          type="button"
          className="rounded-md p-1.5 text-sand-500 hover:bg-gray-100 hover:text-ink-600"
          title="重置筛选"
          aria-label="重置筛选"
          disabled={busy}
          onClick={onReset}
        >
          <RotateCcw size={15} />
        </button>
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        <input
          className="input-field col-span-2 h-10 py-2 text-base sm:text-sm"
          value={filters.resultQuery}
          placeholder="筛选当前结果"
          maxLength={200}
          disabled={busy}
          onChange={(event) => onChange({ ...filters, resultQuery: event.target.value })}
        />
        <select
          className="input-field h-10 py-2 text-base sm:text-sm"
          value={filters.source}
          disabled={busy}
          aria-label="来源筛选"
          onChange={(event) => onChange({ ...filters, source: event.target.value })}
        >
          <option value="">全部来源</option>
          {sources.map((value) => <option key={value} value={value}>{value}</option>)}
        </select>
        <select
          className="input-field h-10 py-2 text-base sm:text-sm"
          value={filters.resolution}
          disabled={busy}
          aria-label="清晰度筛选"
          onChange={(event) => onChange({ ...filters, resolution: event.target.value })}
        >
          <option value="">全部清晰度</option>
          {resolutions.map((value) => <option key={value} value={value}>{resourceResolutionLabel(value)}</option>)}
        </select>
        <select
          className="input-field h-10 py-2 text-base sm:text-sm"
          value={filters.subtitle}
          disabled={busy}
          aria-label="字幕筛选"
          onChange={(event) => onChange({ ...filters, subtitle: event.target.value })}
        >
          <option value="">全部字幕</option>
          <option value="chinese">中文字幕</option>
          <option value="with_subtitle">有字幕标注</option>
        </select>
        <select
          className="input-field h-10 py-2 text-base sm:text-sm"
          value={filters.sortBy}
          disabled={busy}
          aria-label="结果排序"
          onChange={(event) => onChange({ ...filters, sortBy: event.target.value })}
        >
          <option value="relevance">综合排序</option>
          <option value="resolution_desc">清晰度优先</option>
          <option value="seeders_desc">做种数优先</option>
          <option value="size_desc">体积从大到小</option>
          <option value="size_asc">体积从小到大</option>
        </select>
      </div>
      <button type="submit" className="btn-outline mt-2 h-9 px-3" disabled={busy}>
        {busy ? <LoaderCircle size={15} className="animate-spin" /> : <SlidersHorizontal size={15} />}
        应用
      </button>
    </form>
  )
}

function SourceButton({
  active,
  disabled,
  loading,
  children,
  onClick,
}: {
  active: boolean
  disabled: boolean
  loading: boolean
  children: React.ReactNode
  onClick: () => void
}) {
  return (
    <button
      type="button"
      className={`inline-flex h-9 min-w-20 items-center justify-center gap-1.5 border-r border-gray-200 px-3 text-xs font-semibold transition-colors last:border-r-0 ${active ? 'bg-brand-500 text-white' : 'text-ink-100 hover:bg-gray-50'}`}
      disabled={disabled}
      onClick={onClick}
    >
      {loading && <LoaderCircle size={14} className="animate-spin" />}
      {children}
    </button>
  )
}

function ResourceSearchEmptyState({
  failed = false,
  source,
  pansouAvailable,
  filtered = false,
  onResetFilters,
  onSearchPansou,
}: {
  failed?: boolean
  source: SearchSource
  pansouAvailable: boolean
  filtered?: boolean
  onResetFilters?: () => void
  onSearchPansou: () => void
}) {
  const canSearchPansou = pansouAvailable && source !== 'pansou'
  return (
    <div className="flex min-h-56 flex-col items-center justify-center px-4 text-center" aria-live="polite">
      <span className="mb-3 inline-flex h-11 w-11 items-center justify-center rounded-full bg-gray-100 text-sand-500">
        <Globe size={21} />
      </span>
      <h3 className="text-sm font-semibold text-ink-600">
        {filtered ? '当前筛选没有结果' : failed ? '当前搜索暂时未返回结果' : '没有找到相关资源'}
      </h3>
      <p className="mt-1 max-w-sm text-xs leading-5 text-sand-500">
        {filtered ? '可以重置筛选条件后继续查看。' : canSearchPansou ? '可以改用网盘搜索继续查找。' : '可以调整关键词后重新查找。'}
      </p>
      {filtered && onResetFilters ? (
        <button type="button" className="btn-outline mt-4 h-10 px-4" onClick={onResetFilters}>
          <RotateCcw size={16} />
          重置筛选
        </button>
      ) : canSearchPansou && (
        <button type="button" className="btn-outline mt-4 h-10 px-4" onClick={onSearchPansou}>
          <Globe size={16} />
          网盘查找
        </button>
      )}
    </div>
  )
}

function ResourceCandidateRow({
  candidate,
  importing,
  importDisabled,
  actionLabel,
  onImport,
}: {
  candidate: ResourceSearchCandidate
  importing: boolean
  importDisabled: boolean
  actionLabel: string
  onImport: () => void
}) {
  const metadata = [
    candidate.size_text || (candidate.size_bytes ? formatSize(candidate.size_bytes) : ''),
    candidate.source ? `来源 ${resourceSourceLabel(candidate.source)}` : '',
    candidate.seeders !== undefined ? `做种 ${candidate.seeders}` : '',
    candidate.resolution,
    candidate.subtitle,
    candidate.resource_type,
  ].filter(Boolean)

  return (
    <article className="border-b border-gray-200 py-4 last:border-b-0">
      <div className="flex min-w-0 items-start gap-3">
        <div className="min-w-0 flex-1">
          <h3 className="break-words text-sm font-semibold leading-6 text-ink-600">{candidate.title}</h3>
          {metadata.length > 0 && (
            <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-sand-500">
              {metadata.map((item) => <span key={String(item)}>{item}</span>)}
            </div>
          )}
          {candidate.summary && <p className="mt-2 line-clamp-3 break-words text-xs leading-5 text-ink-50">{candidate.summary}</p>}
          {candidate.compatibility_warning && (
            <p className="mt-2 inline-flex items-start gap-1.5 rounded-md border border-amber-200 bg-amber-50 px-2 py-1 text-xs font-medium leading-5 text-amber-800">
              <AlertTriangle size={14} className="mt-0.5 shrink-0" aria-hidden="true" />
              <span>{candidate.compatibility_warning}</span>
            </p>
          )}
        </div>
        <button
          type="button"
          className="btn-outline h-10 shrink-0 px-3"
          title={importDisabled && !importing ? '请先选择入库目录' : '提交入库任务'}
          disabled={importDisabled}
          onClick={onImport}
        >
          {importing ? <LoaderCircle size={17} className="animate-spin" /> : <Download size={17} />}
          <span className="hidden sm:inline">{actionLabel}</span>
        </button>
      </div>
    </article>
  )
}

function resourceSourceLabel(source: string): string {
  return source.trim().toLowerCase() === 'pansou' ? '网盘' : source
}

function resourceResolutionLabel(value: string): string {
  if (value === '2160p') return '4K / 2160p'
  if (value === '1080p') return '1080p'
  if (value === '720p') return '720p'
  return '其他'
}

function resourceFiltersActive(filters: ResourceViewFilters): boolean {
  return Boolean(
    filters.resultQuery.trim()
    || filters.source
    || filters.resolution
    || filters.subtitle
    || (filters.sortBy && filters.sortBy !== 'relevance'),
  )
}

function ResourceSearchPagination({
  page,
  totalPages,
  jumpPage,
  disabled,
  onJumpPageChange,
  onPageChange,
}: {
  page: number
  totalPages: number
  jumpPage: string
  disabled: boolean
  onJumpPageChange: (page: string) => void
  onPageChange: (page: number) => void
}) {
  const submitJump = (event: FormEvent) => {
    event.preventDefault()
    const parsed = Number.parseInt(jumpPage, 10)
    if (!Number.isFinite(parsed)) {
      onJumpPageChange(String(page))
      return
    }
    onPageChange(clampResourcePage(parsed, totalPages))
  }
  return (
    <footer className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-t border-gray-200 bg-[var(--app-panel)] px-4 py-3 sm:px-6">
      <div className="flex items-center gap-2">
        <button
          type="button"
          className="btn-outline h-9 px-3"
          title="上一页"
          disabled={disabled || page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          <ChevronLeft size={17} />
          <span className="hidden sm:inline">上一页</span>
        </button>
        <button
          type="button"
          className="btn-outline h-9 px-3"
          title="下一页"
          disabled={disabled || page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          <span className="hidden sm:inline">下一页</span>
          <ChevronRight size={17} />
        </button>
      </div>
      <form className="flex items-center gap-2" onSubmit={submitJump}>
        <label className="text-xs text-sand-500" htmlFor="resource-search-jump">跳至</label>
        <input
          id="resource-search-jump"
          type="number"
          min={1}
          max={totalPages}
          className="input-field h-9 w-20 px-2 py-1 text-center text-base sm:text-sm"
          value={jumpPage}
          disabled={disabled}
          onChange={(event) => onJumpPageChange(event.target.value)}
        />
        <button type="submit" className="btn-outline h-9 px-3" disabled={disabled}>跳页</button>
      </form>
    </footer>
  )
}

function InlineError({ message, className = '' }: { message: string; className?: string }) {
  return <p className={`break-words text-sm text-red-500 ${className}`}>{message}</p>
}

function DuplicateConfirmation({
  candidate,
  conflict,
  importing,
  upgrading,
  onForce,
  onCancel,
}: {
  candidate: ResourceSearchCandidate
  conflict: ResourceImportDuplicateConflict
  importing: boolean
  upgrading: boolean
  onForce: () => void
  onCancel: () => void
}) {
  return (
    <section className="shrink-0 border-b border-amber-300 bg-amber-50 px-4 py-4 sm:px-6">
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold text-amber-900">{upgrading ? '无法将该资源作为升级版本入库' : '检测到重复资源'}</h3>
          <p className="mt-1 break-words text-sm text-amber-800">{conflict.message}</p>
          <p className="mt-1 truncate text-xs text-amber-700" title={candidate.title}>{candidate.title}</p>
          <div className="mt-3 flex flex-wrap gap-2">
            {conflict.can_force && !upgrading ? (
              <>
                <button type="button" className="btn-primary px-4 py-2" disabled={importing} onClick={onForce}>
                  {importing && <LoaderCircle size={16} className="animate-spin" />}
                  仍然入库
                </button>
                <button type="button" className="btn-outline px-4 py-2" disabled={importing} onClick={onCancel}>
                  取消
                </button>
              </>
            ) : (
              <>
                {conflict.media_id && (
                  <Link className="btn-primary px-4 py-2" to={`/media/${conflict.media_id}`}>
                    查看已入库影片
                  </Link>
                )}
                {!conflict.media_id && (
                  <button type="button" className="btn-outline px-4 py-2" onClick={onCancel}>关闭</button>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </section>
  )
}

function searchRoots(response: ResourceSearchResponse | null, libraryRoots: LibraryRoot[]): ResourceSearchRoot[] {
  if (response?.roots) return response.roots.filter((root) => root.enabled !== false)
  return libraryRoots
    .filter((root) => root.enabled)
    .map((root) => ({ id: root.id, name: root.name, path: root.path, enabled: root.enabled }))
}

function rootLabel(root: ResourceSearchRoot): string {
  if (root.name && root.path) return `${root.name} · ${root.path}`
  return root.name || root.path || root.id
}
