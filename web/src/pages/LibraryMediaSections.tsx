import type { ReactNode } from 'react'
import { Film } from 'lucide-react'

import { MediaCard } from '../components/MediaCard'
import type { Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'

type LibraryMediaSectionsProps = {
  isSeries: boolean
  items: Media[]
  seriesCards: SeriesCard[]
  selectedSeries: SeriesCard | null
  loading: boolean
  movieActions: (media: Media) => ReactNode
  onSeriesClick: (series: SeriesCard) => void
}

export function LibraryMediaSections({
  isSeries,
  items,
  seriesCards,
  selectedSeries,
  loading,
  movieActions,
  onSeriesClick,
}: LibraryMediaSectionsProps) {
  return (
    <>
      {!isSeries && items.length > 0 && (
        <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-7 2xl:grid-cols-8">
          {items.map((media) => (
            <MediaCard key={media.id} media={media} actions={movieActions(media)} />
          ))}
        </div>
      )}

      {!isSeries && items.length === 0 && (
        <LibraryEmptyState message="该媒体库暂无内容，触发一次扫描后再来看看" />
      )}

      {isSeries && seriesCards.length > 0 && !selectedSeries && (
        <div className="grid grid-cols-3 gap-4 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-7 2xl:grid-cols-8">
          {seriesCards.map((series) => (
            <MediaCard
              key={series.key}
              media={series.rep}
              count={series.count}
              onClick={() => onSeriesClick(series)}
            />
          ))}
        </div>
      )}

      {isSeries && seriesCards.length === 0 && !loading && (
        <LibraryEmptyState message="该库尚未发现任何剧集，触发一次扫描后再来看看" />
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
