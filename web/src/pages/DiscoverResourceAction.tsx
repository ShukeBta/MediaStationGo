import { useCallback, useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { Link } from 'react-router-dom'
import { ArrowRight, BellPlus, Check, LoaderCircle, Search } from 'lucide-react'
import toast from 'react-hot-toast'

import type { DiscoverItem } from '../api/discover'
import { libraryAPI } from '../api/library'
import type { ResourceImportTask } from '../api/resourceImports'
import { buildResourceImportFeedURL, buildSubscriptionAliases, subscriptionsAPI } from '../api/subscriptions'
import { useAuthStore } from '../stores/auth'
import type { Library } from '../types'
import { discoverResourceSearchAlternateKeyword, discoverResourceSearchKeyword } from './discoverDetailModalModel'
import { ResourceSearchDrawer } from './ResourceSearchDrawer'
import { mergeResourceImportTasks } from './resourceImportModel'

export function DiscoverResourceAction({
  item,
  sidecarRoot,
  subscriptionActionRoot,
  onSidecarOpenChange,
}: {
  item: DiscoverItem
  sidecarRoot?: HTMLElement | null
  subscriptionActionRoot?: HTMLElement | null
  onSidecarOpenChange?: (open: boolean) => void
}) {
  const [libraries, setLibraries] = useState<Library[]>([])
  const [selectedLibraryID, setSelectedLibraryID] = useState('')
  const [tasks, setTasks] = useState<ResourceImportTask[]>([])
  const [taskID, setTaskID] = useState('')
  const [subscriptionRootID, setSubscriptionRootID] = useState('')
  const [subscribing, setSubscribing] = useState(false)
  const [subscribed, setSubscribed] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [sidecarOpen, setSidecarOpen] = useState(false)
  const [wideLayout, setWideLayout] = useState(() => (
    typeof window !== 'undefined' && window.matchMedia('(min-width: 1280px)').matches
  ))
  const isAdmin = useAuthStore((state) => state.user?.role === 'admin')

  const setDesktopSidecarOpen = useCallback((open: boolean) => {
    setSidecarOpen(open)
    onSidecarOpenChange?.(open)
  }, [onSidecarOpenChange])

  useEffect(() => {
    setDesktopSidecarOpen(false)
  }, [item.original_name, item.provider_id, item.source, setDesktopSidecarOpen])

  useEffect(() => () => onSidecarOpenChange?.(false), [onSidecarOpenChange])

  useEffect(() => {
    const media = window.matchMedia('(min-width: 1280px)')
    const update = () => setWideLayout(media.matches)
    update()
    media.addEventListener('change', update)
    return () => media.removeEventListener('change', update)
  }, [])

  useEffect(() => {
    if (!wideLayout && sidecarOpen) setDesktopSidecarOpen(false)
  }, [setDesktopSidecarOpen, sidecarOpen, wideLayout])

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    libraryAPI
      .list()
      .then((items) => {
        if (cancelled) return
        setLibraries(items.filter((library) => library.enabled))
      })
      .catch(() => {
        if (!cancelled) setError('媒体库加载失败')
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [])

  const preferredLibraryID = useMemo(
    () => selectPreferredLibraryID(libraries, item.media_type),
    [item.media_type, libraries],
  )
  const effectiveLibraryID = selectedLibraryID || preferredLibraryID
  const selectedLibrary = libraries.find((library) => library.id === effectiveLibraryID)
  const enabledRoots = (selectedLibrary?.roots ?? []).filter((root) => root.enabled)
  const effectiveSubscriptionRootID = subscriptionRootID || (enabledRoots.length === 1 ? enabledRoots[0].id : '')
  const canSubscribe = isAdmin && ['tv', 'anime', 'variety'].includes((item.media_type || '').toLowerCase())
  const subscriptionButton = canSubscribe && selectedLibrary ? (
    <button
      type="button"
      className="btn-outline h-9 gap-1.5 px-3 text-xs"
      disabled={!effectiveSubscriptionRootID || subscribing || subscribed}
      onClick={() => void createSubscription()}
    >
      {subscribing ? <LoaderCircle size={14} className="animate-spin" /> : subscribed ? <Check size={14} /> : <BellPlus size={14} />}
      {subscribed ? '已订阅' : '订阅追更'}
    </button>
  ) : null
  const acceptTask = useCallback((task: ResourceImportTask) => {
    setTasks((current) => mergeResourceImportTasks(current, [task]))
  }, [])

  const selectLibrary = (libraryID: string) => {
    setSelectedLibraryID(libraryID)
    const roots = (libraries.find((library) => library.id === libraryID)?.roots ?? []).filter((root) => root.enabled)
    setSubscriptionRootID(roots.length === 1 ? roots[0].id : '')
    setSubscribed(false)
    setTasks([])
    setTaskID('')
  }

  const createSubscription = async () => {
    if (!selectedLibrary || !effectiveSubscriptionRootID || subscribing) return
    setSubscribing(true)
    try {
      const aliases = buildSubscriptionAliases(item)
      await subscriptionsAPI.create({
        name: item.title,
        feed_url: buildResourceImportFeedURL(aliases),
        delivery_mode: 'resource_import',
        library_id: selectedLibrary.id,
        library_root_id: effectiveSubscriptionRootID,
        resource_source: 'default',
        max_imports_per_run: 2,
        season_number: 1,
        filter: item.subscribe_keyword || item.title,
        original_name: item.original_name,
        year: item.year,
        media_type: item.media_type || selectedLibrary.type,
        poster_url: item.poster_url,
        backdrop_url: item.backdrop_url,
        overview: item.overview,
        total_episodes: item.total_episodes,
        resolution: 'best',
        enabled: true,
      })
      setSubscribed(true)
      toast.success('已创建网盘追更订阅')
    } catch (requestError) {
      const message = (requestError as { response?: { data?: { error?: string } } })?.response?.data?.error
      toast.error(message || '创建订阅失败')
    } finally {
      setSubscribing(false)
    }
  }

  const resourceSearch = selectedLibrary && (
    <ResourceSearchDrawer
      key={selectedLibrary.id}
      embedded
      sidecar
      showCloseButton={false}
      autoSearch
      open
      initialQuery={discoverResourceSearchKeyword(item)}
      alternateQuery={discoverResourceSearchAlternateKeyword(item)}
      releaseDate={item.release_date}
      libraryID={selectedLibrary.id}
      libraryName={selectedLibrary.name}
      libraryRoots={selectedLibrary.roots ?? []}
      tasks={tasks}
      taskID={taskID}
      onTaskIDChange={setTaskID}
      onTaskChanged={acceptTask}
      onClose={() => setDesktopSidecarOpen(false)}
    />
  )

  return (
    <>
      <section className="space-y-3">
      {item.in_library && item.media_id && (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-emerald-200 bg-emerald-50 p-3">
          <div>
            <p className="text-sm font-semibold text-emerald-800">该作品已在媒体库中</p>
            <p className="text-xs text-emerald-700">可直接打开库内详情；下方仍可继续查找更高质量片源。</p>
          </div>
          <Link to={`/media/${item.media_id}`} className="btn-primary shrink-0 px-3 py-2 text-xs">
            查看库内作品
            <ArrowRight size={14} />
          </Link>
        </div>
      )}
      {loading ? (
        <div className="flex h-11 items-center gap-2 text-sm text-sand-500">
          <LoaderCircle size={16} className="animate-spin" />
          正在加载媒体库
        </div>
      ) : error ? (
        <p className="text-sm text-red-500">{error}</p>
      ) : libraries.length === 0 ? (
        <p className="text-sm text-sand-500">当前没有可用媒体库</p>
      ) : (
        <>
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
            <label className="flex min-w-0 flex-1 items-center gap-2 sm:max-w-xl">
              <span className="shrink-0 text-xs font-semibold text-sand-500">目标媒体库</span>
              <select
                className="input-base h-10 min-w-0 flex-1 py-2"
                value={effectiveLibraryID}
                onChange={(event) => selectLibrary(event.target.value)}
              >
                {libraries.map((library) => (
                  <option key={library.id} value={library.id}>
                    {library.name}
                  </option>
                ))}
              </select>
            </label>
            {selectedLibrary && wideLayout && (
              <button
                type="button"
                className="btn-primary h-10 shrink-0 gap-2 px-4"
                onClick={() => setDesktopSidecarOpen(true)}
              >
                <Search size={16} />
                {sidecarOpen ? '资源面板已打开' : '查找资源'}
              </button>
            )}
          </div>
          {canSubscribe && selectedLibrary && (enabledRoots.length > 1 || !subscriptionActionRoot) && (
            <div className="flex flex-wrap items-end gap-2">
              {enabledRoots.length > 1 && (
                <label className="min-w-56 flex-1 text-xs text-sand-500">
                  追更入库目录
                  <select
                    className="input-base mt-1"
                    value={effectiveSubscriptionRootID}
                    onChange={(event) => setSubscriptionRootID(event.target.value)}
                  >
                    <option value="">选择目录</option>
                    {enabledRoots.map((root) => <option key={root.id} value={root.id}>{root.name || root.path}</option>)}
                  </select>
                </label>
              )}
              {!subscriptionActionRoot && subscriptionButton}
            </div>
          )}
          {selectedLibrary && (
            !wideLayout ? (
              <div>
                <ResourceSearchDrawer
                  key={selectedLibrary.id}
                  embedded
                  open
                  initialQuery={discoverResourceSearchKeyword(item)}
                  alternateQuery={discoverResourceSearchAlternateKeyword(item)}
                  releaseDate={item.release_date}
                  libraryID={selectedLibrary.id}
                  libraryName={selectedLibrary.name}
                  libraryRoots={selectedLibrary.roots ?? []}
                  tasks={tasks}
                  taskID={taskID}
                  onTaskIDChange={setTaskID}
                  onTaskChanged={acceptTask}
                  onClose={() => undefined}
                />
              </div>
            ) : null
          )}
        </>
      )}
      </section>
      {subscriptionActionRoot && subscriptionButton ? createPortal(subscriptionButton, subscriptionActionRoot) : null}
      {wideLayout && sidecarOpen && sidecarRoot && resourceSearch ? createPortal(resourceSearch, sidecarRoot) : null}
    </>
  )
}

function selectPreferredLibraryID(libraries: Library[], mediaType?: string): string {
  if (libraries.length === 0) return ''
  const normalizedType = (mediaType || '').toLowerCase()
  const exact = libraries.find((library) => library.type.toLowerCase() === normalizedType)
  if (exact) return exact.id
  if (normalizedType === 'anime') {
    const namedAnime = libraries.find((library) => /动漫|动画|番剧/i.test(library.name))
    if (namedAnime) return namedAnime.id
    const anime = libraries.find((library) => library.type.toLowerCase() === 'tv')
    if (anime) return anime.id
  }
  return libraries[0].id
}
