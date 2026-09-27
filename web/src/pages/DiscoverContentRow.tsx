import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { CheckCircle2, ChevronLeft, ChevronRight, Heart, ImageOff, Info, RefreshCw } from 'lucide-react'

import type { DiscoverItem } from '../api/discover'
import { imageURL } from '../api/client'
import { discoverCardMetaText, discoverCardSecondaryText, discoverItemSource, discoverSourceLabel } from './discoverPageModel'

const discoverRowPreloadMargin = '0px'
const discoverCardPreloadMargin = '0px'
const discoverPriorityPosterCount = 3

export const discoverModulePageSize = 18
export type DiscoverRefreshStatus = 'loading' | 'success' | 'error'

const discoverCardVisibilityCallbacks = new Map<Element, () => void>()
let discoverCardVisibilityObserver: IntersectionObserver | null = null

function observeDiscoverCard(element: Element, onVisible: () => void): () => void {
  if (typeof window.IntersectionObserver === 'undefined') {
    onVisible()
    return () => undefined
  }
  if (!discoverCardVisibilityObserver) {
    const observer = new window.IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          const callback = discoverCardVisibilityCallbacks.get(entry.target)
          if (!callback) continue
          discoverCardVisibilityCallbacks.delete(entry.target)
          observer.unobserve(entry.target)
          callback()
        }
        if (discoverCardVisibilityCallbacks.size === 0) {
          observer.disconnect()
          if (discoverCardVisibilityObserver === observer) {
            discoverCardVisibilityObserver = null
          }
        }
      },
      { rootMargin: discoverCardPreloadMargin },
    )
    discoverCardVisibilityObserver = observer
  }
  discoverCardVisibilityCallbacks.set(element, onVisible)
  discoverCardVisibilityObserver.observe(element)
  return () => {
    discoverCardVisibilityCallbacks.delete(element)
    discoverCardVisibilityObserver?.unobserve(element)
    if (discoverCardVisibilityCallbacks.size === 0) {
      discoverCardVisibilityObserver?.disconnect()
      discoverCardVisibilityObserver = null
    }
  }
}

export function ContentRow({
  title,
  items,
  page = 1,
  canNext = false,
  imageVersion,
  refreshImageVersion,
  refreshing = false,
  refreshStatus,
  fixedGrid = false,
  priority = false,
  cardSize = 'default',
  headerControl,
  onPageChange,
  onRefresh,
  onSelect,
}: {
  title: string
  items: DiscoverItem[]
  page?: number
  canNext?: boolean
  imageVersion?: string
  refreshImageVersion?: string
  refreshing?: boolean
  refreshStatus?: DiscoverRefreshStatus
  fixedGrid?: boolean
  priority?: boolean
  cardSize?: 'default' | 'large'
  headerControl?: ReactNode
  onPageChange?: (delta: number) => void
  onRefresh?: () => void
  onSelect: (item: DiscoverItem) => void
}) {
  const rowRef = useRef<HTMLElement>(null)
  const [imagesEnabled, setImagesEnabled] = useState(priority)
  const shouldRenderImages = priority || imagesEnabled
  const gridClassName = fixedGrid
    ? 'grid grid-cols-2 gap-5 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6'
    : cardSize === 'large'
      ? 'grid grid-cols-2 gap-5 sm:grid-cols-3 md:grid-cols-[repeat(auto-fill,minmax(210px,1fr))]'
      : 'grid grid-cols-2 gap-5 sm:grid-cols-3 md:grid-cols-[repeat(auto-fill,minmax(190px,1fr))]'

  useEffect(() => {
    if (shouldRenderImages) return
    const row = rowRef.current
    if (!row || typeof window.IntersectionObserver === 'undefined') {
      setImagesEnabled(true)
      return
    }
    const observer = new window.IntersectionObserver(
      (entries) => {
        if (!entries.some((entry) => entry.isIntersecting)) return
        setImagesEnabled(true)
        observer.disconnect()
      },
      { rootMargin: discoverRowPreloadMargin },
    )
    observer.observe(row)
    return () => observer.disconnect()
  }, [shouldRenderImages])

  return (
    <section ref={rowRef} className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h2 className="pl-1 font-display text-2xl font-semibold text-ink-600">{title}</h2>
        {(headerControl || onRefresh || onPageChange) && (
          <div className="flex items-center gap-2">
            {headerControl}
            {onRefresh && (
              <DiscoverRefreshControl
                title={title}
                refreshing={refreshing}
                status={refreshStatus}
                onRefresh={onRefresh}
              />
            )}
            {onPageChange && (
              <>
                <button
                  type="button"
                  aria-label={`${title} 上一页`}
                  disabled={refreshing || page <= 1}
                  onClick={() => onPageChange(-1)}
                  className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-gray-200 bg-white text-ink-100 transition hover:border-primary-300 hover:text-brand-500 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronLeft size={16} />
                </button>
                <span className="min-w-10 text-center text-xs font-semibold text-sand-500">第 {page} 页</span>
                <button
                  type="button"
                  aria-label={`${title} 下一页`}
                  disabled={refreshing || !canNext}
                  onClick={() => onPageChange(1)}
                  className="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-gray-200 bg-white text-ink-100 transition hover:border-primary-300 hover:text-brand-500 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  <ChevronRight size={16} />
                </button>
              </>
            )}
          </div>
        )}
      </div>
      <div className={gridClassName}>
        {shouldRenderImages
          ? items.map((item, index) => (
              <DiscoverCard
                key={discoverKey(item, index)}
                item={item}
                imageVersion={imageVersion}
                refreshImageVersion={refreshImageVersion}
                imagePriority={priority && index < discoverPriorityPosterCount}
                onSelect={onSelect}
              />
            ))
          : items.map((item, index) => (
              <DiscoverCardPlaceholder
                key={discoverKey(item, index)}
                person={item.media_type === 'person'}
              />
            ))}
      </div>
    </section>
  )
}

export function DiscoverCardPlaceholder({
  person = false,
  pulse = false,
}: {
  person?: boolean
  pulse?: boolean
}) {
  return (
    <div
      aria-hidden="true"
      className={`overflow-hidden rounded-xl border border-transparent bg-gray-100/70 xl:aspect-[2/3] ${pulse ? 'animate-pulse' : ''}`}
    >
      <div className={`${person ? 'aspect-square' : 'aspect-[2/3]'} bg-gray-100 xl:hidden`} />
      <div className={`${person ? 'h-[72px]' : 'h-[90px]'} bg-gray-50/80 xl:hidden`} />
    </div>
  )
}

export function DiscoverRefreshControl({
  title,
  refreshing,
  status,
  onRefresh,
}: {
  title: string
  refreshing: boolean
  status?: DiscoverRefreshStatus
  onRefresh: () => void
}) {
  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        aria-label={discoverRefreshButtonLabel(title, status)}
        title={discoverRefreshButtonTitle(status)}
        disabled={refreshing || status === 'loading'}
        onClick={onRefresh}
        className={`inline-flex h-8 w-8 items-center justify-center rounded-lg border bg-white transition hover:border-primary-300 hover:text-brand-500 disabled:cursor-not-allowed disabled:opacity-40 ${
          status === 'success'
            ? 'border-emerald-300 text-emerald-600'
            : status === 'error'
              ? 'border-red-300 text-red-500'
              : 'border-gray-200 text-ink-100'
        }`}
      >
        <RefreshCw size={15} className={refreshing || status === 'loading' ? 'animate-spin' : ''} />
      </button>
      {status && (
        <span aria-live="polite" className={`text-xs font-semibold ${discoverRefreshStatusClassName(status)}`}>
          {discoverRefreshStatusLabel(status)}
        </span>
      )}
    </div>
  )
}

export function DiscoverSkeleton() {
  return (
    <div className="space-y-8">
      {[1, 2, 3].map((section) => (
        <section key={section} className="space-y-4">
          <div className="h-8 w-48 animate-pulse rounded-xl bg-gray-100" />
          <div className="grid grid-cols-2 gap-5 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6">
            {Array.from({ length: discoverModulePageSize }, (_, index) => index).map((item) => (
              <div key={item} className="aspect-[2/3] animate-pulse rounded-xl bg-gray-100" />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

function DiscoverCard({
  item,
  imageVersion,
  refreshImageVersion,
  imagePriority,
  onSelect,
}: {
  item: DiscoverItem
  imageVersion?: string
  refreshImageVersion?: string
  imagePriority: boolean
  onSelect: (item: DiscoverItem) => void
}) {
  const cardRef = useRef<HTMLButtonElement>(null)
  const source = discoverItemSource(item)
	const isPerson = item.media_type === 'person'
	const isJavDBAdult = item.media_type === 'adult' && source.toLowerCase() === 'javdb'
	const isFD2Adult = item.media_type === 'adult' && source.toLowerCase() === 'fd2ppv'
  const secondaryText = discoverCardSecondaryText(item)
  const imageCandidates = useMemo(
    () =>
      [item.poster_url, item.backdrop_url]
        .map((value) => value?.trim())
        .filter((value, index, values): value is string => Boolean(value) && values.indexOf(value) === index),
    [item.poster_url, item.backdrop_url],
  )
  const [imageIndex, setImageIndex] = useState(0)
  const [posterRetry, setPosterRetry] = useState(0)
  const [posterUnavailable, setPosterUnavailable] = useState(false)
  const [imageEnabled, setImageEnabled] = useState(imagePriority)
  const shouldLoadImage = imagePriority || imageEnabled
  const posterVersion = [imageVersion, posterRetry > 0 ? `r${posterRetry}` : ''].filter(Boolean).join('-')
  const activeImage = imageCandidates[imageIndex] ?? ''
  const shouldRefreshCache = Boolean(
    (imageVersion && refreshImageVersion === imageVersion) || posterRetry > 0,
  )
  const posterSrc = useMemo(
    () =>
      imageURL(activeImage, posterVersion, {
        refreshCache: shouldRefreshCache,
        maxWidth: isJavDBAdult || isFD2Adult ? 800 : isPerson ? 320 : 420,
        quality: isJavDBAdult || isFD2Adult ? 88 : 84,
      }),
    [activeImage, isFD2Adult, isJavDBAdult, isPerson, posterVersion, shouldRefreshCache],
  )

  useEffect(() => {
    if (shouldLoadImage) return
    const card = cardRef.current
    if (!card) return
    return observeDiscoverCard(card, () => setImageEnabled(true))
  }, [shouldLoadImage])

  useEffect(() => {
    setImageIndex(0)
    setPosterRetry(0)
    setPosterUnavailable(false)
    setImageEnabled(imagePriority)
  }, [item.poster_url, item.backdrop_url, imagePriority, imageVersion])

  useEffect(() => {
    if (!posterUnavailable) return
    if (imageIndex + 1 < imageCandidates.length) {
      const timer = window.setTimeout(() => {
        setImageIndex((current) => Math.min(current + 1, imageCandidates.length - 1))
        setPosterRetry(0)
        setPosterUnavailable(false)
      }, 150)
      return () => window.clearTimeout(timer)
    }
    if (posterRetry >= 3) return
    const timer = window.setTimeout(() => {
      setPosterRetry((current) => current + 1)
      setPosterUnavailable(false)
    }, 1200 * (posterRetry + 1))
    return () => window.clearTimeout(timer)
  }, [imageCandidates.length, imageIndex, posterRetry, posterUnavailable])

  const markPosterUnavailable = () => setPosterUnavailable(true)

  return (
    <button
      ref={cardRef}
      type="button"
      onClick={() => onSelect(item)}
      className="group relative overflow-hidden rounded-xl border border-gray-200 bg-gray-50 text-left transition-all duration-300 hover:-translate-y-1 hover:border-primary-500/30 hover:shadow-xl focus:outline-none focus:ring-2 focus:ring-primary-400/40"
    >
      <div
		className={isPerson
			? 'relative flex aspect-square w-full items-center justify-center overflow-hidden bg-gradient-to-br from-rose-50 via-white to-primary-50'
			: 'relative aspect-[2/3] w-full overflow-hidden bg-surface-900'}
	  >
        {shouldLoadImage && posterSrc && !posterUnavailable ? (
          <img
            src={posterSrc}
            alt={item.title}
            width={360}
            height={540}
            loading={imagePriority ? 'eager' : 'lazy'}
            fetchPriority={imagePriority ? 'high' : 'low'}
            decoding="async"
            referrerPolicy="no-referrer"
            onError={markPosterUnavailable}
            onLoad={(event) => {
              const img = event.currentTarget
              if (img.naturalWidth <= 1 && img.naturalHeight <= 1) {
                markPosterUnavailable()
              }
            }}
			className={isPerson
				? 'h-28 w-28 rounded-full object-cover shadow-lg ring-4 ring-white transition-transform duration-500 group-hover:scale-105'
				: `h-full w-full transition-transform duration-500 group-hover:scale-105 ${isFD2Adult ? 'object-contain' : `object-cover ${isJavDBAdult ? 'object-right' : ''}`}`}
          />
        ) : isPerson ? (
			<div className="flex h-28 w-28 items-center justify-center rounded-full bg-rose-100 text-sm font-semibold text-rose-500 ring-4 ring-white">
            女优
          </div>
        ) : shouldLoadImage ? (
          <div className="flex h-full w-full flex-col items-center justify-center gap-2 bg-gradient-to-br from-surface-900 to-surface-800 px-4 text-center text-sand-400">
            <ImageOff size={26} aria-hidden="true" />
            <span className="text-xs font-semibold">封面暂不可用</span>
          </div>
        ) : null}
        <div className="absolute left-1.5 top-1.5 rounded-xl border border-white/20 bg-black/65 px-1.5 py-0.5 text-[10px] font-semibold uppercase text-white backdrop-blur-sm">
          {discoverSourceLabel(source)}
        </div>
        {(item.rating ?? 0) > 0 && (
          <div className="absolute right-1.5 top-1.5 rounded-xl border border-yellow-400/30 bg-black/70 px-1.5 py-0.5 text-[11px] font-semibold text-yellow-400 backdrop-blur-sm">
            ★ {(item.rating ?? 0).toFixed(1)}
          </div>
        )}
        {item.in_library && item.media_id && (
          <div className="absolute bottom-1.5 left-1.5 inline-flex items-center gap-1 rounded-lg border border-emerald-300/40 bg-emerald-600/90 px-1.5 py-0.5 text-[10px] font-semibold text-white shadow-sm">
            <CheckCircle2 size={10} />
            已入库
          </div>
        )}
		{isPerson && item.followed && (
			<div className="absolute bottom-1.5 left-1.5 inline-flex items-center gap-1 rounded-lg border border-rose-300/50 bg-rose-600/90 px-1.5 py-0.5 text-[10px] font-semibold text-white shadow-sm">
				<Heart size={10} fill="currentColor" />
				已关注
			</div>
		)}
      </div>
      <div className="space-y-0.5 px-2.5 py-2">
        <p className="truncate text-xs font-medium text-ink-600 transition-colors group-hover:text-brand-500">
          {item.title}
        </p>
		<p className={isPerson ? 'hidden' : 'text-[11px] text-sand-500'}>
          {discoverCardMetaText(item)}
        </p>
		{!isPerson && (
			<p className="h-4 truncate text-[10px] text-sand-400" title={secondaryText || undefined}>
				{secondaryText || '\u00a0'}
			</p>
		)}
		{isPerson && <p className="text-[11px] text-sand-500">女优</p>}
		<p className={isPerson ? 'hidden' : 'flex items-center gap-1 pt-1 text-[10px] font-semibold text-brand-500'}>
          <Info size={10} />
          {item.in_library && item.media_id ? '查看库内作品' : '详情 / 订阅'}
		</p>
		{isPerson && (
			<p className="flex items-center gap-1 pt-1 text-[10px] font-semibold text-rose-600">
				<Heart size={10} />
				查看作品 / 关注
			</p>
		)}
      </div>
    </button>
  )
}

function discoverKey(item: DiscoverItem, index: number): string {
	return `${item.source || 'source'}:${item.provider_id || item.tmdb_id || item.douban_id || item.bangumi_id || item.title}:${index}`
}

function discoverRefreshButtonLabel(title: string, status?: DiscoverRefreshStatus): string {
  if (status === 'loading') return `${title} 正在刷新`
  if (status === 'success') return `${title} 已刷新`
  if (status === 'error') return `${title} 刷新失败`
  return `${title} 刷新`
}

function discoverRefreshButtonTitle(status?: DiscoverRefreshStatus): string {
  if (status === 'loading') return '正在刷新当前模块'
  if (status === 'success') return '当前模块已刷新，点击可再次刷新'
  if (status === 'error') return '当前模块刷新失败，点击重试'
  return '只刷新当前模块'
}

function discoverRefreshStatusLabel(status: DiscoverRefreshStatus): string {
  if (status === 'loading') return '正在刷新…'
  if (status === 'success') return '已刷新'
  return '刷新失败'
}

function discoverRefreshStatusClassName(status: DiscoverRefreshStatus): string {
  if (status === 'loading') return 'text-brand-500'
  if (status === 'success') return 'text-emerald-600'
  return 'text-red-500'
}
