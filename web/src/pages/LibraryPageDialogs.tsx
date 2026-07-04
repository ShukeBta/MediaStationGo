import { ManualScrapeDialog } from '../components/ManualScrapeDialog'
import { MetadataEditDialog } from '../components/MetadataEditDialog'
import type { Media } from '../types'
import { seriesTitle, type SeriesCard } from '../utils/groupSeries'

type LibraryPageDialogsProps = {
  manualSeriesScrapeOpen: boolean
  seriesMetadataEditOpen: boolean
  manualMovie: Media | null
  selectedSeries: SeriesCard | null
  selectedSeriesMediaIDs: string[]
  libraryType?: string
  scrapeEpisodeArtwork: boolean
  onCloseManualSeriesScrape: () => void
  onCloseSeriesMetadataEdit: () => void
  onCloseManualMovie: () => void
  onApplied: () => void
}

export function LibraryPageDialogs({
  manualSeriesScrapeOpen,
  seriesMetadataEditOpen,
  manualMovie,
  selectedSeries,
  selectedSeriesMediaIDs,
  libraryType,
  scrapeEpisodeArtwork,
  onCloseManualSeriesScrape,
  onCloseSeriesMetadataEdit,
  onCloseManualMovie,
  onApplied,
}: LibraryPageDialogsProps) {
  const selectedSeriesTitle = selectedSeries ? seriesTitle(selectedSeries.rep) : ''

  return (
    <>
      <ManualScrapeDialog
        open={manualSeriesScrapeOpen}
        media={selectedSeries?.rep ?? null}
        mediaIds={selectedSeriesMediaIDs}
        defaultQuery={selectedSeriesTitle}
        mediaType={selectedSeries ? scrapeMediaType(libraryType, selectedSeries.rep) : 'tv'}
        scopeLabel={selectedSeriesTitle || '当前剧集'}
        episodeArtwork={scrapeEpisodeArtwork}
        onClose={onCloseManualSeriesScrape}
        onApplied={onApplied}
      />
      <MetadataEditDialog
        open={seriesMetadataEditOpen}
        media={selectedSeries?.rep ?? null}
        mediaIds={selectedSeriesMediaIDs}
        mode="series"
        scopeLabel={selectedSeriesTitle || '当前剧集'}
        onClose={onCloseSeriesMetadataEdit}
        onSaved={onApplied}
      />
      <ManualScrapeDialog
        open={!!manualMovie}
        media={manualMovie}
        defaultQuery={manualMovie?.title ?? ''}
        mediaType={manualMovie ? scrapeMediaType(libraryType, manualMovie) : libraryType || 'movie'}
        scopeLabel={manualMovie?.title ?? '当前电影'}
        episodeArtwork={scrapeEpisodeArtwork}
        onClose={onCloseManualMovie}
        onApplied={onApplied}
      />
    </>
  )
}

function scrapeMediaType(libraryType: string | undefined, media: Media): string {
  if ((media.season_num ?? 0) > 0 || (media.episode_num ?? 0) > 0) {
    return 'tv'
  }
  return libraryType || 'movie'
}
