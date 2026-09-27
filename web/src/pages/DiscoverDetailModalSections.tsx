import { useEffect, useMemo, useRef, useState } from 'react'
import {
  Building2,
  CalendarDays,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Database,
  ExternalLink,
  Globe2,
  Hash,
  Languages,
  LoaderCircle,
  Star,
  Tags,
  UserRound,
  X,
  ZoomIn,
} from 'lucide-react'

import type { DiscoverItem } from '../api/discover'
import { imageURL } from '../api/client'
import { discoverSourceLabel } from './discoverPageModel'
import {
  discoverItemMetaText,
  discoverItemPeople,
  discoverItemValues,
  discoverPerformerItem,
  discoverReleaseStatus,
} from './discoverDetailModalModel'

export function DiscoverModalHeader({
  item,
  source,
  onClose,
  setTitleActionRoot,
}: {
  item: DiscoverItem
  source: string
  onClose: () => void
  setTitleActionRoot?: (element: HTMLDivElement | null) => void
}) {
  return (
    <div className="mb-4 flex items-start justify-between gap-3">
      <div className="min-w-0">
        <p className="text-xs font-semibold uppercase tracking-widest text-brand-500">{source}</p>
        <div className="flex flex-wrap items-center gap-2.5">
          <h2 className="font-display text-2xl font-bold text-ink-600">{item.title}</h2>
          {['tv', 'anime', 'variety'].includes((item.media_type || '').toLowerCase()) && (
            <div ref={setTitleActionRoot} className="shrink-0" />
          )}
        </div>
        <p className="mt-1 text-sm text-sand-500">{discoverItemMetaText(item)}</p>
      </div>
      <button
        type="button"
        className="shrink-0 rounded-full border border-gray-200 p-2 text-ink-50 hover:bg-gray-50"
        aria-label="关闭作品详情"
        title="关闭"
        onClick={onClose}
      >
        <X size={18} />
      </button>
    </div>
  )
}

export function DiscoverArtworkPanel({
  item,
  deferred = false,
  onPreview,
}: {
  item: DiscoverItem
  deferred?: boolean
  onPreview?: () => void
}) {
  const adultArtwork = item.media_type === 'adult'
  if (adultArtwork && deferred) {
    return (
      <div className="aspect-[3/2] overflow-hidden rounded-2xl bg-gray-950">
        <div className="flex h-full items-center justify-center gap-2 text-sm font-medium text-gray-300">
          <LoaderCircle size={18} className="animate-spin" />
          正在加载完整封面…
        </div>
      </div>
    )
  }
  return (
    <div className="space-y-3">
      <div className={`overflow-hidden rounded-2xl ${adultArtwork ? 'bg-gray-950' : 'bg-gray-100'}`}>
        {item.poster_url ? (
          <button
            type="button"
            className="group relative block w-full cursor-zoom-in overflow-hidden text-left"
            aria-label={`查看 ${item.title} 大图`}
            onClick={onPreview}
          >
            <img
              src={imageURL(item.poster_url, undefined, {
                maxWidth: adultArtwork ? 1280 : 640,
                quality: 88,
              })}
              alt={item.title}
              decoding="async"
              className={`${adultArtwork ? 'aspect-[3/2] object-contain' : 'aspect-[2/3] object-cover'} w-full transition duration-200 group-hover:scale-[1.01]`}
            />
            <span className="absolute bottom-3 right-3 inline-flex items-center gap-1.5 rounded-full bg-black/65 px-2.5 py-1.5 text-xs font-medium text-white opacity-90 shadow-lg backdrop-blur-sm transition group-hover:bg-black/80">
              <ZoomIn size={14} />
              查看大图
            </span>
          </button>
        ) : (
          <div className={`flex items-center justify-center text-sand-500 ${adultArtwork ? 'aspect-[3/2]' : 'aspect-[2/3]'}`}>无海报</div>
        )}
      </div>
    </div>
  )
}

export function DiscoverBackdropPanel({ item }: { item: DiscoverItem }) {
  if (!item.backdrop_url?.trim()) return null
  return (
    <div className="relative aspect-[16/7] overflow-hidden rounded-2xl bg-gray-100">
      <img
        src={imageURL(item.backdrop_url, undefined, { maxWidth: 1280, quality: 86 })}
        alt=""
        loading="lazy"
        decoding="async"
        className="h-full w-full object-cover"
      />
      <div className="absolute inset-0 bg-gradient-to-t from-black/30 via-transparent to-transparent" />
    </div>
  )
}

export function DiscoverPreviewGallery({
  images,
  title,
  onPreview,
}: {
  images?: string[]
  title: string
  onPreview: (url: string) => void
}) {
  const previewImages = useMemo(
    () => (images ?? []).map((url) => url.trim()).filter((url, index, values) => Boolean(url) && values.indexOf(url) === index),
    [images],
  )
  const scrollerRef = useRef<HTMLDivElement>(null)
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)

  useEffect(() => {
    const scroller = scrollerRef.current
    if (!scroller || previewImages.length === 0) return
    const updateScrollState = () => {
      setCanScrollLeft(scroller.scrollLeft > 2)
      setCanScrollRight(scroller.scrollLeft + scroller.clientWidth < scroller.scrollWidth - 2)
    }
    const frame = window.requestAnimationFrame(updateScrollState)
    scroller.addEventListener('scroll', updateScrollState, { passive: true })
    window.addEventListener('resize', updateScrollState)
    return () => {
      window.cancelAnimationFrame(frame)
      scroller.removeEventListener('scroll', updateScrollState)
      window.removeEventListener('resize', updateScrollState)
    }
  }, [previewImages])

  if (previewImages.length === 0) return null

  const scroll = (direction: -1 | 1) => {
    const scroller = scrollerRef.current
    if (!scroller) return
    scroller.scrollBy({ left: direction * Math.max(320, scroller.clientWidth * 0.82), behavior: 'smooth' })
  }

  return (
    <section className="mt-5 space-y-3 border-t border-gray-200 pt-5">
      <div className="flex items-center justify-between gap-3">
        <div>
          <h3 className="font-semibold text-ink-600">作品详情预览</h3>
          <p className="mt-0.5 text-xs text-sand-500">{previewImages.length} 张大图，点击可查看原图</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            type="button"
            className="inline-flex h-9 w-9 items-center justify-center rounded-full border border-gray-200 bg-white text-ink-100 transition hover:border-primary-300 hover:text-brand-500 disabled:cursor-not-allowed disabled:opacity-35"
            aria-label="向左浏览作品详情图"
            disabled={!canScrollLeft}
            onClick={() => scroll(-1)}
          >
            <ChevronLeft size={18} />
          </button>
          <button
            type="button"
            className="inline-flex h-9 w-9 items-center justify-center rounded-full border border-gray-200 bg-white text-ink-100 transition hover:border-primary-300 hover:text-brand-500 disabled:cursor-not-allowed disabled:opacity-35"
            aria-label="向右浏览作品详情图"
            disabled={!canScrollRight}
            onClick={() => scroll(1)}
          >
            <ChevronRight size={18} />
          </button>
        </div>
      </div>
      <div
        ref={scrollerRef}
        className="flex snap-x snap-mandatory gap-3 overflow-x-auto scroll-smooth pb-2 [scrollbar-width:thin]"
      >
        {previewImages.map((url, index) => (
          <button
            key={url}
            type="button"
            className="group relative aspect-video min-w-[82%] snap-start overflow-hidden rounded-2xl border border-gray-200 bg-gray-950 text-left sm:min-w-[420px] lg:min-w-[480px]"
            aria-label={`查看 ${title} 详情大图 ${index + 1}`}
            onClick={() => onPreview(url)}
          >
            <img
              src={imageURL(url, undefined, { maxWidth: 960, quality: 88 })}
              alt={`${title} 详情图 ${index + 1}`}
              loading="lazy"
              decoding="async"
              className="h-full w-full object-contain transition duration-200 group-hover:scale-[1.01]"
            />
            <span className="absolute bottom-2 right-2 rounded-full bg-black/65 px-2 py-1 text-[10px] font-semibold text-white backdrop-blur-sm">
              {index + 1} / {previewImages.length}
            </span>
          </button>
        ))}
      </div>
    </section>
  )
}

export function DiscoverMetadataPanel({
  item,
  loading,
  error,
  onSelectPerformer,
}: {
  item: DiscoverItem
  loading: boolean
  error: string
  onSelectPerformer?: (item: DiscoverItem) => void
}) {
  const people = discoverItemPeople(item)
  const adultItem = item.media_type === 'adult'
  const peopleLabel = adultItem ? '女优' : '演员'
  const genres = discoverItemValues(item.genres).filter((value) => !['adult', item.source?.toLowerCase()].includes(value.toLowerCase()))
  const countries = discoverItemValues(item.countries)
  const languages = discoverItemValues(item.languages)
  const directors = discoverItemValues(item.directors)
  const writers = discoverItemValues(item.writers)
  const aliases = discoverItemValues(item.aliases)
  const releaseStatus = discoverReleaseStatus(item.release_date)
  const catalogID = item.tmdb_id
    ? `TMDb ${item.tmdb_id}`
    : item.douban_id
      ? `豆瓣 ${item.douban_id}`
      : item.bangumi_id
        ? `Bangumi ${item.bangumi_id}`
        : ''
  const releaseLabel = adultItem ? '发行日期' : item.media_type === 'tv' || item.media_type === 'anime' ? '首播日期' : '上映日期'
  const sourceLabel = discoverSourceLabel(item.source)

  return (
    <section className="space-y-4 rounded-2xl border border-gray-200 bg-white p-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold text-ink-600">作品资料</h3>
        {loading && (
          <span className="inline-flex items-center gap-1.5 text-xs text-sand-500">
            <LoaderCircle size={14} className="animate-spin" />
            {adultItem ? `正在补充 ${sourceLabel} 详情…` : '正在补充作品资料…'}
          </span>
        )}
      </div>

      <div className="grid gap-x-6 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
        {item.original_name?.trim() && (
          <MetadataValue icon={<Hash size={15} />} label={adultItem ? '番号' : '原名'} value={item.original_name.trim()} mono={adultItem} />
        )}
        {item.release_date?.trim() && (
          <MetadataValue
            icon={<CalendarDays size={15} />}
            label={releaseLabel}
            value={item.release_date.trim()}
            suffix={releaseStatus === 'upcoming' ? '未发行' : undefined}
          />
        )}
        {!item.release_date?.trim() && item.year && item.year > 0 && (
          <MetadataValue icon={<CalendarDays size={15} />} label="年份" value={String(item.year)} />
        )}
        {item.duration_minutes && item.duration_minutes > 0 && (
          <MetadataValue icon={<Clock3 size={15} />} label="时长" value={`${item.duration_minutes} 分钟`} />
        )}
        {item.maker?.trim() && (
          <MetadataValue icon={<Building2 size={15} />} label={adultItem ? '片商' : '出品方'} value={item.maker.trim()} />
        )}
        {item.rating && item.rating > 0 && (
          <MetadataValue icon={<Star size={15} />} label="评分" value={item.rating.toFixed(1)} />
        )}
        {catalogID && (
          <MetadataValue icon={<Database size={15} />} label="资料编号" value={catalogID} mono />
        )}
        {countries.length > 0 && (
          <MetadataValue icon={<Globe2 size={15} />} label="国家 / 地区" value={countries.join(' / ')} />
        )}
        {languages.length > 0 && (
          <MetadataValue icon={<Languages size={15} />} label="语言" value={languages.join(' / ')} />
        )}
      </div>

      {directors.length > 0 && <MetadataList label="导演" values={directors} />}

      {writers.length > 0 && <MetadataList label="编剧" values={writers} />}

      {aliases.length > 0 && <MetadataList label="又名" values={aliases} />}

      {people.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center gap-1.5 text-xs font-semibold text-sand-500">
            <UserRound size={14} />
            {peopleLabel}
          </div>
          <div className="flex flex-wrap gap-2">
            {people.map((person) => {
              const selectable = Boolean(adultItem && onSelectPerformer && person.source_id?.trim())
              const className = adultItem
                ? 'inline-flex min-h-8 items-center gap-1.5 rounded-md border border-rose-100 bg-rose-50 px-2.5 py-1 text-xs font-medium text-rose-700'
                : 'inline-flex min-h-8 items-center gap-1.5 rounded-md border border-primary-100 bg-primary-50 px-2.5 py-1 text-xs font-medium text-brand-600'
              return selectable ? (
                <button
                  key={`${person.source || 'person'}-${person.source_id || person.name}`}
                  type="button"
                  className={`${className} hover:border-rose-200 hover:bg-rose-100`}
                  onClick={() => onSelectPerformer?.(discoverPerformerItem(person))}
                >
                  <UserRound size={13} />
                  {person.name}
                </button>
              ) : (
                <span key={person.name} className={className}>
                  <UserRound size={13} />
                  {person.name}
                </span>
              )
            })}
          </div>
        </div>
      )}

      {genres.length > 0 && (
        <div className="space-y-2">
          <div className="flex items-center gap-1.5 text-xs font-semibold text-sand-500">
            <Tags size={14} />
            类别
          </div>
          <div className="flex flex-wrap gap-2">
            {genres.map((genre) => (
              <span key={genre} className="rounded-md bg-gray-100 px-2.5 py-1 text-xs text-ink-100">
                {genre}
              </span>
            ))}
          </div>
        </div>
      )}

      {error && (
        <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
          {error}，当前仅展示列表已有资料。
        </p>
      )}

      {item.provider_url?.trim() && (
        <a
          href={item.provider_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1.5 text-xs font-medium text-brand-600 hover:text-brand-700"
        >
          <ExternalLink size={14} />
          查看原始资料
        </a>
      )}
    </section>
  )
}

function MetadataList({ label, values }: { label: string; values: string[] }) {
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-1.5 text-xs font-semibold text-sand-500">
        <UserRound size={14} />
        {label}
      </div>
      <p className="text-sm leading-6 text-ink-100">{values.join(' / ')}</p>
    </div>
  )
}

function MetadataValue({
  icon,
  label,
  value,
  suffix,
  mono = false,
}: {
  icon: React.ReactNode
  label: string
  value: string
  suffix?: string
  mono?: boolean
}) {
  return (
    <div className="flex min-w-0 items-start gap-2">
      <span className="mt-0.5 shrink-0 text-brand-500">{icon}</span>
      <div className="min-w-0">
        <p className="text-xs text-sand-500">{label}</p>
        <div className="flex flex-wrap items-center gap-1.5">
          <span className={`break-words text-sm font-medium text-ink-600 ${mono ? 'font-mono' : ''}`}>{value}</span>
          {suffix && <span className="rounded bg-amber-100 px-1.5 py-0.5 text-2xs font-semibold text-amber-800">{suffix}</span>}
        </div>
      </div>
    </div>
  )
}

export function DiscoverOverviewPanel({ overview }: { overview?: string }) {
  if (!overview?.trim()) return null
  return (
    <section className="rounded-2xl border border-gray-200 bg-gray-50 p-4">
      <h3 className="mb-2 font-semibold text-ink-600">简介</h3>
      <p className="text-sm leading-6 text-ink-100">{overview}</p>
    </section>
  )
}
