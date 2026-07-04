import { FormEvent, type ReactNode, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import toast from 'react-hot-toast'
import {
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  Download,
  Eye,
  Film,
  Filter,
  Loader2,
  RefreshCw,
  Rss,
  Search,
} from 'lucide-react'

import { sitesAPI, type QBitTorrentFile, type SiteAPIErrorPayload, type SiteCategory, type SiteDownloadInput, type SiteSearchResult } from '../api/sites'
import { imageURL } from '../api/client'
import type { Site } from '../types'

function fmtBytes(n: number): string {
  if (!n || n <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = n
  let idx = 0
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024
    idx++
  }
  return `${value.toFixed(1)} ${units[idx]}`
}

function unwrapSites(payload: unknown): Site[] {
  const data = payload as { data?: unknown; items?: unknown }
  if (Array.isArray(data.data)) return data.data as Site[]
  if (Array.isArray(data.items)) return data.items as Site[]
  if (Array.isArray(payload)) return payload as Site[]
  return []
}

function categoryKey(category: SiteCategory): string {
  return `${category.site_id || category.site_type || 'all'}:${category.id}:${category.name}`
}

function itemKey(item: SiteSearchResult, idx: number): string {
  return `${item.site_id}:${item.id || item.torrent_url || item.download_url || idx}`
}

function itemVisualKey(item: SiteSearchResult): string {
  return `${item.site_id}:${item.id || item.torrent_url || item.download_url || item.title}`
}

function cleanTitle(title: string): string {
  return title.length > 110 ? `${title.slice(0, 110)}...` : title
}

function apiErrorPayload(err: unknown): SiteAPIErrorPayload {
  const data = (err as { response?: { data?: unknown } })?.response?.data
  if (data && typeof data === 'object') return data as SiteAPIErrorPayload
  return {}
}

function apiErrorMessage(err: unknown, fallback: string): string {
  return apiErrorPayload(err).error || fallback
}

function downloadPrepareErrorMessage(err: unknown): string {
  const payload = apiErrorPayload(err)
  const code = payload.code || payload.error
  if (code === 'media_already_in_library' || payload.error === 'media already exists in library') {
    return payload.reason || '媒体库疑似已有该资源，已阻止重复下载'
  }
  if (code === 'download_already_exists' || payload.error === 'download already exists') {
    return payload.reason || '下载任务或 qB 种子已存在，已阻止重复提交；可到下载中心或 qB 检查同名任务'
  }
  return payload.error || '加入下载失败'
}

function categoryGroupOrder(group: string): number {
  const normalized = group.trim()
  const order = ['全部', '影视', '电影', '剧集', '动漫', '综艺', '纪录片', '成人']
  const idx = order.indexOf(normalized)
  return idx >= 0 ? idx : order.length
}

function detailFromItem(item: SiteSearchResult): Record<string, unknown> {
  return {
    id: item.id,
    title: item.title,
    subtitle: item.subtitle,
    category: item.category,
    poster_url: item.poster_url,
    backdrop_url: item.backdrop_url,
    size: item.size,
    seeders: item.seeders,
    leechers: item.leechers,
    snatched: item.snatched,
    free: item.free,
    adult: item.adult,
    upload_time: item.upload_time,
    detail_url: item.torrent_url,
    download_url: item.download_url,
    torrent_url: item.torrent_url,
    site_id: item.site_id,
    site_name: item.site_name,
  }
}

function mergeDetail(item: SiteSearchResult, incoming: Record<string, unknown>): Record<string, unknown> {
  const base = detailFromItem(item)
  for (const [key, value] of Object.entries(incoming)) {
    if (value === null || value === undefined || value === '') continue
    if (typeof value === 'number' && value === 0 && typeof base[key] === 'number' && Number(base[key]) > 0) {
      continue
    }
    if (key === 'upload_time' && value === '0001-01-01T00:00:00Z') continue
    base[key] = value
  }
  return base
}

function resourceVisual(item: SiteSearchResult | Record<string, unknown>): string {
  const poster = typeof item.poster_url === 'string' ? item.poster_url : ''
  const backdrop = typeof item.backdrop_url === 'string' ? item.backdrop_url : ''
  return poster || backdrop
}

function stringList(value: unknown): string[] {
  if (Array.isArray(value)) return value.map((item) => String(item)).filter(Boolean)
  if (typeof value === 'string' && value.trim()) return value.split(/[,，/|、]/).map((item) => item.trim()).filter(Boolean)
  return []
}

function detailVisual(detail: Record<string, unknown>): string {
  const visual = resourceVisual(detail)
  if (visual) return visual
  return stringList(detail.images)[0] || ''
}

const ptImageMaxRetries = 2

function ptImageURL(remote?: string, retryAttempt = 0, retryNonce = 0): string {
  const raw = (remote || '').trim()
  if (!raw) return ''
  const normalized = raw.startsWith('//') ? `https:${raw}` : raw
  if (retryAttempt <= 0) return imageURL(normalized)
  return imageURL(normalized, String(retryNonce || retryAttempt), { refreshCache: true, retryFailed: true })
}

const listVisualStorageKey = 'mediastation.pt.listVisuals.v1'
const maxStoredListVisuals = 600
const listVisualPrefetchLimit = 50
const listVisualPrefetchConcurrency = 3

function loadStoredListVisuals(): Record<string, string> {
  if (typeof window === 'undefined') return {}
  try {
    const parsed = JSON.parse(window.localStorage.getItem(listVisualStorageKey) || '{}') as Record<string, unknown>
    const out: Record<string, string> = {}
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof value === 'string' && value) out[key] = value
    }
    return out
  } catch {
    return {}
  }
}

function saveStoredListVisuals(values: Record<string, string>) {
  if (typeof window === 'undefined') return
  try {
    const entries = Object.entries(values).slice(-maxStoredListVisuals)
    window.localStorage.setItem(listVisualStorageKey, JSON.stringify(Object.fromEntries(entries)))
  } catch {
    // Ignore storage quota / private mode errors.
  }
}

function uniqueStrings(values: string[]): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const value of values) {
    const trimmed = value.trim()
    if (!trimmed || seen.has(trimmed)) continue
    seen.add(trimmed)
    out.push(trimmed)
  }
  return out
}

function detailToSearchResult(detail: Record<string, unknown>, fallbackSiteID = ''): SiteSearchResult {
  return {
    site_name: String(detail.site_name || ''),
    site_id: String(detail.site_id || fallbackSiteID || ''),
    id: String(detail.id || ''),
    title: String(detail.title || ''),
    subtitle: typeof detail.subtitle === 'string' ? detail.subtitle : undefined,
    poster_url: typeof detail.poster_url === 'string' ? detail.poster_url : undefined,
    backdrop_url: typeof detail.backdrop_url === 'string' ? detail.backdrop_url : undefined,
    overview: typeof detail.description === 'string' ? detail.description : undefined,
    torrent_url: String(detail.torrent_url || detail.detail_url || ''),
    download_url: String(detail.download_url || ''),
    category: typeof detail.category === 'string' ? detail.category : undefined,
    size: Number(detail.size || 0),
    seeders: Number(detail.seeders || 0),
    leechers: Number(detail.leechers || 0),
    snatched: Number(detail.snatched || 0),
    free: Boolean(detail.free),
    adult: Boolean(detail.adult),
    upload_time: typeof detail.upload_time === 'string' ? detail.upload_time : undefined,
  }
}

function parseYearHint(...values: Array<string | undefined>): number | undefined {
  for (const value of values) {
    const match = (value || '').match(/(?:^|[^\d])((?:19|20)\d{2})(?:[^\d]|$)/)
    if (!match) continue
    const year = Number.parseInt(match[1], 10)
    if (Number.isFinite(year) && year > 0) return year
  }
  return undefined
}

function resourceOriginalTitle(item: SiteSearchResult): string | undefined {
  const subtitle = (item.subtitle || '').trim()
  if (!subtitle) return undefined
  const first = subtitle.split(' · ')[0]?.trim() || subtitle
  return first.slice(0, 180) || undefined
}

function ResourceThumb({
  item,
  visual,
  className,
  iconSize,
  onClick,
}: {
  item: SiteSearchResult
  visual: string
  className: string
  iconSize: number
  onClick?: () => void
}) {
  const content = (
    <RetryablePTImage
      remote={visual}
      alt={item.title}
      className="h-full w-full object-cover"
      fallback={<Film size={iconSize} />}
    />
  )
  if (!onClick) {
    return <div className={className}>{content}</div>
  }
  return (
    <button
      className={className}
      onClick={onClick}
      type="button"
    >
      {content}
    </button>
  )
}

function RetryablePTImage({
  remote,
  alt,
  className,
  fallback = null,
  loading = 'eager',
}: {
  remote?: string
  alt: string
  className: string
  fallback?: ReactNode
  loading?: 'eager' | 'lazy'
}) {
  const [attempt, setAttempt] = useState(0)
  const [retryNonce, setRetryNonce] = useState(0)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    setAttempt(0)
    setRetryNonce(0)
    setFailed(false)
  }, [remote])

  const requestRetry = () => {
    if (attempt >= ptImageMaxRetries) {
      setFailed(true)
      return
    }
    setAttempt(attempt + 1)
    setRetryNonce(Date.now())
  }

  if (!remote || failed) return <>{fallback}</>
  return (
    <img
      key={`${remote}:${attempt}:${retryNonce}`}
      src={ptImageURL(remote, attempt, retryNonce)}
      alt={alt}
      loading={loading}
      decoding="async"
      referrerPolicy="no-referrer"
      className={className}
      onLoad={(event) => {
        const img = event.currentTarget
        if (img.naturalWidth <= 1 && img.naturalHeight <= 1) requestRetry()
      }}
      onError={requestRetry}
    />
  )
}

interface PreparedDownloadChoice {
  item: SiteSearchResult
  hash: string
  files: QBitTorrentFile[]
}

function siteDownloadInput(item: SiteSearchResult): SiteDownloadInput {
  return {
    site_id: item.site_id,
    id: item.id,
    title: item.title,
    download_url: item.download_url,
    torrent_url: item.torrent_url,
    poster_url: item.poster_url,
    backdrop_url: item.backdrop_url,
    overview: item.overview || item.subtitle,
    source_category: item.category,
    media_category: item.adult ? '成人' : undefined,
  }
}

export function PTResourcesPage() {
  const [sites, setSites] = useState<Site[]>([])
  const [categories, setCategories] = useState<SiteCategory[]>([])
  const [siteID, setSiteID] = useState('')
  const [category, setCategory] = useState('')
  const [keyword, setKeyword] = useState('')
  const [submittedKeyword, setSubmittedKeyword] = useState('')
  const [page, setPage] = useState(1)
  const [pageInput, setPageInput] = useState('1')
  const [items, setItems] = useState<SiteSearchResult[]>([])
  const [total, setTotal] = useState(0)
  const [pageSize, setPageSize] = useState(50)
  const [totalPages, setTotalPages] = useState(0)
  const [loading, setLoading] = useState(false)
  const [actingKey, setActingKey] = useState('')
  const [detail, setDetail] = useState<Record<string, unknown> | null>(null)
  const [detailLoading, setDetailLoading] = useState(false)
  const [downloadChoice, setDownloadChoice] = useState<PreparedDownloadChoice | null>(null)
  const [selectedDownloadFiles, setSelectedDownloadFiles] = useState<Set<number>>(new Set())
  const [categoryGroup, setCategoryGroup] = useState('')
  const listVisualsRef = useRef<Record<string, string>>(loadStoredListVisuals())
  const visualPrefetchingRef = useRef<Set<string>>(new Set())
  const downloadChoiceRef = useRef<PreparedDownloadChoice | null>(null)
  const confirmedPreparedHashesRef = useRef<Set<string>>(new Set())
  const [listVisuals, setListVisuals] = useState<Record<string, string>>(() => listVisualsRef.current)

  const loadSites = async () => {
    const payload = await sitesAPI.list()
    setSites(unwrapSites(payload).filter((site) => site.enabled !== false))
  }

  const loadCategories = async (nextSiteID = siteID) => {
    const rows = await sitesAPI.categories(nextSiteID)
    setCategories(rows)
    if (category && !rows.some((row) => row.id === category || row.name === category)) {
      setCategory('')
    }
  }

  const loadResources = async (nextPage = page, nextKeyword = submittedKeyword) => {
    const selected = categories.find((row) => row.id === category || row.name === category)
    setLoading(true)
    if (nextPage === 1) {
      setItems([])
      setTotal(0)
      setTotalPages(0)
      setPage(1)
      setPageInput('1')
    }
    try {
      const data = await sitesAPI.browse({
        site_id: siteID || undefined,
        category: category || undefined,
        keyword: nextKeyword || undefined,
        page: nextPage,
        include_adult: Boolean(selected?.adult),
      })
      setItems(data.items || [])
      setTotal(data.total || 0)
      setPage(data.page || nextPage)
      setPageInput(String(data.page || nextPage))
      setPageSize(data.page_size || 50)
      setTotalPages(data.total_pages || (data.total ? Math.ceil(data.total / (data.page_size || 50)) : 0))
    } catch (err: unknown) {
      const message =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error || '加载 PT 资源失败'
      toast.error(message)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    loadSites().catch(() => toast.error('加载 PT 资源中心失败'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    loadCategories(siteID).catch(() => toast.error('加载分类失败'))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [siteID])

  useEffect(() => {
    setPage(1)
    loadResources(1).catch(() => undefined)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [siteID, category])

  useEffect(() => {
    downloadChoiceRef.current = downloadChoice
  }, [downloadChoice])

  useEffect(() => {
    return () => {
      const current = downloadChoiceRef.current
      const hash = current?.hash
      if (!hash || confirmedPreparedHashesRef.current.has(hash)) return
      void sitesAPI.cancelPreparedDownload(hash).catch(() => undefined)
    }
  }, [])

  const cacheListVisual = useCallback((item: SiteSearchResult, visual: string) => {
    const key = itemVisualKey(item)
    if (!key || !visual) return
    setListVisuals((prev) => {
      if (prev[key]) return prev
      const next = { ...prev, [key]: visual }
      listVisualsRef.current = next
      saveStoredListVisuals(next)
      return next
    })
  }, [])

  useEffect(() => {
    for (const item of items) {
      const visual = resourceVisual(item)
      if (visual) cacheListVisual(item, visual)
    }
  }, [cacheListVisual, items])

  useEffect(() => {
    const missing = items
      .filter((item) => item.id && item.site_id && !resourceVisual(item) && !listVisualsRef.current[itemVisualKey(item)])
      .slice(0, listVisualPrefetchLimit)
    if (missing.length === 0) return undefined

    let cancelled = false
    let cursor = 0
    const controller = new AbortController()

    const worker = async () => {
      while (!cancelled) {
        const item = missing[cursor]
        cursor += 1
        if (!item) return
        const key = itemVisualKey(item)
        if (!key || visualPrefetchingRef.current.has(key) || listVisualsRef.current[key] || resourceVisual(item)) {
          continue
        }
        visualPrefetchingRef.current.add(key)
        try {
          const data = await sitesAPI.detail(item.site_id, item.id || '', controller.signal)
          const merged = mergeDetail(item, data as Record<string, unknown>)
          const visual = detailVisual(merged)
          if (!cancelled && visual) cacheListVisual(item, visual)
        } catch {
          // Background visual fill is best-effort; the row still opens normally.
        } finally {
          visualPrefetchingRef.current.delete(key)
        }
      }
    }

    const workers = Array.from({ length: Math.min(listVisualPrefetchConcurrency, missing.length) }, () => worker())
    void Promise.allSettled(workers)
    return () => {
      cancelled = true
      controller.abort()
    }
  }, [cacheListVisual, items])

  const groupedCategories = useMemo(() => {
    const byKey = new Map<string, SiteCategory>()
    for (const item of categories) {
      byKey.set(categoryKey(item), item)
    }
    const groups = new Map<string, SiteCategory[]>()
    for (const item of Array.from(byKey.values())) {
      const key = item.group || (siteID ? '其他' : item.site_name || '其他')
      groups.set(key, [...(groups.get(key) || []), item])
    }
    return Array.from(groups.entries()).sort(([a], [b]) => categoryGroupOrder(a) - categoryGroupOrder(b) || a.localeCompare(b, 'zh-Hans-CN'))
  }, [categories, siteID])

  const selectedSite = sites.find((site) => site.id === siteID)
  const selectedCategory = categories.find((row) => (row.id === category || row.name === category) && (!row.site_id || !siteID || row.site_id === siteID))
  const visibleCategoryGroup = categoryGroup || selectedCategory?.group || groupedCategories[0]?.[0] || ''
  const visibleCategoryRows = groupedCategories.find(([group]) => group === visibleCategoryGroup)?.[1] || []
  const selectedCategoryAdult = Boolean(selectedCategory?.adult)
  const canGoNext = totalPages > 0 ? page < totalPages : items.length > 0
  const resultCountText = loading ? '加载中' : `${total || items.length} 条`
  const resultText = `${selectedSite ? selectedSite.name : '全部站点'} / ${selectedCategory?.name || '全部'} / ${submittedKeyword || '全站'} / ${resultCountText}`

  const onSearch = (event: FormEvent) => {
    event.preventDefault()
    const next = keyword.trim()
    setSubmittedKeyword(next)
    setPage(1)
    loadResources(1, next)
  }

  const onJumpPage = (event: FormEvent) => {
    event.preventDefault()
    const parsed = Number.parseInt(pageInput, 10)
    if (!Number.isFinite(parsed) || parsed <= 0) {
      toast.error('页码不正确')
      setPageInput(String(page))
      return
    }
    const next = totalPages > 0 ? Math.min(parsed, totalPages) : parsed
    loadResources(next)
  }

  const visualForItem = (item: SiteSearchResult): string => resourceVisual(item) || listVisuals[itemVisualKey(item)] || ''

  const onDownload = async (item: SiteSearchResult) => {
    const key = `download:${itemKey(item, 0)}`
    setActingKey(key)
    const toastID = toast.loading('正在提交到 qB 并读取文件列表...')
    try {
      const prepared = await sitesAPI.prepareDownload(siteDownloadInput(item))
      const files = prepared.files || []
      setDownloadChoice({ item, hash: prepared.hash, files })
      setSelectedDownloadFiles(new Set(files.map((file) => file.index)))
      toast.success(files.length > 0 ? '已读取 qB 文件列表，请选择下载内容' : '已加入 qB 暂停任务，可确认后开始下载', { id: toastID })
    } catch (err: unknown) {
      toast.error(downloadPrepareErrorMessage(err), { id: toastID })
    } finally {
      setActingKey('')
    }
  }

  const closeDownloadChoice = async () => {
    const current = downloadChoice
    setDownloadChoice(null)
    setSelectedDownloadFiles(new Set())
    if (!current?.hash) return
    try {
      await sitesAPI.cancelPreparedDownload(current.hash)
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '取消 qB 暂停任务失败'))
    }
  }

  const confirmDownloadChoice = async () => {
    if (!downloadChoice) return
    const { item, hash, files } = downloadChoice
    const key = `download:${itemKey(item, 0)}`
    setActingKey(key)
    const toastID = toast.loading('正在应用文件选择并开始下载...')
    try {
      const selectedIndexes =
        files.length > 0 && selectedDownloadFiles.size < files.length ? Array.from(selectedDownloadFiles) : undefined
      await sitesAPI.confirmDownload({
        ...siteDownloadInput(item),
        hash,
        selected_file_indexes: selectedIndexes,
      })
      confirmedPreparedHashesRef.current.add(hash)
      toast.success('已开始下载', { id: toastID })
      setDownloadChoice(null)
      setSelectedDownloadFiles(new Set())
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '开始下载失败'), { id: toastID })
    } finally {
      setActingKey('')
    }
  }

  const onSubscribe = async (item: SiteSearchResult) => {
    const key = `subscribe:${itemKey(item, 0)}`
    setActingKey(key)
    const toastID = toast.loading('正在创建订阅并执行搜索...')
    try {
      const data = await sitesAPI.subscribe({
        site_id: item.site_id,
        id: item.id,
        category: selectedCategory?.id || undefined,
        include_adult: selectedCategoryAdult || item.adult,
        name: item.title,
        keyword: item.title,
        filter: item.title,
        original_title: resourceOriginalTitle(item),
        year: parseYearHint(item.title, item.subtitle),
        media_category: item.adult ? '成人' : undefined,
        poster_url: item.poster_url,
        backdrop_url: item.backdrop_url,
        overview: item.overview || item.subtitle,
      })
      const queued = Number(data?.queued || 0)
      const searchKeyword = typeof data?.search_keyword === 'string' ? data.search_keyword : ''
      const keywordHint = searchKeyword ? `；关键词：${searchKeyword}` : ''
      toast.success(queued > 0 ? `已创建订阅并加入 ${queued} 个下载${keywordHint}` : `已创建订阅，暂未命中资源${keywordHint}；可到订阅中心查看执行日志`, { id: toastID })
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '创建订阅失败'), { id: toastID })
    } finally {
      setActingKey('')
    }
  }

  const onDetail = async (item: SiteSearchResult) => {
    setDetail(detailFromItem(item))
    setDetailLoading(true)
    if (!item.id) {
      setDetailLoading(false)
      return
    }
    const key = `detail:${itemKey(item, 0)}`
    setActingKey(key)
    try {
      const data = await sitesAPI.detail(item.site_id, item.id)
      const merged = mergeDetail(item, data as Record<string, unknown>)
      setDetail(merged)
      const visual = detailVisual(merged)
      if (visual) cacheListVisual(item, visual)
    } catch (err: unknown) {
      toast.error(apiErrorMessage(err, '站点详情接口不可用，已显示列表信息'))
    } finally {
      setActingKey('')
      setDetailLoading(false)
    }
  }

  const detailDescription = typeof detail?.description === 'string' ? detail.description : ''
  const detailFiles = Array.isArray(detail?.files) ? detail.files.map((file) => String(file)) : []
  const detailGenres = stringList(detail?.genres)
  const detailTags = stringList(detail?.tags)
  const detailPoster = typeof detail?.poster_url === 'string' ? detail.poster_url : ''
  const detailBackdrop = typeof detail?.backdrop_url === 'string' ? detail.backdrop_url : ''
  const detailSubtitle = typeof detail?.subtitle === 'string' ? detail.subtitle : ''
  const detailImages = uniqueStrings(stringList(detail?.images)).filter((url) => url !== detailPoster && url !== detailBackdrop)
  const detailDownloadItem: SiteSearchResult | null = detail ? detailToSearchResult(detail, siteID) : null
  const downloadChoiceFiles = downloadChoice?.files || []
  const downloadChoiceItem = downloadChoice?.item || null
  const selectedDownloadFileList = downloadChoiceFiles.filter((file) => selectedDownloadFiles.has(file.index))

  return (
    <div className="space-y-6">
      <header className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="flex items-center gap-3">
          <Filter className="h-6 w-6 text-brand-500" />
          <div>
            <h1 className="font-display text-3xl font-bold text-ink-600">PT 资源中心</h1>
            <p className="text-sm text-ink-50">站点资源、分类、成人资源、订阅下载</p>
          </div>
        </div>
        <button className="neon-button flex items-center gap-2" onClick={() => loadResources(page)} disabled={loading}>
          <RefreshCw size={16} className={loading ? 'animate-spin' : ''} />
          刷新
        </button>
      </header>

      <section className="grid gap-3 md:grid-cols-3">
        {sites.map((site) => (
          <button
            key={site.id}
            className={`glass-panel text-left transition ${
              siteID === site.id ? '!border-primary-400/70 !bg-primary-400/10' : 'hover:border-primary-400/40'
            }`}
            onClick={() => setSiteID(siteID === site.id ? '' : site.id)}
          >
            <div className="flex items-center justify-between gap-3">
              <p className="font-semibold text-ink-600">{site.name}</p>
              <span className="rounded border border-gray-200 px-2 py-0.5 text-xs text-ink-100">{site.type}</span>
            </div>
            <div className="mt-2 flex flex-wrap gap-2 text-xs">
              <span className={site.login_status === 'ok' ? 'text-emerald-500' : 'text-sand-500'}>
                {site.login_status || 'unknown'}
              </span>
              {site.downloader && <span className="text-ink-50">{site.downloader}</span>}
              {site.rss_url && <span className="text-brand-500">RSS</span>}
            </div>
          </button>
        ))}
        {!sites.length && (
          <div className="glass-panel md:col-span-3 text-sand-500">暂无已启用站点。</div>
        )}
      </section>

      <section className="glass-panel space-y-4">
        <form onSubmit={onSearch} className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_180px_140px]">
          <div className="flex min-w-0 items-center gap-2">
            <Search size={16} className="text-sand-500" />
            <input
              className="input-base min-w-0 flex-1"
              placeholder="关键词"
              value={keyword}
              onChange={(event) => setKeyword(event.target.value)}
            />
          </div>
          <select
            className="input-base"
            value={siteID}
            onChange={(event) => setSiteID(event.target.value)}
          >
            <option value="">全部站点</option>
            {sites.map((site) => (
              <option key={site.id} value={site.id}>
                {site.name}
              </option>
            ))}
          </select>
          <button className="neon-button" type="submit" disabled={loading}>
            {loading ? '加载中...' : '搜索'}
          </button>
        </form>

        <div className="-mx-1 overflow-x-auto px-1 pb-1 md:mx-0 md:overflow-visible md:px-0 md:pb-0">
          <div className="flex w-max min-w-full items-center gap-2 md:w-auto md:flex-wrap">
            <button
              className={`shrink-0 rounded border px-3 py-1.5 text-sm ${
                category === '' ? 'border-primary-400 bg-primary-400/10 text-brand-500' : 'border-gray-200 text-ink-100'
              }`}
              onClick={() => setCategory('')}
            >
              全部
            </button>
            {groupedCategories.map(([group, rows]) => (
              <button
                key={group}
                type="button"
                className={`shrink-0 rounded border px-3 py-1.5 text-sm ${
                  visibleCategoryGroup === group
                    ? 'border-primary-400 bg-primary-400/10 text-brand-500'
                    : 'border-gray-200 text-ink-100 hover:border-primary-400/40'
                }`}
                onClick={() => {
                  setCategoryGroup(group)
                  setCategory('')
                }}
              >
                <span className="inline-block max-w-[9rem] truncate align-bottom">{group}</span>
                <span className="ml-1 text-xs text-sand-500">{rows.length}</span>
              </button>
            ))}
          </div>
        </div>

        <div className="rounded border border-gray-100 bg-white/50 p-3">
          <div className="mb-2 flex items-center justify-between gap-3">
            <span className="min-w-0 truncate text-xs font-medium text-sand-500">{visibleCategoryGroup || '分类'}</span>
            <span className="shrink-0 text-[11px] text-sand-500">{visibleCategoryRows.length} 项</span>
          </div>
          <div className="-mx-1 overflow-x-auto px-1 pb-1 md:mx-0 md:overflow-visible md:px-0 md:pb-0">
            <div className="flex w-max min-w-full items-center gap-2 md:w-auto md:flex-wrap">
              {visibleCategoryRows.map((row) => (
                <button
                  key={categoryKey(row)}
                  className={`shrink-0 rounded border px-2.5 py-1 text-xs ${
                    (category === row.id || category === row.name) && (!row.site_id || !siteID || row.site_id === siteID)
                      ? row.adult
                        ? 'border-red-400 bg-red-400/10 text-red-500'
                        : 'border-primary-400 bg-primary-400/10 text-brand-500'
                      : 'border-gray-200 text-ink-100 hover:border-primary-400/40'
                  }`}
                  onClick={() => {
                    if (row.site_id && siteID !== row.site_id) setSiteID(row.site_id)
                    setCategoryGroup(row.group || visibleCategoryGroup)
                    setCategory(row.id)
                  }}
                >
                  {row.adult && <AlertTriangle size={12} className="mr-1 inline" />}
                  <span className="inline-block max-w-[10rem] truncate align-bottom">{row.name}</span>
                </button>
              ))}
            </div>
          </div>
        </div>
      </section>

      <section className="glass-panel">
        <div className="mb-4 flex flex-col gap-3 border-b border-gray-200 bg-white/90 py-3 md:sticky md:top-0 md:z-30 md:-mx-5 md:flex-row md:items-center md:justify-between md:bg-white/95 md:px-5 md:backdrop-blur">
          <div className="min-w-0 text-sm text-ink-50">
            <p>{resultText}</p>
            <p className="mt-1 text-xs text-sand-500">
              第 {page} 页{totalPages > 0 ? ` / 共 ${totalPages} 页` : ''} · 每页 {pageSize} 条
            </p>
          </div>
          <div className="fixed bottom-[calc(env(safe-area-inset-bottom)+1rem)] right-4 z-40 flex items-center gap-1 rounded-full border border-gray-200 bg-white/95 p-1.5 shadow-lg backdrop-blur md:static md:flex-wrap md:gap-2 md:rounded-none md:border-0 md:bg-transparent md:p-0 md:shadow-none md:backdrop-blur-0">
            <button
              className="flex h-9 w-9 items-center justify-center rounded-full border border-gray-200 text-ink-100 disabled:opacity-40 md:h-auto md:w-auto md:rounded md:p-2"
              disabled={page <= 1 || loading}
              onClick={() => loadResources(page - 1)}
              title="上一页"
            >
              <ChevronLeft size={16} />
            </button>
            <form onSubmit={onJumpPage} className="flex items-center gap-1.5 md:gap-2">
              <input
                className="input-base h-9 w-14 px-1 py-1 text-center text-sm md:w-20 md:px-2"
                inputMode="numeric"
                value={pageInput}
                onChange={(event) => setPageInput(event.target.value)}
                disabled={loading}
              />
              <button className="btn-outline h-9 justify-center rounded-full px-2 py-1 text-xs md:rounded md:px-3" type="submit" disabled={loading}>
                跳转
              </button>
            </form>
            <button
              className="flex h-9 w-9 items-center justify-center rounded-full border border-gray-200 text-ink-100 disabled:opacity-40 md:h-auto md:w-auto md:rounded md:p-2"
              disabled={loading || !canGoNext}
              onClick={() => loadResources(page + 1)}
              title="下一页"
            >
              <ChevronRight size={16} />
            </button>
          </div>
        </div>

        <div className="grid gap-3 pb-20 lg:hidden">
          {items.map((item, idx) => {
            const key = itemKey(item, idx)
            return (
              <ResourceCard
                key={key}
                item={item}
                itemKeyValue={key}
                actingKey={actingKey}
                visual={visualForItem(item)}
                onDetail={onDetail}
                onDownload={onDownload}
                onSubscribe={onSubscribe}
              />
            )
          })}
        </div>

        <div className="hidden overflow-x-auto lg:block">
          <table className="w-full min-w-[900px] text-left text-sm">
          <thead className="text-xs uppercase tracking-wider text-sand-500">
            <tr>
              <th className="py-2">图</th>
              <th className="py-2">站点</th>
              <th>标题</th>
              <th>分类</th>
              <th>大小</th>
              <th>S</th>
              <th>L</th>
              <th>完成</th>
              <th className="text-right">操作</th>
            </tr>
          </thead>
          <tbody>
            {items.map((item, idx) => {
              const key = itemKey(item, idx)
              const visual = visualForItem(item)
              return (
                <tr key={key} className="border-t border-gray-200 align-top">
                  <td className="py-3">
                    <ResourceThumb
                      item={item}
                      visual={visual}
                      className="flex h-16 w-11 items-center justify-center overflow-hidden rounded border border-gray-200 bg-gray-100 text-sand-500"
                      iconSize={16}
                    />
                  </td>
                  <td className="py-3 text-brand-500">{item.site_name}</td>
                  <td className="max-w-xl py-3">
                    <div className="flex flex-wrap items-center gap-2">
                      {item.free && <span className="rounded border border-emerald-400/40 px-1.5 py-0.5 text-xs text-emerald-500">Free</span>}
                      {item.adult && <span className="rounded border border-red-400/40 px-1.5 py-0.5 text-xs text-red-500">成人</span>}
                    </div>
                    <p className="mt-1 text-left text-ink-600">
                      {cleanTitle(item.title)}
                    </p>
                    {item.subtitle && <p className="mt-1 text-xs text-ink-50">{cleanTitle(item.subtitle)}</p>}
                    {item.upload_time && <p className="mt-1 text-xs text-sand-500">{new Date(item.upload_time).toLocaleString()}</p>}
                  </td>
                  <td className="whitespace-nowrap text-ink-100">{item.category || '-'}</td>
                  <td className="whitespace-nowrap text-ink-100">{fmtBytes(item.size)}</td>
                  <td className="text-emerald-400">{item.seeders || '-'}</td>
                  <td className="text-red-400">{item.leechers || '-'}</td>
                  <td className="text-ink-100">{item.snatched || '-'}</td>
                  <td className="py-3 text-right">
                    <div className="flex justify-end gap-2">
                      <button
                        className="rounded border border-gray-200 p-1.5 text-ink-100 hover:border-primary-400/50"
                        onClick={() => onDetail(item)}
                        disabled={actingKey === `detail:${key}`}
                        title="详情"
                      >
                        <Eye size={14} />
                      </button>
                      <button
                        className="rounded border border-primary-400/50 p-1.5 text-brand-500 hover:bg-primary-400/10"
                        onClick={() => onDownload(item)}
                        disabled={actingKey === `download:${key}`}
                        title="下载"
                      >
                        {actingKey === `download:${key}` ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
                      </button>
                      <button
                        className="rounded border border-emerald-400/50 p-1.5 text-emerald-500 hover:bg-emerald-400/10"
                        onClick={() => onSubscribe(item)}
                        disabled={actingKey === `subscribe:${key}`}
                        title="订阅"
                      >
                        {actingKey === `subscribe:${key}` ? <Loader2 size={14} className="animate-spin" /> : <Rss size={14} />}
                      </button>
                    </div>
                  </td>
                </tr>
              )
            })}
          </tbody>
          </table>
        </div>

        {!loading && items.length === 0 && (
          <div className="py-10 text-center text-sand-500">暂无资源。</div>
        )}
      </section>

      {downloadChoice && downloadChoiceItem && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4" onClick={() => void closeDownloadChoice()}>
          <div className="glass-panel max-h-[82vh] w-full max-w-3xl overflow-y-auto" onClick={(event) => event.stopPropagation()}>
            <div className="mb-4 flex items-center justify-between gap-3">
              <div className="min-w-0">
                <h2 className="truncate font-display text-xl font-bold text-ink-600">{downloadChoiceItem.title || '选择下载内容'}</h2>
                <p className="mt-1 text-xs text-sand-500">
                  {downloadChoiceFiles.length > 0 ? `已选择 ${selectedDownloadFileList.length}/${downloadChoiceFiles.length} 个文件` : 'qB 暂未返回文件列表，确认后会开始完整任务'}
                </p>
              </div>
              <button className="rounded border border-gray-200 px-3 py-1 text-sm text-ink-100" onClick={() => void closeDownloadChoice()}>
                关闭
              </button>
            </div>

            {downloadChoiceFiles.length > 0 ? (
              <div className="space-y-3">
                <div className="flex flex-wrap items-center gap-2">
                  <button className="rounded border border-gray-200 px-2 py-1 text-xs" onClick={() => setSelectedDownloadFiles(new Set(downloadChoiceFiles.map((file) => file.index)))}>
                    全选
                  </button>
                  <button className="rounded border border-gray-200 px-2 py-1 text-xs" onClick={() => setSelectedDownloadFiles(new Set())}>
                    清空
                  </button>
                </div>
                <div className="max-h-[46vh] space-y-1 overflow-y-auto pr-1 text-xs text-ink-100">
                  {downloadChoiceFiles.map((file) => (
                    <label key={`${file.index}:${file.name}`} className="flex cursor-pointer items-start gap-2 rounded bg-white/50 px-2 py-1">
                      <input
                        type="checkbox"
                        className="mt-0.5"
                        checked={selectedDownloadFiles.has(file.index)}
                        onChange={(event) => {
                          setSelectedDownloadFiles((prev) => {
                            const next = new Set(prev)
                            if (event.target.checked) next.add(file.index)
                            else next.delete(file.index)
                            return next
                          })
                        }}
                      />
                      <span className="min-w-0 flex-1 break-all">{file.name}</span>
                      <span className="shrink-0 text-sand-500">{fmtBytes(file.size)}</span>
                    </label>
                  ))}
                </div>
              </div>
            ) : (
              <div className="rounded border border-gray-200 bg-white/60 p-4 text-sm text-ink-100">
                qB 暂未返回文件列表。确认后会恢复完整任务；不想下载请关闭或取消。
              </div>
            )}

            <div className="mt-5 flex justify-end gap-2">
              <button className="rounded border border-gray-200 px-3 py-2 text-sm text-ink-100" onClick={() => void closeDownloadChoice()}>
                取消
              </button>
              <button
                className="btn-primary px-4 py-2 text-sm"
                disabled={(downloadChoiceFiles.length > 0 && selectedDownloadFileList.length === 0) || actingKey === `download:${itemKey(downloadChoiceItem, 0)}`}
                onClick={() => void confirmDownloadChoice()}
              >
                {actingKey === `download:${itemKey(downloadChoiceItem, 0)}` ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
                确认下载
              </button>
            </div>
          </div>
        </div>
      )}

      {detail && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 px-4" onClick={() => setDetail(null)}>
          <div className="glass-panel max-h-[86vh] w-full max-w-5xl overflow-y-auto" onClick={(event) => event.stopPropagation()}>
            <div className="mb-4 flex items-center justify-between gap-3">
              <div className="min-w-0">
                <h2 className="truncate font-display text-xl font-bold text-ink-600">{String(detail.title || detail.id || '详情')}</h2>
                {detailLoading && <p className="mt-1 text-xs text-sand-500">正在加载详情...</p>}
              </div>
              <div className="flex shrink-0 items-center gap-2">
                {detailDownloadItem && (
                  <button
                    className="btn-primary px-3 py-1.5 text-sm"
                    onClick={() => onDownload(detailDownloadItem)}
                    disabled={actingKey === `download:${itemKey(detailDownloadItem, 0)}`}
                  >
                    {actingKey === `download:${itemKey(detailDownloadItem, 0)}` ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
                    下载
                  </button>
                )}
                <button className="rounded border border-gray-200 px-3 py-1 text-sm text-ink-100" onClick={() => setDetail(null)}>
                  关闭
                </button>
              </div>
            </div>

            {detailBackdrop && (
              <div className="mb-4 overflow-hidden rounded border border-gray-200 bg-gray-100">
                <RetryablePTImage
                  remote={detailBackdrop}
                  alt=""
                  className="h-44 w-full object-cover"
                />
              </div>
            )}

            <div className="grid gap-5 lg:grid-cols-[220px_minmax(0,1fr)]">
              <div className="space-y-3">
                {detailPoster ? (
                  <RetryablePTImage
                    remote={detailPoster}
                    alt={String(detail.title || '')}
                    className="aspect-[2/3] w-full rounded border border-gray-200 bg-gray-100 object-cover"
                    fallback={<div className="flex aspect-[2/3] w-full items-center justify-center rounded border border-gray-200 bg-gray-100 text-sm text-sand-500">无图片</div>}
                  />
                ) : (
                  <div className="flex aspect-[2/3] w-full items-center justify-center rounded border border-gray-200 bg-gray-100 text-sm text-sand-500">无图片</div>
                )}
                <div className="grid grid-cols-3 gap-2 text-xs">
                  <InfoPill label="S" value={String(detail.seeders || '-')} tone="seed" />
                  <InfoPill label="L" value={String(detail.leechers || '-')} tone="leech" />
                  <InfoPill label="完成" value={String(detail.snatched || '-')} />
                </div>
              </div>

              <div className="min-w-0 space-y-4">
                {detailSubtitle && <p className="break-words text-sm leading-6 text-ink-100">{detailSubtitle}</p>}
                <div className="grid gap-3 text-sm md:grid-cols-2 xl:grid-cols-3">
                  <Info label="站点" value={String(detail.site_name || '-')} />
                  <Info label="分类" value={String(detail.category || '-')} />
                  <Info label="大小" value={fmtBytes(Number(detail.size || 0))} />
                  <Info label="年份" value={String(detail.year || '-')} />
                  <Info label="评分" value={String(detail.rating || '-')} />
                  <Info label="InfoHash" value={String(detail.info_hash || '-')} />
                </div>
                {(detailGenres.length > 0 || detailTags.length > 0) && (
                  <div className="flex flex-wrap gap-2">
                    {[...detailGenres, ...detailTags].map((tag) => (
                      <span key={tag} className="rounded border border-gray-200 bg-white/60 px-2 py-1 text-xs text-ink-100">{tag}</span>
                    ))}
                  </div>
                )}
                {detailDescription && (
                  <pre className="whitespace-pre-wrap rounded border border-gray-200 bg-white/60 p-3 text-xs leading-5 text-ink-100">
                    {detailDescription}
                  </pre>
                )}
                {detailImages.length > 0 && (
                  <div className="grid gap-3 sm:grid-cols-2">
                    {detailImages.slice(0, 12).map((url) => (
                      <a
                        key={url}
                        className="overflow-hidden rounded border border-gray-200 bg-gray-100"
                        href={ptImageURL(url)}
                        target="_blank"
                        rel="noopener noreferrer"
                      >
                        <RetryablePTImage
                          remote={url}
                          alt=""
                          className="max-h-80 w-full object-contain"
                          fallback={<div className="flex h-32 items-center justify-center text-sand-500"><Film size={22} /></div>}
                        />
                      </a>
                    ))}
                  </div>
                )}
                {detailFiles.length > 0 && (
                  <div className="space-y-1 text-xs text-ink-100">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <p className="font-semibold text-ink-600">文件列表</p>
                      <span className="text-[11px] text-sand-500">{detailFiles.length} 个文件</span>
                    </div>
                    {detailFiles.slice(0, 120).map((file) => (
                      <div key={file} className="rounded bg-white/50 px-2 py-1">
                        <span className="break-all">{file}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

function ResourceCard({
  item,
  itemKeyValue,
  actingKey,
  visual,
  onDetail,
  onDownload,
  onSubscribe,
}: {
  item: SiteSearchResult
  itemKeyValue: string
  actingKey: string
  visual: string
  onDetail: (item: SiteSearchResult) => void
  onDownload: (item: SiteSearchResult) => void
  onSubscribe: (item: SiteSearchResult) => void
}) {
  return (
    <article className="overflow-hidden rounded-lg border border-gray-200 bg-white p-3 shadow-sm">
      <div className="flex gap-3">
        <ResourceThumb
          item={item}
          visual={visual}
          className="flex h-28 w-20 shrink-0 items-center justify-center overflow-hidden rounded border border-gray-200 bg-gray-100 text-sand-500"
          iconSize={22}
        />
        <div className="min-w-0 flex-1">
          <div className="mb-1 flex flex-wrap items-center gap-1.5 text-[11px]">
            <span className="max-w-[7rem] truncate rounded border border-gray-200 px-1.5 py-0.5 text-brand-500">{item.site_name}</span>
            {item.category && <span className="max-w-[10rem] truncate rounded border border-gray-200 px-1.5 py-0.5 text-ink-100">{item.category}</span>}
            {item.free && <span className="rounded border border-emerald-400/40 px-1.5 py-0.5 text-emerald-500">Free</span>}
            {item.adult && <span className="rounded border border-red-400/40 px-1.5 py-0.5 text-red-500">成人</span>}
          </div>
          <p className="line-clamp-2 text-left text-sm font-semibold leading-5 text-ink-600">
            {item.title}
          </p>
          {item.subtitle && <p className="mt-1 line-clamp-2 text-xs text-ink-50">{item.subtitle}</p>}
          <div className="mt-2 grid grid-cols-4 gap-2 text-xs">
            <InfoPill label="大小" value={fmtBytes(item.size)} />
            <InfoPill label="S" value={String(item.seeders || '-')} tone="seed" />
            <InfoPill label="L" value={String(item.leechers || '-')} tone="leech" />
            <InfoPill label="完成" value={String(item.snatched || '-')} />
          </div>
        </div>
      </div>
      <div className="mt-3 grid grid-cols-3 gap-2">
        <button
          className="btn-outline justify-center px-2 py-2 text-xs"
          onClick={() => onDetail(item)}
          disabled={actingKey === `detail:${itemKeyValue}`}
        >
          <Eye size={14} />
          详情
        </button>
        <button
          className="btn-outline justify-center px-2 py-2 text-xs text-brand-500"
          onClick={() => onDownload(item)}
          disabled={actingKey === `download:${itemKeyValue}`}
        >
          {actingKey === `download:${itemKeyValue}` ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
          {actingKey === `download:${itemKeyValue}` ? '下载中' : '下载'}
        </button>
        <button
          className="btn-outline justify-center px-2 py-2 text-xs text-emerald-500"
          onClick={() => onSubscribe(item)}
          disabled={actingKey === `subscribe:${itemKeyValue}`}
        >
          {actingKey === `subscribe:${itemKeyValue}` ? <Loader2 size={14} className="animate-spin" /> : <Rss size={14} />}
          {actingKey === `subscribe:${itemKeyValue}` ? '订阅中' : '订阅'}
        </button>
      </div>
    </article>
  )
}

function InfoPill({ label, value, tone }: { label: string; value: string; tone?: 'seed' | 'leech' }) {
  const color = tone === 'seed' ? 'text-emerald-500' : tone === 'leech' ? 'text-red-500' : 'text-ink-600'
  return (
    <div className="min-w-0 rounded border border-gray-100 bg-gray-50 px-2 py-1">
      <p className="truncate text-[10px] text-sand-500">{label}</p>
      <p className={`truncate font-semibold ${color}`}>{value}</p>
    </div>
  )
}

function Info({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded border border-gray-200 bg-white/50 p-3">
      <p className="text-xs text-sand-500">{label}</p>
      <p className="mt-1 break-all text-ink-600">{value}</p>
    </div>
  )
}
