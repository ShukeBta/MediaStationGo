import { useEffect, useRef, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { motion } from 'framer-motion'
import { Film, Play, Layers, Star } from 'lucide-react'
import { imageURL } from '../api/client'
import type { Media } from '../types'

export const MediaCard = ({
  media, progress, count, rating, linkTo, onClick, actions,
}: {
  media: Media
  progress?: number
  count?: number
  rating?: number
  linkTo?: string
  onClick?: () => void
  actions?: ReactNode
}) => {
  const ref = useRef<HTMLDivElement>(null)
  const href = linkTo ?? `/media/${media.id}`
  const [posterFit, setPosterFit] = useState<'cover' | 'contain'>('cover')
  const posterSrc = imageURL(media.poster_url, media.updated_at)
  const displayRating = rating ?? media.rating
  const versionCount = media.versions?.length ?? 0

  useEffect(() => {
    setPosterFit('cover')
  }, [media.poster_url, media.updated_at])

  const card = (
      <motion.div
        ref={ref}
        whileHover={{ scale: 1.04, y: -6 }}
        transition={{ type: 'spring', stiffness: 280, damping: 22 }}
        className="relative overflow-hidden rounded-2xl border border-[var(--app-border)] bg-[var(--app-panel)] shadow-[0_1px_3px_rgba(0,0,0,0.01),0_1px_2px_rgba(0,0,0,0.015)] transition-all duration-300 hover:border-brand-500/40 hover:shadow-[0_12px_32px_var(--app-shadow)]"
      >
        {/* Poster Wrapper */}
        <div className="relative aspect-[2/3] w-full overflow-hidden bg-[var(--app-panel-soft)]">
          {media.poster_url ? (
            <>
              {posterFit === 'contain' && (
                <img
                  src={posterSrc}
                  alt=""
                  aria-hidden="true"
                  loading="lazy"
                  className="absolute inset-0 h-full w-full scale-110 object-cover object-center opacity-25 blur-xl"
                  referrerPolicy="no-referrer"
                />
              )}
              <img
                src={posterSrc}
                alt={media.title}
                loading="lazy"
                decoding="async"
                onLoad={(event) => {
                  const img = event.currentTarget
                  setPosterFit(img.naturalWidth > img.naturalHeight ? 'contain' : 'cover')
                }}
                className={
                  'relative block h-full w-full object-center transition-transform duration-700 ease-out group-hover:scale-105 ' +
                  (posterFit === 'contain' ? 'object-contain p-1.5' : 'object-cover')
                }
                referrerPolicy="no-referrer"
              />
            </>
          ) : (
            <div className="flex h-full w-full flex-col items-center justify-center gap-2 bg-[var(--app-panel-soft)] text-[var(--app-muted)]">
              <Film size={28} className="stroke-[1.5]" />
              <span className="text-[10px] uppercase tracking-wider font-bold">No Poster</span>
            </div>
          )}

          {/* Episode count badge */}
          {count !== undefined && count > 1 && (
            <span className="absolute right-3 top-3 inline-flex items-center gap-1 rounded-xl border border-white/15 bg-[#111827]/90 px-2 py-1 text-[10px] font-bold text-white shadow-sm">
              <Layers size={10} className="text-[#c9954a]" />
              <span>{count} 集</span>
            </span>
          )}

          {count === undefined && versionCount > 1 && (
            <span className="absolute right-3 top-3 inline-flex items-center gap-1 rounded-xl border border-white/15 bg-[#111827]/90 px-2 py-1 text-[10px] font-bold text-white shadow-sm">
              <Layers size={10} className="text-[#c9954a]" />
              <span>{versionCount} 版本</span>
            </span>
          )}

          {/* Rating Badge */}
          {displayRating > 0 && (
            <span className="absolute left-3 top-3 inline-flex items-center gap-0.5 rounded-xl border border-white/15 bg-[#111827]/90 px-2 py-1 text-[10px] font-bold text-[#c9954a] shadow-sm">
              <Star size={10} fill="currentColor" />
              <span>{displayRating.toFixed(1)}</span>
            </span>
          )}

          {/* Premium Hover Overlay */}
          <div className="absolute inset-0 bg-gradient-to-t from-[#111827]/90 via-[#111827]/30 to-transparent opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex flex-col justify-end p-4">
            <motion.div
              initial={{ y: 15, opacity: 0 }}
              whileHover={{ y: 0, opacity: 1 }}
              transition={{ duration: 0.2 }}
              className="space-y-2"
            >
              <span className="inline-flex items-center gap-1.5 rounded-xl bg-brand-500 px-4 py-2 text-xs font-bold text-white shadow-md shadow-brand-500/20">
                <Play size={10} fill="currentColor" className="text-white" />
                <span>立即观影</span>
              </span>
              <p className="text-[10px] text-gray-200 font-semibold line-clamp-2 leading-relaxed">
                {media.overview || "暂无简介内容"}
              </p>
            </motion.div>
          </div>

          {/* Progress Bar overlay */}
          {progress !== undefined && progress > 0 && progress < 1 && (
            <div className="absolute inset-x-0 bottom-0 h-1.5 bg-[var(--app-hover)]">
              <div 
                className="h-full bg-gradient-to-r from-brand-400 to-brand-500 rounded-r-full transition-all duration-300" 
                style={{ width: `${Math.round(progress * 100)}%` }} 
              />
            </div>
          )}
        </div>

        {/* Media Metadata Info */}
        <div className="space-y-1 border-t border-[var(--app-border)] bg-[var(--app-panel)] p-4">
          <p className="truncate text-sm font-bold text-[var(--app-text)] transition-colors duration-200 group-hover:text-brand-500">
            {media.title}
          </p>
          <div className="flex items-center justify-between text-[11px] font-bold uppercase tracking-wider text-[var(--app-muted)]">
            <span>{media.year > 0 ? media.year : "未知年份"}</span>
            {media.video_codec && (
              <span className="rounded-xl border border-[var(--app-border)] bg-[var(--app-panel-soft)] px-1.5 py-0.5 text-[var(--app-subtle)]">
                {media.video_codec}
              </span>
            )}
          </div>
        </div>
      </motion.div>
  )

  if (onClick) {
    return (
      <div className="group relative block w-full">
        <button type="button" onClick={onClick} className="block w-full text-left">
          {card}
        </button>
        {actions && (
          <div className="absolute right-2 top-2 z-20 flex flex-wrap justify-end gap-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
            {actions}
          </div>
        )}
      </div>
    )
  }

  if (actions) {
    return (
      <div className="group relative block">
        <Link to={href} className="block">
          {card}
        </Link>
        <div className="absolute right-2 top-2 z-20 flex flex-wrap justify-end gap-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
          {actions}
        </div>
      </div>
    )
  }

  return (
    <Link to={href} className="group block">
        {card}
    </Link>
  )
}
