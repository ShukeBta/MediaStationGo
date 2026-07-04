import { AnimatePresence, motion } from 'framer-motion'

import type { Media } from '../types'
import type { SeriesCard } from '../utils/groupSeries'
import { LibrarySeriesDetailHeader } from './LibrarySeriesDetailHeader'
import { LibrarySeriesEpisodes } from './LibrarySeriesEpisodes'

type SeasonEpisodes = {
  season: number
  episodes: Media[]
}

type LibrarySeriesDetailSectionProps = {
  selectedSeries: SeriesCard | null
  selectedEpisodes: SeasonEpisodes[]
  selectedSeason: number | null
  visibleEpisodes: Media[]
  allEpisodes: Media[]
  loadingEpisodes: boolean
  playbackFrom: string
  isAdmin: boolean
  seriesToolBusy: string
  onBack: () => void
  onSmartScrape: () => void
  onManualScrape: () => void
  onMetadataEdit: () => void
  onProbe: () => void
  onNFO: () => void
  onOrganize: () => void
  onSoftDelete: () => void
  onSeasonChange: (season: number) => void
}

export function LibrarySeriesDetailSection({
  selectedSeries,
  selectedEpisodes,
  selectedSeason,
  visibleEpisodes,
  allEpisodes,
  loadingEpisodes,
  playbackFrom,
  isAdmin,
  seriesToolBusy,
  onBack,
  onSmartScrape,
  onManualScrape,
  onMetadataEdit,
  onProbe,
  onNFO,
  onOrganize,
  onSoftDelete,
  onSeasonChange,
}: LibrarySeriesDetailSectionProps) {
  return (
    <AnimatePresence mode="wait">
      {selectedSeries && (
        <motion.div
          initial={{ opacity: 0, y: 16 }}
          animate={{ opacity: 1, y: 0 }}
          exit={{ opacity: 0 }}
          className="space-y-6"
        >
          <LibrarySeriesDetailHeader
            series={selectedSeries}
            visibleEpisodes={visibleEpisodes}
            allEpisodes={allEpisodes}
            playbackFrom={playbackFrom}
            isAdmin={isAdmin}
            seriesToolBusy={seriesToolBusy}
            onBack={onBack}
            onSmartScrape={onSmartScrape}
            onManualScrape={onManualScrape}
            onMetadataEdit={onMetadataEdit}
            onProbe={onProbe}
            onNFO={onNFO}
            onOrganize={onOrganize}
            onSoftDelete={onSoftDelete}
          />

          <LibrarySeriesEpisodesPanel
            loading={loadingEpisodes}
            selectedEpisodes={selectedEpisodes}
            selectedSeason={selectedSeason}
            visibleEpisodes={visibleEpisodes}
            playbackFrom={playbackFrom}
            onSeasonChange={onSeasonChange}
          />
        </motion.div>
      )}
    </AnimatePresence>
  )
}

type LibrarySeriesEpisodesPanelProps = {
  loading: boolean
  selectedEpisodes: SeasonEpisodes[]
  selectedSeason: number | null
  visibleEpisodes: Media[]
  playbackFrom: string
  onSeasonChange: (season: number) => void
}

function LibrarySeriesEpisodesPanel({
  loading,
  selectedEpisodes,
  selectedSeason,
  visibleEpisodes,
  playbackFrom,
  onSeasonChange,
}: LibrarySeriesEpisodesPanelProps) {
  return (
    <div className="space-y-6">
      <LibrarySeriesEpisodes
        loading={loading}
        selectedEpisodes={selectedEpisodes}
        selectedSeason={selectedSeason}
        visibleEpisodes={visibleEpisodes}
        playbackFrom={playbackFrom}
        onSeasonChange={onSeasonChange}
      />
    </div>
  )
}
