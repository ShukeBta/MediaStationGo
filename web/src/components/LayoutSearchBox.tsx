import { useEffect, useRef, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { AnimatePresence, motion } from 'framer-motion'
import { Library as LibraryIcon, Search, X } from 'lucide-react'
import clsx from 'clsx'

import { imageURL } from '../api/client'
import { seriesCardLink, type SeriesCard } from '../utils/groupSeries'
import { mediaPosterURL } from '../utils/mediaArtwork'

type LayoutSearchBoxProps = {
  query: string
  focused: boolean
  loading: boolean
  error: string
  cards: SeriesCard[]
  total: number
  onQueryChange: (value: string) => void
  onClear: () => void
  onFocusedChange: (focused: boolean) => void
  onSubmit: (event: FormEvent) => void
}

export function LayoutSearchBox({
  query,
  focused,
  loading,
  error,
  cards,
  total,
  onQueryChange,
  onClear,
  onFocusedChange,
  onSubmit,
}: LayoutSearchBoxProps) {
  const trimmedQuery = query.trim()
  const rootRef = useRef<HTMLFormElement>(null)

  useEffect(() => {
    if (!focused) return

    const handleOutsidePointer = (event: PointerEvent) => {
      const target = event.target
      if (target instanceof Node && !rootRef.current?.contains(target)) {
        onFocusedChange(false)
      }
    }

    document.addEventListener('pointerdown', handleOutsidePointer, true)
    return () => document.removeEventListener('pointerdown', handleOutsidePointer, true)
  }, [focused, onFocusedChange])

  return (
    <form ref={rootRef} onSubmit={onSubmit} className="relative hidden w-full sm:block">
      <span className={clsx(
        'absolute left-4 top-1/2 -translate-y-1/2 transition-colors duration-200',
        focused ? 'text-brand-500' : 'text-[var(--app-muted)]',
      )}>
        <Search size={16} />
      </span>
      <input
        type="text"
        value={query}
        onChange={(event) => onQueryChange(event.target.value)}
        onMouseDown={() => onFocusedChange(true)}
        onClick={() => onFocusedChange(true)}
        onFocus={() => onFocusedChange(true)}
        onBlur={() => window.setTimeout(() => onFocusedChange(false), 120)}
        placeholder="搜索片名、原名、演员或类型..."
        className="w-full rounded-full border border-[var(--app-border)] bg-[var(--app-control-bg)] py-2.5 pl-11 pr-12 text-sm text-[var(--app-text)] placeholder:text-[var(--app-muted)] outline-none transition-all duration-300 focus:border-brand-500 focus:bg-[var(--app-panel)] focus:ring-4 focus:ring-brand-100/40"
      />
      {query.length > 0 ? (
        <button
          type="button"
          onMouseDown={(event) => event.preventDefault()}
          onClick={onClear}
          title="清空搜索"
          aria-label="清空搜索"
          className="absolute right-3 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-full text-[var(--app-muted)] transition-colors hover:bg-[var(--app-hover)] hover:text-[var(--app-text)]"
        >
          <X size={15} />
        </button>
      ) : (
        <div className="pointer-events-none absolute right-4 top-1/2 -translate-y-1/2">
          <span className="rounded-xl border border-[var(--app-border)] bg-[var(--app-panel)] px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wider text-[var(--app-muted)]">
            Enter
          </span>
        </div>
      )}
      <AnimatePresence>
        {focused && trimmedQuery && (
          <motion.div
            initial={{ opacity: 0, y: 8, scale: 0.98 }}
            animate={{ opacity: 1, y: 0, scale: 1 }}
            exit={{ opacity: 0, y: 6, scale: 0.98 }}
            transition={{ duration: 0.14 }}
            onMouseDown={(event) => event.preventDefault()}
            className="absolute left-0 right-0 top-full z-50 mt-3 overflow-hidden rounded-2xl border border-[var(--app-border)] bg-[var(--app-panel)] shadow-2xl"
          >
            <div className="max-h-[420px] overflow-y-auto p-2">
              {loading && (
                <div className="flex items-center gap-2 px-3 py-4 text-sm text-[var(--app-muted)]">
                  <span className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-brand-500 border-t-transparent" />
                  搜索中...
                </div>
              )}
              {!loading && error && (
                <div className="px-3 py-4 text-sm text-red-500">{error}</div>
              )}
              {!loading && !error && cards.length === 0 && (
                <div className="px-3 py-4 text-sm text-[var(--app-muted)]">没有找到匹配的本地媒体</div>
              )}
              {!loading && !error && cards.length > 0 && (
                <div className="space-y-1">
                  {cards.map((card) => (
                    <SearchResultItem
                      key={card.key}
                      card={card}
                      onClick={() => onFocusedChange(false)}
                    />
                  ))}
                </div>
              )}
            </div>
            <Link
              to={`/search?q=${encodeURIComponent(trimmedQuery)}`}
              onClick={() => onFocusedChange(false)}
              className="flex items-center justify-between border-t border-[var(--app-border)] px-4 py-3 text-sm font-semibold text-brand-500 hover:bg-[var(--app-hover)]"
            >
              <span>查看全部搜索结果</span>
              <span className="text-xs text-[var(--app-muted)]">
                {total > 0 ? `${total} 部作品` : 'Enter'}
              </span>
            </Link>
          </motion.div>
        )}
      </AnimatePresence>
    </form>
  )
}

function SearchResultItem({ card, onClick }: { card: SeriesCard; onClick: () => void }) {
  const poster = mediaPosterURL(card.rep)

  return (
    <Link
      to={seriesCardLink(card)}
      onClick={onClick}
      className="flex items-center gap-3 rounded-xl px-2.5 py-2 transition-colors hover:bg-[var(--app-hover)]"
    >
      <div className="h-14 w-10 shrink-0 overflow-hidden rounded-lg bg-[var(--app-panel-soft)]">
        {poster ? (
          <img
            src={imageURL(poster, card.rep.updated_at, { maxWidth: 96, quality: 78 })}
            alt={card.rep.title}
            decoding="async"
            className="h-full w-full object-cover"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-[var(--app-muted)]">
            <LibraryIcon size={16} />
          </div>
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-semibold text-[var(--app-text)]">
          {card.rep.title || card.rep.original_name || '未命名媒体'}
        </div>
        <div className="mt-1 flex items-center gap-2 text-[11px] text-[var(--app-muted)]">
          {card.rep.year ? <span>{card.rep.year}</span> : null}
          <span>{card.count > 1 ? `${card.count} 集/条目` : '单条媒体'}</span>
          {card.rep.width ? <span>{card.rep.width}x{card.rep.height}</span> : null}
        </div>
      </div>
    </Link>
  )
}
