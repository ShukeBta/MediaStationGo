import { LoaderCircle } from 'lucide-react'

import { MediaCard } from '../components/MediaCard'
import type { SeriesCard } from '../utils/groupSeries'
import { seriesCardLink } from '../utils/groupSeries'

type SearchLocalResultsProps = {
  localCards: SeriesCard[]
  itemCount: number
  searchTotal: number
  loading: boolean
  loadingMore: boolean
  hasMore: boolean
  onLoadMore: () => void
}

export function SearchLocalResults({
  localCards,
  itemCount,
  searchTotal,
  loading,
  loadingMore,
  hasMore,
  onLoadMore,
}: SearchLocalResultsProps) {
  if (localCards.length === 0) return null

  return (
    <section className="space-y-4">
      <div className="text-sm font-semibold text-ink-100">
        本地媒体库 · 已显示 {itemCount} / {searchTotal} 部作品
      </div>
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6">
        {localCards.map((card) => (
          <MediaCard
            key={card.key}
            media={card.rep}
            count={card.count}
            linkTo={seriesCardLink(card)}
          />
        ))}
      </div>
      {hasMore && !loading && (
        <div className="flex justify-center">
          <button
            type="button"
            className="neon-button inline-flex items-center gap-2"
            onClick={onLoadMore}
            disabled={loadingMore}
          >
            {loadingMore && <LoaderCircle size={16} className="animate-spin" />}
            {loadingMore ? '正在加载' : '加载更多'}
          </button>
        </div>
      )}
    </section>
  )
}
