import type { ReactNode } from 'react'
import { Film } from 'lucide-react'

import { MediaCard } from '../components/MediaCard'
import { Pagination } from '../components/Pagination'
import type { Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'

type LibraryMediaSectionsProps = {
  isSeries: boolean
  items: Media[]
  seriesCards: SeriesCard[]
  selectedSeries: SeriesCard | null
  loading: boolean
  page: number
  total: number
  onPageChange: (page: number) => void
  movieActions: (media: Media) => ReactNode
  onSeriesClick: (series: SeriesCard) => void
  highlightedMediaID?: string
  followedSeriesKeys?: Set<string>
}

const LIBRARY_CARD_PAGE_SIZE = 48

export function LibraryMediaSections({
  isSeries,
  items,
  seriesCards,
  selectedSeries,
  loading,
  page,
  total,
  onPageChange,
  movieActions,
  onSeriesClick,
  highlightedMediaID,
  followedSeriesKeys = new Set(),
}: LibraryMediaSectionsProps) {
  const totalPages = Math.max(1, Math.ceil(total / LIBRARY_CARD_PAGE_SIZE))
  if (selectedSeries) return null
  if (loading) return <div role="status" className="py-24 text-center text-sm text-sand-500">加载当前页…</div>

  return (
    <>
      {!isSeries && items.length > 0 && (
        <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-7 2xl:grid-cols-8">
          {items.map((media) => (
            <div
              key={media.id}
              className={highlightedMediaID === media.id ? 'rounded-lg ring-4 ring-emerald-400/70 ring-offset-2' : ''}
            >
              <MediaCard media={media} actions={movieActions(media)} />
            </div>
          ))}
        </div>
      )}

      {!isSeries && items.length === 0 && (
        <LibraryEmptyState message="没有符合当前条件的内容" />
      )}

      {isSeries && seriesCards.length > 0 && !selectedSeries && (
        <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-7 2xl:grid-cols-8">
          {seriesCards.map((series) => (
            <div
              key={series.key}
              className={
                highlightedMediaID === series.rep.id || highlightedMediaID === series.linkMedia.id
                  ? 'rounded-lg ring-4 ring-emerald-400/70 ring-offset-2'
                  : ''
              }
            >
              <MediaCard
                media={series.rep}
                count={series.count}
                autoFollow={followedSeriesKeys.has(series.key)}
                onClick={() => onSeriesClick(series)}
              />
            </div>
          ))}
        </div>
      )}

      {isSeries && seriesCards.length === 0 && !loading && (
        <LibraryEmptyState message="没有符合当前条件的剧集" />
      )}

      {total > 0 && totalPages > 1 && (
        <Pagination page={page} totalPages={totalPages} onPageChange={onPageChange} className="pt-2" />
      )}
    </>
  )
}

function LibraryEmptyState({ message }: { message: string }) {
  return (
    <div className="flex flex-col items-center justify-center py-24 text-center">
      <Film className="mb-4 h-12 w-12 text-gray-500" />
      <p className="text-ink-50">{message}</p>
    </div>
  )
}
