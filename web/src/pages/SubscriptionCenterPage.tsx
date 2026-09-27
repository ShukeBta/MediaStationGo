import { useEffect, useMemo, useState } from 'react'
import toast from 'react-hot-toast'
import { CheckSquare, Loader2, RefreshCw, Rss, Search, Square, X } from 'lucide-react'

import { imageURL } from '../api/client'
import { discoverAPI, type DiscoverItem, type DiscoverSection } from '../api/discover'
import { subscriptionsAPI } from '../api/subscriptions'
import { DiscoverDetailModal } from './DiscoverDetailModal'

const defaultSections = ['tmdb_trending_day', 'tmdb_popular_movie', 'tmdb_popular_tv', 'douban_hot_movie', 'douban_hot_tv']
const pageSize = 40

export function SubscriptionCenterPage() {
  const [sections, setSections] = useState<DiscoverSection[]>([])
  const [selectedSections, setSelectedSections] = useState<string[]>(defaultSections)
  const [source, setSource] = useState('all')
  const [mediaType, setMediaType] = useState('')
  const [q, setQ] = useState('')
  const [page, setPage] = useState(1)
  const [items, setItems] = useState<DiscoverItem[]>([])
  const [selectedKeys, setSelectedKeys] = useState<string[]>([])
  const [activeItem, setActiveItem] = useState<DiscoverItem | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [bulkBusy, setBulkBusy] = useState(false)

  useEffect(() => {
    discoverAPI
      .sections()
      .then((data) => {
        setSections(data)
        const allowed = new Set(data.map((section) => section.key))
        setSelectedSections(defaultSections.filter((key) => allowed.has(key)))
      })
      .catch(() => setSections([]))
  }, [])

  const searchMode = q.trim().length > 0

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError('')
    setSelectedKeys([])
    const run = async () => {
      if (searchMode) {
        const data = await discoverAPI.search(q.trim(), source, mediaType, page, pageSize)
        if (cancelled) return
        setItems(data.items)
        setError(data.error || '')
        return
      }
      const keys = selectedSections.length > 0 ? selectedSections : defaultSections
      const feed = await discoverAPI.feedPage(keys, page, pageSize)
      if (cancelled) return
      const merged = keys.flatMap((key) => feed[key] ?? [])
      setItems(filterBySourceAndType(dedupeItems(merged), source, mediaType))
    }
    run()
      .catch((err) => {
        if (cancelled) return
        const msg =
          (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
          '加载订阅中心失败'
        setItems([])
        setError(msg)
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [q, source, mediaType, page, selectedSections, searchMode])

  const selectedItems = useMemo(() => {
    const set = new Set(selectedKeys)
    return items.filter((item, index) => set.has(itemKey(item, index)))
  }, [items, selectedKeys])

  const toggleSection = (key: string) => {
    setPage(1)
    setSelectedSections((current) => current.includes(key) ? current.filter((item) => item !== key) : [...current, key])
  }

  const toggleItem = (key: string) => {
    setSelectedKeys((current) => current.includes(key) ? current.filter((item) => item !== key) : [...current, key])
  }

  const bulkSubscribe = async () => {
    if (selectedItems.length === 0) return
    setBulkBusy(true)
    try {
      let created = 0
      let queued = 0
      for (const item of selectedItems) {
        const sub = await subscriptionsAPI.create(subscriptionPayload(item))
        created += 1
        if (sub?.id) {
          const run = await subscriptionsAPI.runNow(sub.id)
          queued += Number(run?.queued || 0)
        }
      }
      toast.success(queued > 0 ? `已创建 ${created} 个订阅，并加入 ${queued} 个下载` : `已创建 ${created} 个订阅，暂未命中资源`)
      setSelectedKeys([])
    } catch (err) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        '批量创建订阅失败'
      toast.error(msg)
    } finally {
      setBulkBusy(false)
    }
  }

  return (
    <div className="mx-auto max-w-7xl space-y-5 px-4 py-6">
      <header className="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <h1 className="font-display text-3xl font-bold text-ink-600">订阅中心</h1>
          <p className="mt-1 text-sm text-ink-50">从 TMDb / 豆瓣 / Bangumi 查找条目，选择后创建自动订阅。</p>
        </div>
        <button
          type="button"
          onClick={bulkSubscribe}
          disabled={selectedItems.length === 0 || bulkBusy}
          className="neon-button disabled:opacity-50"
        >
          {bulkBusy ? <Loader2 size={16} className="animate-spin" /> : <Rss size={16} />}
          批量创建订阅 {selectedItems.length > 0 ? `(${selectedItems.length})` : ''}
        </button>
      </header>

      <section className="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm">
        <div className="grid gap-3 lg:grid-cols-[1fr_160px_160px_120px]">
          <label className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-sand-500" />
            <input
              className="input-base pl-9"
              placeholder="搜索电影、剧集、动漫..."
              value={q}
              onChange={(event) => {
                setPage(1)
                setQ(event.target.value)
              }}
            />
            {q && (
              <button
                type="button"
                className="absolute right-3 top-1/2 -translate-y-1/2 text-sand-500 hover:text-ink-600"
                onClick={() => {
                  setPage(1)
                  setQ('')
                }}
              >
                <X size={16} />
              </button>
            )}
          </label>
          <select className="input-base" value={source} onChange={(event) => { setPage(1); setSource(event.target.value) }}>
            <option value="all">全部来源</option>
            <option value="tmdb">TMDb</option>
            <option value="douban">豆瓣</option>
            <option value="bangumi">Bangumi</option>
          </select>
          <select className="input-base" value={mediaType} onChange={(event) => { setPage(1); setMediaType(event.target.value) }}>
            <option value="">全部类型</option>
            <option value="movie">电影</option>
            <option value="tv">电视剧</option>
            <option value="anime">动漫</option>
            <option value="variety">综艺</option>
          </select>
          <button type="button" className="btn-outline justify-center" onClick={() => setPage(1)}>
            <RefreshCw size={15} />
            刷新
          </button>
        </div>

        {!searchMode && sections.length > 0 && (
          <div className="mt-4 flex flex-wrap gap-2">
            {sections.map((section) => {
              const active = selectedSections.includes(section.key)
              return (
                <button
                  key={section.key}
                  type="button"
                  onClick={() => toggleSection(section.key)}
                  className={
                    'rounded-full border px-3 py-1.5 text-xs font-semibold transition ' +
                    (active
                      ? 'border-primary-400 bg-primary-400/15 text-brand-500'
                      : 'border-gray-200 bg-gray-50 text-gray-500 hover:border-primary-300 hover:text-ink-600')
                  }
                >
                  {section.label}
                </button>
              )
            })}
          </div>
        )}
      </section>

      {error && <div className="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-600">{error}</div>}

      <div className="flex items-center justify-between text-sm text-ink-50">
        <span>{loading ? '加载中...' : `当前页 ${items.length} 项`}</span>
        <div className="flex items-center gap-2">
          <button className="btn-outline px-3 py-1.5 text-xs" disabled={page <= 1 || loading} onClick={() => setPage((value) => Math.max(1, value - 1))}>
            上一页
          </button>
          <span className="rounded-lg bg-gray-100 px-3 py-1 text-xs font-semibold text-ink-100">第 {page} 页</span>
          <button className="btn-outline px-3 py-1.5 text-xs" disabled={loading || items.length === 0} onClick={() => setPage((value) => value + 1)}>
            下一页
          </button>
        </div>
      </div>

      {loading ? (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8">
          {Array.from({ length: 16 }).map((_, index) => (
            <div key={index} className="aspect-[2/3] animate-pulse rounded-xl bg-gray-100" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="rounded-2xl border border-gray-200 bg-white p-10 text-center text-sand-500">没有找到可订阅条目。</div>
      ) : (
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 xl:grid-cols-8">
          {items.map((item, index) => {
            const key = itemKey(item, index)
            const checked = selectedKeys.includes(key)
            return (
              <article key={key} className="group overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm transition hover:-translate-y-0.5 hover:shadow-lg">
                <div className="relative aspect-[2/3] bg-gray-100">
                  {item.poster_url ? (
                    <img src={imageURL(item.poster_url)} alt={item.title} loading="lazy" referrerPolicy="no-referrer" className="h-full w-full object-cover" />
                  ) : (
                    <div className="flex h-full w-full items-center justify-center text-xs text-sand-500">无海报</div>
                  )}
                  <button
                    type="button"
                    className="absolute left-2 top-2 rounded-lg bg-black/65 p-1 text-white backdrop-blur"
                    onClick={() => toggleItem(key)}
                    title={checked ? '取消选择' : '选择'}
                  >
                    {checked ? <CheckSquare size={16} /> : <Square size={16} />}
                  </button>
                  <span className="absolute right-2 top-2 rounded-lg bg-black/65 px-1.5 py-0.5 text-[10px] font-bold uppercase text-white">
                    {item.source || 'tmdb'}
                  </span>
                </div>
                <div className="space-y-2 p-2.5">
                  <button
                    type="button"
                    className="block w-full truncate text-left text-sm font-semibold text-ink-600 hover:text-brand-500"
                    title={item.title}
                    onClick={() => setActiveItem(item)}
                  >
                    {item.title}
                  </button>
                  <p className="text-[11px] text-sand-500">
                    {[item.media_type, item.year && item.year > 0 ? item.year : '', item.rating ? `★ ${item.rating.toFixed(1)}` : ''].filter(Boolean).join(' · ') || '外部数据'}
                  </p>
                  <button type="button" className="btn-outline w-full justify-center px-2 py-1.5 text-xs" onClick={() => setActiveItem(item)}>
                    详情 / 订阅
                  </button>
                </div>
              </article>
            )
          })}
        </div>
      )}

      {activeItem && <DiscoverDetailModal item={activeItem} onClose={() => setActiveItem(null)} />}
    </div>
  )
}

function filterBySourceAndType(items: DiscoverItem[], source: string, mediaType: string) {
  return items.filter((item) => {
    if (source !== 'all' && (item.source || 'tmdb') !== source) return false
    if (mediaType && item.media_type !== mediaType) return false
    return true
  })
}

function dedupeItems(items: DiscoverItem[]) {
  const seen = new Set<string>()
  const out: DiscoverItem[] = []
  for (const item of items) {
    const key = itemKey(item, out.length)
    const stable = key.replace(/:\d+$/, '')
    if (seen.has(stable)) continue
    seen.add(stable)
    out.push(item)
  }
  return out
}

function itemKey(item: DiscoverItem, index: number) {
  return `${item.source || 'tmdb'}:${item.tmdb_id || item.douban_id || item.bangumi_id || item.title}:${index}`
}

function subscriptionPayload(item: DiscoverItem) {
  const keyword = item.title || item.subscribe_keyword || [item.title, item.year && item.year > 0 ? item.year : ''].filter(Boolean).join(' ')
  const source = item.source || 'tmdb'
  return {
    name: item.title,
    feed_url: `site-search://search?keyword=${encodeURIComponent(keyword)}&source=${encodeURIComponent(source)}`,
    filter: keyword,
    media_type: item.media_type || undefined,
    tmdb_id: item.tmdb_id || undefined,
    douban_id: item.douban_id || undefined,
    source,
    original_title: item.original_title || item.original_name || undefined,
    original_language: item.original_language || undefined,
    year: item.year && item.year > 0 ? item.year : undefined,
    rating: item.rating && item.rating > 0 ? item.rating : undefined,
    genres: item.genres || undefined,
    poster_url: item.poster_url || undefined,
    backdrop_url: item.backdrop_url || undefined,
    overview: item.overview || undefined,
    enabled: true,
  }
}
