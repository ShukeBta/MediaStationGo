import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import { ArrowRight, Clock, Film, Play, Sparkles } from 'lucide-react'

import { imageURL } from '../api/client'
import { MediaCard } from '../components/MediaCard'
import type { HistoryItem } from '../api/playback'
import type { Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'
import { seriesCardLink } from '../utils/groupSeries'

export function HomeLoadingState() {
  return (
    <div className="flex items-center justify-center py-48">
      <motion.div animate={{ opacity: [0.4, 1, 0.4] }} transition={{ repeat: Infinity, duration: 1.5 }} className="flex flex-col items-center gap-4">
        <div className="relative flex items-center justify-center">
          <div className="h-10 w-10 animate-spin rounded-full border-2 border-[var(--app-border)] border-t-[var(--app-active-bg)]" />
          <Film className="absolute h-4 w-4 text-brand-500" />
        </div>
        <span className="text-sm font-semibold uppercase tracking-widest text-[var(--app-muted)]">首页内容准备中…</span>
      </motion.div>
    </div>
  )
}

export function HomeEmptyState() {
  return (
    <div className="flex flex-col items-center justify-center py-32 text-center max-w-md mx-auto">
      <div className="mb-6 flex h-24 w-24 items-center justify-center rounded-3xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] shadow-sm">
        <Film className="h-10 w-10 text-[var(--app-muted)]" />
      </div>
      <p className="text-xl font-bold text-[var(--app-text)]">您的家庭影视站暂无内容</p>
      <p className="mt-2 text-sm leading-relaxed text-[var(--app-muted)]">
        前往管理后台添加媒体目录，扫描后首页将展示本周力荐、继续观看和最近入库。
      </p>
      <Link to="/admin" className="mt-8 btn-primary">
        前往管理后台
      </Link>
    </div>
  )
}

export function HomeFeaturedSection({
  featuredItem,
  featuredVisual,
  featuredPoster,
  featuredMark,
}: {
  featuredItem: Media
  featuredVisual: string
  featuredPoster: string
  featuredMark: string
}) {
  return (
    <section className="relative overflow-hidden rounded-[2rem] border border-[var(--app-border)] bg-[var(--app-panel)] shadow-[0_24px_80px_var(--app-shadow)]">
      <div className="absolute inset-0 z-0">
        <div className="theme-hero-bg h-full w-full" />
        {featuredVisual && (
          <img
            src={imageURL(featuredVisual, featuredItem.updated_at)}
            alt=""
            className="absolute inset-0 h-full w-full scale-105 object-cover object-center opacity-[0.34] blur-[1px]"
            referrerPolicy="no-referrer"
            onError={(event) => { event.currentTarget.style.display = 'none' }}
          />
        )}
        <div className="theme-hero-overlay absolute inset-0" />
        <div className="theme-hero-fade absolute inset-x-0 bottom-0 h-32" />
      </div>

      <div className="relative z-10 grid gap-8 px-6 py-8 sm:px-8 md:grid-cols-[minmax(0,1fr)_280px] md:px-12 md:py-12 lg:grid-cols-[minmax(0,1fr)_340px] lg:px-14 lg:py-14">
        <div className="flex min-w-0 flex-col justify-center space-y-5">
          <div className="inline-flex w-fit items-center gap-2 rounded-full border border-[var(--app-brand-border)] bg-[var(--app-brand-soft)] px-3.5 py-1.5 text-xs font-bold uppercase tracking-widest text-[var(--app-brand-text)] shadow-sm backdrop-blur">
            <Sparkles size={12} fill="currentColor" />
            <span>本周力荐 / Featured</span>
          </div>

          <div className="space-y-3">
            <div className="inline-flex max-w-full items-center gap-2 rounded-2xl bg-[var(--app-active-bg)] px-3 py-2 text-[var(--app-active-text)] shadow-lg">
              <span className="h-2 w-2 rounded-full bg-[#d4af37]" />
              <span className="truncate text-xs font-black tracking-[0.26em]">{featuredMark}</span>
            </div>
            <h1 className="font-display text-3xl font-extrabold leading-tight tracking-tight text-[var(--app-text)] sm:text-4xl md:text-5xl">
              {featuredItem.title}
            </h1>
          </div>

          <p className="line-clamp-3 max-w-2xl text-sm font-semibold leading-relaxed text-[var(--app-subtle)] sm:text-base">
            {featuredItem.overview || '家庭私人媒体中心收藏。支持多端播放、外部播放器、智能刮削与订阅下载。'}
          </p>

          <div className="flex flex-wrap items-center gap-3 text-xs font-bold text-[var(--app-muted)]">
            {featuredItem.year > 0 && (
              <span className="rounded-xl border border-[var(--app-border)] bg-[var(--app-panel)] px-2.5 py-1 text-[var(--app-text)] shadow-sm">{featuredItem.year} 年</span>
            )}
            {featuredItem.video_codec && (
              <span className="rounded-lg border border-[var(--app-brand-border)] bg-[var(--app-brand-soft)] px-2 py-1 text-[10px] font-bold uppercase text-[var(--app-brand-text)]">
                {featuredItem.video_codec}
              </span>
            )}
            {featuredItem.container && (
              <span className="rounded-lg border border-[var(--app-border)] bg-[var(--app-panel)] px-2 py-1 font-mono text-[10px] uppercase text-[var(--app-subtle)]">
                {featuredItem.container}
              </span>
            )}
          </div>

          <div className="flex flex-wrap items-center gap-4 pt-2">
            <Link to={`/media/${featuredItem.id}`} className="inline-flex items-center justify-center gap-2 rounded-xl bg-[var(--app-command-bg)] px-6 py-3.5 text-sm font-bold text-[var(--app-command-text)] shadow-lg transition-all hover:-translate-y-0.5">
              <Play size={16} fill="currentColor" />
              <span>立即播放</span>
            </Link>
            <Link to="/discover" className="inline-flex items-center justify-center gap-2 rounded-xl border border-[var(--app-border)] bg-[var(--app-panel)] px-5 py-3.5 text-sm font-bold text-[var(--app-subtle)] shadow-sm transition-all hover:-translate-y-0.5 hover:border-brand-500/40 hover:text-[var(--app-text)]">
              <span>发现更多精彩</span>
              <ArrowRight size={16} />
            </Link>
          </div>
        </div>

        <div className="relative order-first mx-auto flex w-full max-w-[220px] items-center md:order-none md:max-w-[260px] lg:max-w-[310px]">
          <div className="absolute -right-6 top-5 h-32 w-32 rounded-full bg-[#d4af37]/20 blur-3xl" />
          <div className="relative aspect-[2/3] w-full overflow-hidden rounded-[1.7rem] border border-[var(--app-border)] bg-[var(--app-poster-shell)] p-2 shadow-[0_32px_80px_var(--app-shadow)]">
            <div className="flex h-full w-full flex-col items-center justify-center rounded-[1.25rem] text-center" style={{ background: 'var(--app-poster-empty)' }}>
              <Film className="mb-4 h-12 w-12 text-[#c9954a]" />
              <span className="px-6 font-display text-3xl font-black tracking-tight text-[var(--app-text)]">{featuredItem.title}</span>
            </div>
            {featuredPoster && (
              <img
                src={imageURL(featuredPoster, featuredItem.updated_at)}
                alt={featuredItem.title}
                className="absolute inset-2 h-[calc(100%-1rem)] w-[calc(100%-1rem)] rounded-[1.25rem] object-cover"
                referrerPolicy="no-referrer"
                onError={(event) => { event.currentTarget.style.display = 'none' }}
              />
            )}
          </div>
        </div>
      </div>
    </section>
  )
}

export function ContinueWatchingSection({ history }: { history: HistoryItem[] }) {
  return (
    <section className="space-y-5">
      <div className="flex items-center gap-2.5">
        <span className="rounded-xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] p-1.5 text-[var(--app-text)]">
          <Clock size={16} />
        </span>
        <h2 className="font-display text-xl font-extrabold tracking-tight text-[var(--app-text)]">继续观看</h2>
        <span className="rounded-full border border-[var(--app-border)] bg-[var(--app-panel-soft)] px-2.5 py-0.5 text-xs font-bold text-[var(--app-muted)]">{history.length} 个记录</span>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-4">
        {history.slice(0, 8).map((h) => {
          const media = h.media!
          const progress = h.duration_ms > 0 ? h.position_ms / h.duration_ms : 0
          return <ContinueCard key={h.id} media={media} progress={progress} />
        })}
      </div>
    </section>
  )
}

export function RecentMediaSection({ recentCards }: { recentCards: SeriesCard[] }) {
  return (
    <section className="space-y-5">
      <div className="flex items-center justify-between border-b border-[var(--app-border)] pb-3">
        <div className="flex items-center gap-2.5">
          <span className="rounded-xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] p-1.5 text-[var(--app-text)]">
            <Clock size={18} />
          </span>
          <div>
            <h2 className="font-display text-xl font-extrabold tracking-tight text-[var(--app-text)]">最近入库</h2>
            <p className="text-xs text-[var(--app-muted)]">按整部电影、剧集、番剧和综艺合集展示新增内容。</p>
          </div>
        </div>
        <Link to="/poster-wall" className="group inline-flex items-center gap-1 text-xs font-bold text-[var(--app-subtle)] transition-colors hover:text-brand-500">
          <span>海报墙</span>
          <ArrowRight size={14} className="transition-transform group-hover:translate-x-0.5" />
        </Link>
      </div>

      <div className="grid grid-cols-2 gap-5 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 2xl:grid-cols-7">
        {recentCards.map((card) => (
          <MediaCard
            key={card.key}
            media={card.rep}
            count={card.count}
            linkTo={seriesCardLink(card)}
          />
        ))}
      </div>
    </section>
  )
}

function ContinueCard({ media, progress }: { media: Media; progress: number }) {
  return (
    <Link to={`/media/${media.id}`} className="group flex items-center gap-4 rounded-2xl border border-[var(--app-border)] bg-[var(--app-panel)] p-3.5 shadow-[0_1px_3px_rgba(0,0,0,0.01)] transition-all duration-300 hover:border-brand-500/30 hover:bg-[var(--app-panel-soft)] hover:shadow-md">
      <div className="relative h-18 w-12 shrink-0 overflow-hidden rounded-xl border border-[var(--app-border)] bg-[var(--app-panel-soft)]">
        {media.poster_url ? (
          <img
            src={imageURL(media.poster_url, media.updated_at)}
            alt=""
            loading="lazy"
            className="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
            referrerPolicy="no-referrer"
          />
        ) : (
          <div className="flex h-full items-center justify-center bg-[var(--app-panel-soft)] text-[var(--app-muted)]">
            <Film size={16} />
          </div>
        )}
        <div className="absolute inset-0 flex items-center justify-center bg-black/30 opacity-0 transition-opacity group-hover:opacity-100">
          <Play size={14} fill="white" className="text-white" />
        </div>
      </div>
      <div className="min-w-0 flex-1 space-y-1.5">
        <p className="truncate text-sm font-bold text-[var(--app-text)] transition-colors group-hover:text-brand-500">
          {media.title}
        </p>
        <div className="space-y-1">
          <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--app-hover)]">
            <motion.div
              initial={{ width: 0 }}
              animate={{ width: `${Math.round(progress * 100)}%` }}
              transition={{ duration: 0.5, delay: 0.1 }}
              className="h-full rounded-full bg-gradient-to-r from-brand-400 to-brand-500"
            />
          </div>
          <p className="text-[10px] font-bold uppercase tracking-wide text-[var(--app-muted)]">
            已观看到 {Math.round(progress * 100)}%
          </p>
        </div>
      </div>
    </Link>
  )
}
